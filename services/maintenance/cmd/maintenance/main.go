package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/classifier"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/config"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/dispatcher"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/grafana"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/state"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/telegram"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/transport"
)

func main() {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Starting AnimeEnigma Maintenance Service...")

	// Preflight: verify Claude CLI
	if err := verifyClaudeCLI(cfg.Claude.Path); err != nil {
		fmt.Fprintf(os.Stderr, "claude CLI check failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Claude CLI verified")

	// Initialize Telegram client
	tg := telegram.NewClient(cfg.Telegram.BotToken, cfg.Telegram.ChatID)

	// Preflight: verify Telegram bot
	webhookInfo, err := tg.GetWebhookInfo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "telegram webhook check failed: %v\n", err)
		os.Exit(1)
	}
	if webhookInfo.URL != "" {
		fmt.Fprintf(os.Stderr, "FATAL: alerts bot has webhook set (%s). getUpdates will not work. Remove webhook first.\n", webhookInfo.URL)
		os.Exit(1)
	}
	fmt.Println("✓ No webhook conflict")

	botInfo, err := tg.GetMe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "telegram getMe failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Bot: @%s (ID: %d)\n", botInfo.Username, botInfo.ID)

	// Check if bot can use reactions (needs admin in supergroup)
	reactionsSupported := false
	member, err := tg.GetChatMember(botInfo.ID)
	if err == nil && member.Status == "administrator" {
		reactionsSupported = true
	}
	tg.SetReactionsSupported(reactionsSupported)
	fmt.Printf("✓ Reactions: %v (bot status: %s)\n", reactionsSupported, memberStatus(member))

	// Initialize state manager
	stateMgr := state.NewManager(
		resolveProjectPath(cfg.Claude.ProjectRoot, cfg.StatePath),
		resolveProjectPath(cfg.Claude.ProjectRoot, cfg.IssuePath),
	)
	if err := stateMgr.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "state load failed: %v\n", err)
		os.Exit(1)
	}
	stateMgr.SetBotInfo(botInfo.ID, reactionsSupported)
	stateMgr.SetSessionStarted()
	fmt.Println("✓ State loaded")

	// Initialize Claude dispatcher
	disp := dispatcher.New(
		cfg.Claude.Path,
		cfg.Claude.ProjectRoot,
		cfg.Claude.PromptPath,
		cfg.Claude.Model,
		cfg.Claude.CodeModel,
		cfg.Claude.TimeoutSec,
	)

	// Start HTTP server (health + metrics + report intake)
	// workChan is initialized here so the router can inject reports; stored on the service struct
	workChan := make(chan workItem, 10)
	router := transport.NewRouter(func(report domain.ReportRequest) {
		workChan <- workItem{reports: []domain.ReportRequest{report}}
	})
	server := &http.Server{
		Addr:    cfg.Server.Address(),
		Handler: router,
	}
	go func() {
		fmt.Printf("✓ HTTP server on %s\n", cfg.Server.Address())
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "HTTP server error: %v\n", err)
		}
	}()

	// Initialize Grafana client
	gf := grafana.NewClient(cfg.Grafana.URL)
	// Preflight: verify Grafana connectivity
	if alerts, err := gf.GetFiringAlerts(); err != nil {
		fmt.Printf("⚠ Grafana check failed (will retry): %v\n", err)
	} else {
		fmt.Printf("✓ Grafana connected (%d active alerts)\n", len(alerts))
	}

	// Send startup message
	tg.SendMessage("🤖 <b>Maintenance service started</b>\nMonitoring alerts (Grafana API) + user messages (Telegram).")
	fmt.Println("✓ Startup message sent")

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start polling loop
	svc := &service{
		tg:          tg,
		gf:          gf,
		disp:        disp,
		state:       stateMgr,
		cfg:         cfg,
		alertsBotID: cfg.Telegram.AlertsBotID,
		workChan:    workChan,
	}

	go svc.run(ctx)

	// Wait for shutdown signal
	sig := <-sigChan
	fmt.Printf("\nReceived %s, shutting down...\n", sig)
	cancel()

	// Grace period for in-progress Claude invocations
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	server.Shutdown(shutdownCtx)

	tg.SendMessage("🤖 <b>Maintenance service stopping</b>")
	stateMgr.Save()
	fmt.Println("Shutdown complete.")
}

type service struct {
	tg          *telegram.Client
	gf          *grafana.Client
	disp        *dispatcher.Dispatcher
	state       *state.Manager
	cfg         *config.Config
	alertsBotID int64 // The alerts bot's user ID (for classifying Grafana messages)
	workChan    chan workItem
	mu          sync.Mutex
}

// workItem carries either Telegram updates, Grafana alerts, or HTTP reports to the processor.
type workItem struct {
	telegramUpdates []telegram.Update
	grafanaAlerts   []domain.ClassifiedMessage
	reports         []domain.ReportRequest
}

func (s *service) run(ctx context.Context) {
	workChan := s.workChan

	// Goroutine 1: Telegram poller (user messages, button clicks)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			st := s.state.State()
			updates, err := s.tg.GetUpdates(st.LastUpdateID+1, 60)
			if err != nil {
				fmt.Printf("[telegram] poll error: %v\n", err)
				time.Sleep(5 * time.Second)
				continue
			}
			if len(updates) > 0 {
				maxID := int64(0)
				for _, u := range updates {
					if u.UpdateID > maxID {
						maxID = u.UpdateID
					}
				}
				s.state.UpdateOffset(maxID)
				// Send each update individually for one-by-one processing
				for _, u := range updates {
					select {
					case <-ctx.Done():
						return
					case workChan <- workItem{telegramUpdates: []telegram.Update{u}}:
					}
				}
			}
		}
	}()

	// Goroutine 2: Grafana alert poller (checks firing alerts periodically)
	go func() {
		interval := time.Duration(s.cfg.Grafana.PollInterval) * time.Second
		if interval < 30*time.Second {
			interval = 30 * time.Second
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				alerts, err := s.gf.GetFiringAlerts()
				if err != nil {
					fmt.Printf("[grafana] poll error: %v\n", err)
					continue
				}

				// Also check for resolved alerts: compare current with active_alerts
				s.checkResolvedAlerts(alerts)

				// Only send new (not already tracked, not suppressed) alerts
				var newAlerts []domain.ClassifiedMessage
				for _, a := range alerts {
					if len(a.Alerts) > 0 {
						key := a.Alerts[0].Name + ":" + a.Alerts[0].Service
						// Skip suppressed alerts
						if s.isSuppressed(key) {
							continue
						}
						if existing := s.state.GetActiveAlert(key); existing == nil {
							newAlerts = append(newAlerts, a)
						}
					}
				}
				if len(newAlerts) > 0 {
					fmt.Printf("[grafana] %d new alerts detected\n", len(newAlerts))
					workChan <- workItem{grafanaAlerts: newAlerts}
				}
			}
		}
	}()

	// Goroutine 3: Processor (sequential, handles both sources)
	for {
		select {
		case <-ctx.Done():
			return
		case work := <-workChan:
			// Separate sources while draining
			var telegramQueue []telegram.Update
			var grafanaAlerts []domain.ClassifiedMessage
			var reports []domain.ReportRequest

			telegramQueue = append(telegramQueue, work.telegramUpdates...)
			grafanaAlerts = append(grafanaAlerts, work.grafanaAlerts...)
			reports = append(reports, work.reports...)

		drainLoop:
			for {
				select {
				case more := <-workChan:
					telegramQueue = append(telegramQueue, more.telegramUpdates...)
					grafanaAlerts = append(grafanaAlerts, more.grafanaAlerts...)
					reports = append(reports, more.reports...)
				default:
					break drainLoop
				}
			}

			// Process Grafana alerts first (lightweight, coalesced)
			if len(grafanaAlerts) > 0 {
				s.processWork(ctx, workItem{grafanaAlerts: grafanaAlerts})
			}

			// Process HTTP reports one at a time (each posts to Telegram + invokes Claude)
			for i, report := range reports {
				select {
				case <-ctx.Done():
					return
				default:
				}
				s.processReport(ctx, report)

				if i < len(reports)-1 {
					select {
					case <-ctx.Done():
						return
					case <-time.After(3 * time.Second):
					}
				}
			}

			// Process Telegram updates one at a time
			for i, u := range telegramQueue {
				select {
				case <-ctx.Done():
					return
				default:
				}
				s.processWork(ctx, workItem{telegramUpdates: []telegram.Update{u}})

				// Cooldown between messages (pacing + allows Grafana interleave)
				if i < len(telegramQueue)-1 {
					select {
					case <-ctx.Done():
						return
					case <-time.After(3 * time.Second):
					}
				}
			}
		}
	}
}

// checkResolvedAlerts detects alerts that were active but are no longer firing.
func (s *service) checkResolvedAlerts(currentAlerts []domain.ClassifiedMessage) {
	// Build set of currently firing alert keys
	currentKeys := make(map[string]bool)
	for _, a := range currentAlerts {
		if len(a.Alerts) > 0 {
			key := a.Alerts[0].Name + ":" + a.Alerts[0].Service
			currentKeys[key] = true
		}
	}

	// Check each active alert — if no longer in Grafana, it resolved
	st := s.state.State()
	for key, active := range st.ActiveAlerts {
		if !currentKeys[key] {
			fmt.Printf("[grafana] Alert resolved: %s\n", key)
			s.state.UpdateIssue(active.IssueID, func(issue *domain.Issue) {
				issue.Status = domain.StatusResolved
				issue.ResolvedAt = time.Now().UTC().Format(time.RFC3339)
				issue.Resolution = "Alert resolved (no longer firing in Grafana)"
			})
			s.state.RemoveActiveAlert(key)

			// Notify in Telegram
			duration := "unknown"
			if firstSeen, err := time.Parse(time.RFC3339, active.FirstSeen); err == nil {
				duration = time.Since(firstSeen).Truncate(time.Second).String()
			}
			s.tg.SendMessage(fmt.Sprintf(
				"<b>✅ Alert Resolved</b>\n<b>Alert:</b> %s (%s)\n<b>Duration:</b> %s\n<b>Issue:</b> %s",
				active.AlertUID, active.Service, duration, active.IssueID,
			))
		}
	}
	s.state.Save()
}

func (s *service) processWork(ctx context.Context, work workItem) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Classify Telegram updates (user messages, button clicks)
	var batch domain.ClassifiedBatch
	if len(work.telegramUpdates) > 0 {
		batch = classifier.ClassifyBatch(work.telegramUpdates, s.alertsBotID, s.cfg.Admins)
	}

	// Add Grafana alerts to the relevant queue
	batch.Relevant = append(batch.Relevant, work.grafanaAlerts...)

	// Triage: check for multi-service outage
	activeAlertCount := s.state.CountActiveAlerts()
	batchAlertCount := classifier.CountAffectedServices(batch)
	if activeAlertCount+batchAlertCount >= 3 {
		s.escalateBatch(batch)
		s.state.Save()
		return
	}

	// Handle button clicks first
	for _, cb := range batch.ButtonClicks {
		s.handleButtonClick(ctx, cb)
	}

	// Debounce
	time.Sleep(500 * time.Millisecond)

	// Process relevant messages (invoke Claude for each)
	for _, msg := range batch.Relevant {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Dedup: check if this alert is already being tracked
		if msg.Type == domain.MessageAlertFiring && len(msg.Alerts) > 0 {
			alert := msg.Alerts[0]
			key := alert.Name + ":" + alert.Service
			if existing := s.state.GetActiveAlert(key); existing != nil {
				// Already tracking this alert — skip
				continue
			}
		}

		// Cooldown check: skip if same service was recently fixed
		if msg.Type == domain.MessageAlertFiring && len(msg.Alerts) > 0 {
			svc := msg.Alerts[0].Service
			if s.state.WasRecentlyFixed(svc, 10*time.Minute) {
				continue
			}
		}

		// React with eyes
		if !s.tg.SetReaction(msg.MessageID, "👀") {
			fmt.Printf("[reaction] failed to set 👀 on message %d\n", msg.MessageID)
		}

		// Invoke Claude
		fmt.Printf("[claude] analyzing message %d (type: %d, priority: %d)...\n", msg.MessageID, msg.Type, msg.Priority)
		analyzeStart := time.Now()
		result, err := s.disp.Analyze(ctx, msg)
		elapsed := time.Since(analyzeStart).Round(time.Second)

		if err != nil {
			fmt.Printf("[claude] FAILED after %s: %v\n", elapsed, err)
			if !s.tg.SetReaction(msg.MessageID, "💔") {
				fmt.Printf("[reaction] failed to set 💔 on message %d\n", msg.MessageID)
			}
			s.tg.SendReply(msg.MessageID, fmt.Sprintf("<b>⚠️ Analysis failed</b>\n%s", truncateForTelegram(err.Error())))
			continue
		}

		fmt.Printf("[claude] completed in %s — tier=%s issue=%q category=%s priority=%s\n",
			elapsed, result.Tier, result.Issue.Title, result.Issue.Category, result.Issue.Priority)
		fmt.Printf("[claude] diagnosis: %s\n", truncateForTelegram(result.Diagnosis.RootCause))
		if result.FixPlan != nil {
			fmt.Printf("[claude] fix_plan: type=%s target=%s desc=%s\n", result.FixPlan.Type, result.FixPlan.Target, result.FixPlan.Description)
		}

		// React with success
		if !s.tg.SetReaction(msg.MessageID, "👍") {
			fmt.Printf("[reaction] failed to set 👍 on message %d\n", msg.MessageID)
		}

		// Handle result based on tier
		s.handleResult(ctx, msg, result)
	}

	// Housekeeping
	expired := s.state.ExpirePendingFixes(24 * time.Hour)
	for _, id := range expired {
		s.state.UpdateIssue(id, func(issue *domain.Issue) {
			issue.Status = domain.StatusEscalated
			issue.Resolution = "Pending fix expired after 1 hour"
		})
	}

	s.state.Save()
}

// processReport handles an error report received via HTTP:
// posts it to Telegram (so maintenance bot owns the message), reacts, analyzes, replies.
func (s *service) processReport(ctx context.Context, report domain.ReportRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Build the same 🚨 report message format the player service used
	var b strings.Builder
	b.WriteString("🚨 <b>Player Error Report</b>\n\n")
	b.WriteString(fmt.Sprintf("👤 <b>User:</b> %s (ID: %s)\n", escTelegram(report.Username), escTelegram(report.UserID)))
	b.WriteString(fmt.Sprintf("🎬 <b>Player:</b> %s\n", escTelegram(report.PlayerType)))
	b.WriteString(fmt.Sprintf("📺 <b>Anime:</b> %s\n", escTelegram(report.AnimeName)))
	if report.EpisodeNumber != nil {
		b.WriteString(fmt.Sprintf("📋 <b>Episode:</b> %d\n", *report.EpisodeNumber))
	}
	if report.ServerName != "" {
		b.WriteString(fmt.Sprintf("🖥 <b>Server:</b> %s\n", escTelegram(report.ServerName)))
	}
	if report.ErrorMessage != "" {
		msg := report.ErrorMessage
		if len(msg) > 200 {
			msg = msg[:200] + "..."
		}
		b.WriteString(fmt.Sprintf("\n⚠️ <b>Error:</b> <code>%s</code>\n", escTelegram(msg)))
	}
	if report.Description != "" {
		desc := report.Description
		if len(desc) > 500 {
			desc = desc[:500] + "..."
		}
		b.WriteString(fmt.Sprintf("\n💬 <b>Description:</b>\n%s\n", escTelegram(desc)))
	}
	if report.URL != "" {
		b.WriteString(fmt.Sprintf("\n🔗 %s", escTelegram(report.URL)))
	}
	if report.ReportFile != "" {
		b.WriteString(fmt.Sprintf("\n📁 <code>%s</code>", escTelegram(report.ReportFile)))
	}

	// Post to Telegram — maintenance bot owns this message
	fmt.Printf("[report] received from %s (player: %s, anime: %s)\n", report.Username, report.PlayerType, report.AnimeName)
	msgID, err := s.tg.SendMessage(b.String())
	if err != nil {
		fmt.Printf("[report] FAILED to post to Telegram: %v\n", err)
		return
	}
	fmt.Printf("[report] posted to Telegram as message %d\n", msgID)

	// React with 👀
	if !s.tg.SetReaction(msgID, "👀") {
		fmt.Printf("[reaction] failed to set 👀 on message %d\n", msgID)
	}

	// Build ClassifiedMessage for Claude analysis
	msg := domain.ClassifiedMessage{
		MessageID: msgID,
		Type:      domain.MessageErrorReport,
		Priority:  domain.P2,
		Text:      b.String(),
		From:      domain.User{Username: report.Username},
		RawJSON:   fmt.Sprintf(`{"report": {"player":"%s","anime":"%s","error":"%s","description":"%s","server":"%s","url":"%s","file":"%s"}}`, report.PlayerType, report.AnimeName, report.ErrorMessage, report.Description, report.ServerName, report.URL, report.ReportFile),
	}

	// Invoke Claude
	fmt.Printf("[claude] analyzing report (message %d)...\n", msgID)
	analyzeStart := time.Now()
	result, err := s.disp.Analyze(ctx, msg)
	elapsed := time.Since(analyzeStart).Round(time.Second)

	if err != nil {
		fmt.Printf("[claude] report analysis FAILED after %s: %v\n", elapsed, err)
		if !s.tg.SetReaction(msgID, "💔") {
			fmt.Printf("[reaction] failed to set 💔 on message %d\n", msgID)
		}
		s.tg.SendReply(msgID, fmt.Sprintf("<b>⚠️ Analysis failed</b>\n%s", truncateForTelegram(err.Error())))
		return
	}

	fmt.Printf("[claude] report analysis completed in %s — tier=%s issue=%q category=%s\n",
		elapsed, result.Tier, result.Issue.Title, result.Issue.Category)
	fmt.Printf("[claude] diagnosis: %s\n", truncateForTelegram(result.Diagnosis.RootCause))
	if result.FixPlan != nil {
		fmt.Printf("[claude] fix_plan: type=%s target=%s desc=%s\n", result.FixPlan.Type, result.FixPlan.Target, result.FixPlan.Description)
	}

	if !s.tg.SetReaction(msgID, "👍") {
		fmt.Printf("[reaction] failed to set 👍 on message %d\n", msgID)
	}
	s.handleResult(ctx, msg, result)
	s.state.Save()
	fmt.Printf("[report] processing complete for message %d\n", msgID)
}

func (s *service) handleResult(ctx context.Context, msg domain.ClassifiedMessage, result *domain.AnalysisResult) {
	// Create issue
	source := "grafana_alert"
	reporter := "grafana"
	if msg.Type == domain.MessageAdminMessage {
		source = "admin_request"
		reporter = "@" + msg.From.Username
	} else if msg.Type == domain.MessageUserIssue {
		source = "user_report"
		reporter = "@" + msg.From.Username
	} else if msg.Type == domain.MessageErrorReport {
		source = "error_report"
		reporter = "player_service"
	}

	affectedService := ""
	if len(msg.Alerts) > 0 {
		affectedService = msg.Alerts[0].Service
	}

	issueID := s.state.CreateIssue(domain.Issue{
		Source:            source,
		Category:          domain.IssueCategory(result.Issue.Category),
		Priority:          result.Issue.Priority,
		Status:            domain.IssueStatus(result.Issue.Status),
		Title:             result.Issue.Title,
		Reporter:          reporter,
		TelegramMessageID: msg.MessageID,
		AffectedService:   affectedService,
		Actions:           result.ActionsTaken,
	})
	fmt.Printf("[issue] created %s — %q (tier=%s, source=%s, reporter=%s)\n", issueID, result.Issue.Title, result.Tier, source, reporter)

	// Track active alert
	if msg.Type == domain.MessageAlertFiring && len(msg.Alerts) > 0 {
		alert := msg.Alerts[0]
		key := alert.Name + ":" + alert.Service
		s.state.SetActiveAlert(key, domain.ActiveAlert{
			AlertUID:  alert.Name,
			Service:   alert.Service,
			FirstSeen: time.Now().UTC().Format(time.RFC3339),
			LastSeen:  time.Now().UTC().Format(time.RFC3339),
			IssueID:   issueID,
			Status:    string(result.Tier),
		})
	}

	// Record fix if auto-fixed
	if result.Tier == domain.TierAutoFix && affectedService != "" {
		s.state.RecordFix(affectedService, "auto_fix")
		s.state.SetCooldown("restart", affectedService, 10*time.Minute)
	}

	// Send response to Telegram
	replyHTML := result.ReplyHTML
	if !strings.Contains(replyHTML, issueID) {
		replyHTML += fmt.Sprintf("\n\n<b>Issue:</b> %s", issueID)
	}

	// sendFunc: reply to existing message, or send standalone if from Grafana API (no message_id)
	sendFunc := func(html string) (int, error) {
		var replyID int
		var sendErr error
		if msg.MessageID > 0 {
			replyID, sendErr = s.tg.SendReply(msg.MessageID, html)
		} else {
			replyID, sendErr = s.tg.SendMessage(html)
		}
		if sendErr != nil {
			fmt.Printf("[telegram] FAILED to send reply for %s: %v\n", issueID, sendErr)
		} else {
			fmt.Printf("[telegram] reply sent (message %d) for %s\n", replyID, issueID)
		}
		return replyID, sendErr
	}

	switch result.Tier {
	case domain.TierAutoFix, domain.TierEscalate, domain.TierInfoOnly:
		sendFunc(replyHTML)

	case domain.TierButtonFix:
		buttons := []telegram.InlineButton{
			{Text: "🔧 Apply Fix", CallbackData: "fix:" + issueID},
			{Text: "❌ Dismiss", CallbackData: "dismiss:" + issueID},
		}
		var sentMsgID int
		var err error
		if msg.MessageID > 0 {
			sentMsgID, err = s.tg.SendReplyWithButtons(msg.MessageID, replyHTML, buttons)
		} else {
			sentMsgID, err = s.tg.SendReplyWithButtons(0, replyHTML, buttons)
		}
		if err == nil && result.FixPlan != nil {
			s.state.AddPendingFix(issueID, domain.PendingFix{
				IssueID:           issueID,
				ProposedAt:        time.Now().UTC().Format(time.RFC3339),
				FixPlan:           *result.FixPlan,
				TelegramMessageID: sentMsgID,
				AlertMessageID:    msg.MessageID,
			})
		}
	}
}

func (s *service) handleButtonClick(ctx context.Context, msg domain.ClassifiedMessage) {
	parts := strings.SplitN(msg.CallbackData, ":", 2)
	if len(parts) != 2 {
		s.tg.AnswerCallbackQuery(msg.CallbackID, "Invalid callback data")
		return
	}

	action, issueID := parts[0], parts[1]

	// Verify admin
	isAdmin := false
	for _, admin := range s.cfg.Admins {
		if strings.EqualFold(msg.From.Username, admin) {
			isAdmin = true
			break
		}
	}
	if !isAdmin {
		s.tg.AnswerCallbackQuery(msg.CallbackID, "Admin only")
		return
	}

	fix := s.state.GetPendingFix(issueID)
	if fix == nil {
		s.tg.AnswerCallbackQuery(msg.CallbackID, "Fix expired or already handled")
		return
	}

	// Use the button's message for replies (always exists), fall back to alert message
	replyToID := fix.TelegramMessageID
	if replyToID == 0 {
		replyToID = fix.AlertMessageID
	}

	switch action {
	case "fix":
		fmt.Printf("[button] @%s approved fix for %s (target=%s, type=%s)\n", msg.From.Username, issueID, fix.FixPlan.Target, fix.FixPlan.Type)
		s.tg.AnswerCallbackQuery(msg.CallbackID, "Applying fix...")
		s.tg.SetReaction(replyToID, "👀")

		fmt.Printf("[claude] executing fix for %s...\n", issueID)
		analyzeStart := time.Now()
		result, err := s.disp.ExecuteFix(ctx, *fix)
		elapsed := time.Since(analyzeStart).Round(time.Second)

		if err != nil {
			fmt.Printf("[claude] fix execution FAILED for %s after %s: %v\n", issueID, elapsed, err)
			s.tg.SetReaction(replyToID, "💔")
			s.tg.SendReply(replyToID, fmt.Sprintf("<b>❌ Fix failed</b>\n%s", truncateForTelegram(err.Error())))
		} else {
			fmt.Printf("[claude] fix executed for %s in %s — tier=%s\n", issueID, elapsed, result.Tier)
			if len(result.ActionsTaken) > 0 {
				for _, a := range result.ActionsTaken {
					fmt.Printf("[claude]   action: %s → %s\n", a.Action, a.Result)
				}
			}
			s.tg.SetReaction(replyToID, "👍")
			replyHTML := result.ReplyHTML
			if replyHTML == "" {
				replyHTML = fmt.Sprintf("<b>🔧 Fix Applied</b> (approved by @%s)\n\n<b>Issue:</b> %s", msg.From.Username, issueID)
			}
			if _, err := s.tg.SendReply(replyToID, replyHTML); err != nil {
				fmt.Printf("[telegram] FAILED to send fix result for %s: %v\n", issueID, err)
			} else {
				fmt.Printf("[telegram] fix result sent for %s\n", issueID)
			}
			s.state.UpdateIssue(issueID, func(issue *domain.Issue) {
				issue.Status = domain.StatusResolved
				issue.Resolution = fmt.Sprintf("Fix applied by @%s", msg.From.Username)
				issue.Actions = append(issue.Actions, result.ActionsTaken...)
			})
			if fix.FixPlan.Target != "" {
				s.state.RecordFix(fix.FixPlan.Target, string(fix.FixPlan.Type))
				s.state.SetCooldown(string(fix.FixPlan.Type), fix.FixPlan.Target, 10*time.Minute)
			}
		}
		s.state.RemovePendingFix(issueID)
		s.state.Save()
		fmt.Printf("[button] fix processing complete for %s\n", issueID)

	case "dismiss":
		fmt.Printf("[button] @%s dismissed %s\n", msg.From.Username, issueID)
		s.tg.AnswerCallbackQuery(msg.CallbackID, "Dismissed")
		s.tg.SetReaction(replyToID, "💔")
		s.state.RemovePendingFix(issueID)
		s.state.UpdateIssue(issueID, func(issue *domain.Issue) {
			issue.Status = domain.StatusWontFix
			issue.Resolution = fmt.Sprintf("Dismissed by @%s", msg.From.Username)
		})
		s.tg.SendReply(fix.AlertMessageID, fmt.Sprintf("<b>Issue %s dismissed</b> by @%s", issueID, msg.From.Username))
	}
}

func (s *service) escalateBatch(batch domain.ClassifiedBatch) {
	var alertNames []string
	for _, msg := range batch.Relevant {
		for _, a := range msg.Alerts {
			alertNames = append(alertNames, a.Name)
		}
	}

	html := fmt.Sprintf(
		"<b>⚠️ Multi-Service Outage Detected</b>\n\n"+
			"<b>Affected alerts:</b> %s\n"+
			"<b>Count:</b> 3+ services\n\n"+
			"Automated fixes disabled — likely infrastructure issue.\n"+
			"Manual investigation required.",
		strings.Join(alertNames, ", "),
	)

	if len(batch.Relevant) > 0 {
		s.tg.SendReply(batch.Relevant[0].MessageID, html)
	} else {
		s.tg.SendMessage(html)
	}
}

// --- Helpers ---

func verifyClaudeCLI(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("claude --version failed: %w (output: %s)", err, string(output))
	}
	fmt.Printf("✓ Claude CLI: %s", strings.TrimSpace(string(output)))
	return nil
}

func resolveProjectPath(projectRoot, relativePath string) string {
	if strings.HasPrefix(relativePath, "/") {
		return relativePath
	}
	return projectRoot + "/" + relativePath
}

func memberStatus(m *telegram.ChatMember) string {
	if m == nil {
		return "unknown"
	}
	return m.Status
}

func (s *service) isSuppressed(alertKey string) bool {
	for _, suppressed := range s.cfg.SuppressedAlerts {
		if strings.EqualFold(alertKey, suppressed) {
			return true
		}
	}
	return false
}

func escTelegram(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func truncateForTelegram(s string) string {
	if len(s) > 500 {
		return s[:497] + "..."
	}
	return s
}
