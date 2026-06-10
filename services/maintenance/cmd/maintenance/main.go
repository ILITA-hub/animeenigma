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

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/classifier"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/config"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/dispatcher"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/grafana"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/state"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/telegram"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/transport"
)

// log is the package-level structured logger, initialized in main().
var log *logger.Logger

func main() {
	// Initialize structured logger
	log = logger.Default()
	defer func() { _ = log.Sync() }()

	// Load config
	cfg, err := config.Load()
	if err != nil {
		log.FatalSync("config error", "error", err)
	}

	log.Infow("starting AnimeEnigma maintenance service")

	// Preflight: verify Claude CLI
	if err := verifyClaudeCLI(cfg.Claude.Path); err != nil {
		log.FatalSync("claude CLI check failed", "error", err)
	}
	log.Infow("claude CLI verified")

	// Initialize Telegram client
	tg := telegram.NewClient(cfg.Telegram.BotToken, cfg.Telegram.ChatID, log)

	// Preflight: verify Telegram bot
	webhookInfo, err := tg.GetWebhookInfo()
	if err != nil {
		log.FatalSync("telegram webhook check failed", "error", err)
	}
	if webhookInfo.URL != "" {
		log.FatalSync("alerts bot has webhook set — getUpdates will not work, remove webhook first",
			"webhook_url", webhookInfo.URL,
		)
	}
	log.Infow("no webhook conflict")

	botInfo, err := tg.GetMe()
	if err != nil {
		log.FatalSync("telegram getMe failed", "error", err)
	}
	log.Infow("bot identified",
		"username", botInfo.Username,
		"bot_id", botInfo.ID,
	)

	// Check if bot can use reactions (needs admin in supergroup)
	reactionsSupported := false
	member, err := tg.GetChatMember(botInfo.ID)
	if err == nil && member.Status == "administrator" {
		reactionsSupported = true
	}
	tg.SetReactionsSupported(reactionsSupported)
	log.Infow("reactions support determined",
		"reactions_supported", reactionsSupported,
		"bot_status", memberStatus(member),
	)

	// Initialize state manager
	stateMgr := state.NewManager(
		resolveProjectPath(cfg.Claude.ProjectRoot, cfg.StatePath),
		resolveProjectPath(cfg.Claude.ProjectRoot, cfg.IssuePath),
	)
	if err := stateMgr.Load(); err != nil {
		log.FatalSync("state load failed", "error", err)
	}
	stateMgr.SetBotInfo(botInfo.ID, reactionsSupported)
	stateMgr.SetSessionStarted()
	log.Infow("state loaded")

	// Initialize Claude dispatcher
	disp := dispatcher.New(
		cfg.Claude.Path,
		cfg.Claude.ProjectRoot,
		cfg.Claude.PromptPath,
		cfg.Claude.Model,
		cfg.Claude.CodeModel,
		cfg.Claude.TimeoutSec,
		log,
	)

	// Start HTTP server (health + metrics + report intake + Grafana webhook)
	// workChan is initialized here so the router can inject reports; stored on the service struct
	workChan := make(chan workItem, 10)
	router := transport.NewRouter(
		func(report domain.ReportRequest) {
			workChan <- workItem{reports: []domain.ReportRequest{report}}
		},
		func(payload domain.GrafanaWebhookPayload) {
			workChan <- workItem{webhookEvents: []domain.GrafanaWebhookPayload{payload}}
		},
		cfg.Grafana.WebhookUser,
		cfg.Grafana.WebhookPass,
	)
	server := &http.Server{
		Addr:    cfg.Server.Address(),
		Handler: router,
	}
	go func() {
		log.Infow("HTTP server listening", "address", cfg.Server.Address())
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorw("HTTP server error", "error", err)
		}
	}()

	// Initialize Grafana client
	gf := grafana.NewClient(cfg.Grafana.URL)
	// Preflight: verify Grafana connectivity
	if alerts, err := gf.GetFiringAlerts(); err != nil {
		log.Warnw("grafana check failed (will retry)", "error", err)
	} else {
		log.Infow("grafana connected", "active_alerts", len(alerts))
	}

	// Send startup message
	tg.SendMessage("🤖 *Maintenance service started*\nMonitoring alerts (Grafana API) + user messages (Telegram).")
	log.Infow("startup message sent")

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start polling loop
	svc := &service{
		tg:       tg,
		gf:       gf,
		disp:     disp,
		state:    stateMgr,
		cfg:      cfg,
		workChan: workChan,
	}

	go svc.run(ctx)

	// Wait for shutdown signal
	sig := <-sigChan
	log.Infow("shutdown signal received", "signal", sig.String())
	cancel()

	// Grace period for in-progress Claude invocations
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	server.Shutdown(shutdownCtx)

	tg.SendMessage("🤖 *Maintenance service stopping*")
	stateMgr.Save()
	log.Infow("shutdown complete")
}

type service struct {
	tg       *telegram.Client
	gf       *grafana.Client
	disp     *dispatcher.Dispatcher
	state    *state.Manager
	cfg      *config.Config
	workChan chan workItem
	mu       sync.Mutex
}

// workItem carries either Telegram updates, Grafana alerts, HTTP reports, or webhook events to the processor.
type workItem struct {
	telegramUpdates []telegram.Update
	grafanaAlerts   []domain.ClassifiedMessage
	reports         []domain.ReportRequest
	webhookEvents   []domain.GrafanaWebhookPayload
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
				log.Warnw("telegram poll error", "error", err)
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

	// Goroutine 2: Grafana reconciliation poller.
	// Primary alert delivery is now via webhook (POST /api/grafana-webhook).
	// This poller is a safety net that catches missed webhook deliveries
	// (e.g. network blip, maintenance restart during burst, Grafana entrypoint failure).
	// Do NOT remove — without it, a missed webhook silently drops an alert.
	go func() {
		interval := time.Duration(s.cfg.Grafana.PollInterval) * time.Second
		if interval < 300*time.Second {
			interval = 300 * time.Second
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
					log.Warnw("grafana reconcile poll error", "error", err)
					continue
				}

				s.checkResolvedAlerts(alerts)

				var newAlerts []domain.ClassifiedMessage
				for _, a := range alerts {
					if len(a.Alerts) > 0 {
						key := a.Alerts[0].Name + ":" + a.Alerts[0].Service
						if s.isSuppressed(key) {
							continue
						}
						if existing := s.state.GetActiveAlert(key); existing == nil {
							newAlerts = append(newAlerts, a)
						}
					}
				}
				if len(newAlerts) > 0 {
					log.Infow("grafana reconcile detected missed alerts", "count", len(newAlerts))
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
			var webhookEvents []domain.GrafanaWebhookPayload

			telegramQueue = append(telegramQueue, work.telegramUpdates...)
			grafanaAlerts = append(grafanaAlerts, work.grafanaAlerts...)
			reports = append(reports, work.reports...)
			webhookEvents = append(webhookEvents, work.webhookEvents...)

		drainLoop:
			for {
				select {
				case more := <-workChan:
					telegramQueue = append(telegramQueue, more.telegramUpdates...)
					grafanaAlerts = append(grafanaAlerts, more.grafanaAlerts...)
					reports = append(reports, more.reports...)
					webhookEvents = append(webhookEvents, more.webhookEvents...)
				default:
					break drainLoop
				}
			}

			// Process webhook events: convert firing to ClassifiedMessages; resolve directly
			for _, payload := range webhookEvents {
				for _, wa := range payload.Alerts {
					alertName := wa.Labels["alertname"]
					service := grafana.ExtractService(wa.Labels, wa.Annotations)
					key := alertName + ":" + service

					if wa.Status == "resolved" || payload.Status == "resolved" {
						s.resolveAlertFromWebhook(key, wa)
						continue
					}
					// firing: build ClassifiedMessage for processWork pipeline
					severity := "warning"
					priority := domain.P1
					if grafana.CriticalAlerts[alertName] {
						severity = "critical"
						priority = domain.P0
					}
					grafanaAlerts = append(grafanaAlerts, domain.ClassifiedMessage{
						Type:     domain.MessageAlertFiring,
						Priority: priority,
						Text:     fmt.Sprintf("%s: %s", alertName, wa.Annotations["summary"]),
						From:     domain.User{Username: "grafana-webhook", IsBot: true},
						Alerts: []domain.AlertInfo{{
							Name:        alertName,
							Summary:     wa.Annotations["summary"],
							Description: wa.Annotations["description"],
							Service:     service,
							Severity:    severity,
						}},
					})
				}
			}

			// Process Grafana alerts (poller + webhook-converted) — coalesced through processWork
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
			log.Infow("grafana alert resolved", "alert_key", key)
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
				"*✅ Alert Resolved*\n*Alert:* %s (%s)\n*Duration:* %s\n*Issue:* %s",
				active.AlertUID, active.Service, duration, active.IssueID,
			))
		}
	}
	s.state.Save()
}

// resolveAlertFromWebhook handles a resolve event pushed by the Grafana webhook.
// Invariant: state.RemoveActiveAlert MUST happen before tg.SendMessage so the
// reconciliation poller cannot re-emit the same resolve.
func (s *service) resolveAlertFromWebhook(key string, wa domain.GrafanaWebhookAlert) {
	s.mu.Lock()
	defer s.mu.Unlock()

	active := s.state.GetActiveAlert(key)
	if active == nil {
		log.Warnw("webhook resolve for unknown alert — skipping", "alert_key", key)
		return
	}

	log.Infow("webhook alert resolved", "alert_key", key)
	s.state.UpdateIssue(active.IssueID, func(issue *domain.Issue) {
		issue.Status = domain.StatusResolved
		issue.ResolvedAt = time.Now().UTC().Format(time.RFC3339)
		issue.Resolution = "Alert resolved (webhook notification from Grafana)"
	})
	s.state.RemoveActiveAlert(key)

	duration := "unknown"
	if firstSeen, err := time.Parse(time.RFC3339, active.FirstSeen); err == nil {
		duration = time.Since(firstSeen).Truncate(time.Second).String()
	}
	s.tg.SendMessage(fmt.Sprintf(
		"*✅ Alert Resolved*\n*Alert:* %s (%s)\n*Duration:* %s\n*Issue:* %s",
		active.AlertUID, active.Service, duration, active.IssueID,
	))
	s.state.Save()
}

func (s *service) processWork(ctx context.Context, work workItem) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Classify Telegram updates (user messages, button clicks)
	var batch domain.ClassifiedBatch
	if len(work.telegramUpdates) > 0 {
		batch = classifier.ClassifyBatch(work.telegramUpdates, s.cfg.Admins)
	}

	// Add Grafana alerts to the relevant queue
	batch.Relevant = append(batch.Relevant, work.grafanaAlerts...)

	// Triage: check for multi-service outage
	activeAlertCount := s.state.CountActiveAlerts()
	batchAlertCount := classifier.CountAffectedServices(batch)
	if activeAlertCount+batchAlertCount >= 3 {
		// Throttle to at most one escalation message per 24h to avoid spam.
		if !s.state.IsInCooldown("escalate-outage", "global") {
			s.escalateBatch(batch)
			s.state.SetCooldown("escalate-outage", "global", 24*time.Hour)
		}
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

		// For webhook-sourced alerts (MessageID == 0), post 🔴 Firing notification
		if msg.Type == domain.MessageAlertFiring && msg.MessageID == 0 && len(msg.Alerts) > 0 {
			alert := msg.Alerts[0]
			fireMsg := fmt.Sprintf("*🔴 Firing*\n*%s*\n%s\n%s",
				escTelegram(alert.Name), escTelegram(alert.Summary), escTelegram(alert.Description))
			if sentID, err := s.tg.SendMessage(fireMsg); err == nil {
				msg.MessageID = sentID
			} else {
				log.Errorw("webhook failed to send firing message", "error", err)
			}
		}

		// React with eyes
		if !s.tg.SetReaction(msg.MessageID, "👀") {
			log.Warnw("failed to set reaction",
				"emoji", "👀",
				"message_id", msg.MessageID,
			)
		}

		// Invoke Claude
		log.Infow("analyzing message",
			"message_id", msg.MessageID,
			"type", msg.Type,
			"priority", msg.Priority,
		)
		analyzeStart := time.Now()
		result, err := s.disp.Analyze(ctx, msg)
		elapsed := time.Since(analyzeStart).Round(time.Second)

		if err != nil {
			log.Errorw("claude analysis failed",
				"duration", elapsed,
				"error", err,
			)
			if !s.tg.SetReaction(msg.MessageID, "💔") {
				log.Warnw("failed to set reaction",
					"emoji", "💔",
					"message_id", msg.MessageID,
				)
			}
			s.tg.SendReply(msg.MessageID, fmt.Sprintf("*⚠️ Analysis failed*\n%s", truncateForTelegram(err.Error())))
			continue
		}

		log.Infow("claude analysis completed",
			"duration", elapsed,
			"tier", result.Tier,
			"issue_title", result.Issue.Title,
			"category", result.Issue.Category,
			"priority", result.Issue.Priority,
		)
		log.Infow("claude diagnosis", "root_cause", truncateForTelegram(result.Diagnosis.RootCause))
		if result.FixPlan != nil {
			log.Infow("claude fix plan",
				"fix_type", result.FixPlan.Type,
				"target", result.FixPlan.Target,
				"description", result.FixPlan.Description,
			)
		}

		// React with success
		if !s.tg.SetReaction(msg.MessageID, "👍") {
			log.Warnw("failed to set reaction",
				"emoji", "👍",
				"message_id", msg.MessageID,
			)
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

	// Footer "Обратная связь" feedback (player_type=feedback) is user commentary
	// with no player context — render it by category instead of the player-error
	// layout, and skip the player/anime lines that would just print blanks.
	isFeedback := report.PlayerType == "feedback"

	var b strings.Builder
	if isFeedback {
		emoji, label := "📨", "User Feedback"
		switch report.Category {
		case "bug":
			emoji, label = "🐛", "Bug Report"
		case "issue":
			emoji, label = "❓", "Issue Report"
		case "feature":
			emoji, label = "💡", "Feature Request"
		}
		b.WriteString(fmt.Sprintf("%s *%s*\n\n", emoji, label))
	} else {
		b.WriteString("🚨 *Player Error Report*\n\n")
	}
	b.WriteString(fmt.Sprintf("👤 *User:* %s (ID: %s)\n", escTelegram(report.Username), escTelegram(report.UserID)))
	if !isFeedback {
		b.WriteString(fmt.Sprintf("🎬 *Player:* %s\n", escTelegram(report.PlayerType)))
	}
	if report.AnimeName != "" {
		b.WriteString(fmt.Sprintf("📺 *Anime:* %s\n", escTelegram(report.AnimeName)))
	}
	if report.EpisodeNumber != nil {
		b.WriteString(fmt.Sprintf("📋 *Episode:* %d\n", *report.EpisodeNumber))
	}
	if report.ServerName != "" {
		b.WriteString(fmt.Sprintf("🖥 *Server:* %s\n", escTelegram(report.ServerName)))
	}
	if report.ErrorMessage != "" {
		msg := report.ErrorMessage
		if len(msg) > 200 {
			msg = msg[:200] + "..."
		}
		b.WriteString(fmt.Sprintf("\n⚠️ *Error:* `%s`\n", escTelegram(msg)))
	}
	if report.Description != "" {
		desc := report.Description
		if len(desc) > 500 {
			desc = desc[:500] + "..."
		}
		b.WriteString(fmt.Sprintf("\n💬 *Description:*\n%s\n", escTelegram(desc)))
	}
	if report.URL != "" {
		b.WriteString(fmt.Sprintf("\n🔗 %s", escTelegram(report.URL)))
	}
	if report.ReportFile != "" {
		b.WriteString(fmt.Sprintf("\n📁 `%s`", escTelegram(report.ReportFile)))
	}

	// Post to Telegram — maintenance bot owns this message
	log.Infow("report received",
		"username", report.Username,
		"player_type", report.PlayerType,
		"anime_name", report.AnimeName,
	)
	msgID, err := s.tg.SendMessage(b.String())
	if err != nil {
		log.Errorw("report failed to post to telegram", "error", err)
		return
	}
	log.Infow("report posted to telegram", "message_id", msgID)

	// React with 👀
	if !s.tg.SetReaction(msgID, "👀") {
		log.Warnw("failed to set reaction",
			"emoji", "👀",
			"message_id", msgID,
		)
	}

	// All reports — including footer feedback — are now run through Claude analysis
	// so genuine bugs get diagnosed and (per the risk policy) auto-fixed. The
	// risk gate (decideAutoApply) never auto-implements category=feature, so feature
	// requests still surface an admin "Implement?" button; pure commentary lands as
	// info_only (acknowledged, no fix). report.Category (bug|issue|feature) is passed
	// to Claude so it can classify correctly.

	// Build ClassifiedMessage for Claude analysis
	msg := domain.ClassifiedMessage{
		MessageID: msgID,
		Type:      domain.MessageErrorReport,
		Priority:  domain.P2,
		Text:      b.String(),
		From:      domain.User{Username: report.Username},
		RawJSON:   fmt.Sprintf(`{"report": {"player":"%s","category":"%s","anime":"%s","error":"%s","description":"%s","server":"%s","url":"%s","file":"%s"}}`, report.PlayerType, report.Category, report.AnimeName, report.ErrorMessage, report.Description, report.ServerName, report.URL, report.ReportFile),
	}

	// Invoke Claude
	log.Infow("analyzing report", "message_id", msgID, "category", report.Category)
	analyzeStart := time.Now()
	result, err := s.disp.Analyze(ctx, msg)
	elapsed := time.Since(analyzeStart).Round(time.Second)

	if err != nil {
		log.Errorw("claude report analysis failed",
			"duration", elapsed,
			"error", err,
		)
		if !s.tg.SetReaction(msgID, "💔") {
			log.Warnw("failed to set reaction",
				"emoji", "💔",
				"message_id", msgID,
			)
		}
		s.tg.SendReply(msgID, fmt.Sprintf("*⚠️ Analysis failed*\n%s", truncateForTelegram(err.Error())))
		return
	}

	log.Infow("claude report analysis completed",
		"duration", elapsed,
		"tier", result.Tier,
		"issue_title", result.Issue.Title,
		"category", result.Issue.Category,
	)
	log.Infow("claude diagnosis", "root_cause", truncateForTelegram(result.Diagnosis.RootCause))
	if result.FixPlan != nil {
		log.Infow("claude fix plan",
			"fix_type", result.FixPlan.Type,
			"target", result.FixPlan.Target,
			"description", result.FixPlan.Description,
		)
	}

	if !s.tg.SetReaction(msgID, "👍") {
		log.Warnw("failed to set reaction",
			"emoji", "👍",
			"message_id", msgID,
		)
	}
	s.handleResult(ctx, msg, result)
	s.state.Save()
	log.Infow("report processing complete", "message_id", msgID)
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
	log.Infow("issue created",
		"issue_id", issueID,
		"title", result.Issue.Title,
		"tier", result.Tier,
		"source", source,
		"reporter", reporter,
	)

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

	// Send response to Telegram. Never leave a report with an empty body — a
	// categorised-only message (e.g. a feature request that won't be auto-built)
	// must still get a human-readable acknowledgement, not just a 👍 reaction.
	replyText := result.ReplyMarkdown
	if strings.TrimSpace(replyText) == "" {
		replyText = fmt.Sprintf("*✅ Acknowledged*\n%s — logged and categorised as *%s*. No automatic action was taken.",
			escTelegram(result.Issue.Title), escTelegram(result.Issue.Category))
	}
	if !strings.Contains(replyText, issueID) {
		replyText += fmt.Sprintf("\n\n*Issue:* %s", issueID)
	}

	// sendFunc: reply to existing message, or send standalone if from Grafana API (no message_id)
	sendFunc := func(text string) (int, error) {
		var replyID int
		var sendErr error
		if msg.MessageID > 0 {
			replyID, sendErr = s.tg.SendReply(msg.MessageID, text)
		} else {
			replyID, sendErr = s.tg.SendMessage(text)
		}
		if sendErr != nil {
			log.Errorw("telegram failed to send reply",
				"issue_id", issueID,
				"error", sendErr,
			)
		} else {
			log.Infow("telegram reply sent",
				"message_id", replyID,
				"issue_id", issueID,
			)
		}
		return replyID, sendErr
	}

	switch result.Tier {
	case domain.TierAutoFix, domain.TierEscalate, domain.TierInfoOnly, domain.TierResolved:
		sendFunc(replyText)

	case domain.TierButtonFix:
		// Active auto-fix: when the risk gate allows (see decideAutoApply), apply the
		// fix autonomously instead of waiting for an admin button. The diagnosis is
		// still posted first (no buttons), then applyFix executes + reports the result.
		if apply, label, _ := s.decideAutoApply(msg, result); apply && result.FixPlan != nil {
			replyToID, _ := sendFunc(replyText)
			log.Infow("auto-applying fix",
				"issue_id", issueID,
				"label", label,
				"risk", result.Risk,
				"category", result.Issue.Category,
				"target", result.FixPlan.Target,
			)
			fix := domain.PendingFix{
				IssueID:        issueID,
				ProposedAt:     time.Now().UTC().Format(time.RFC3339),
				FixPlan:        *result.FixPlan,
				AlertMessageID: msg.MessageID,
			}
			s.applyFix(ctx, replyToID, issueID, fix, label)
			return
		}

		buttons := []telegram.InlineButton{
			{Text: "🔧 Apply Fix", CallbackData: "fix:" + issueID},
			{Text: "❌ Dismiss", CallbackData: "dismiss:" + issueID},
		}
		sentMsgID, err := s.tg.SendReplyWithButtons(msg.MessageID, replyText, buttons)
		if err != nil {
			// Never let an approval request vanish silently (AUTO-422 root
			// cause): log the failure and fall back to a buttonless reply so
			// the admin still sees the diagnosis and the issue ID.
			log.Errorw("telegram failed to send approval buttons",
				"issue_id", issueID,
				"error", err,
			)
			sentMsgID, _ = sendFunc(replyText + "\n\n⚠️ Approval buttons could not be delivered — the fix plan is saved; ask the bot to re-send it.")
		} else {
			log.Infow("approval buttons sent",
				"message_id", sentMsgID,
				"issue_id", issueID,
			)
		}
		// Record the pending fix even when Telegram delivery failed — the plan
		// must survive so it can be re-sent or applied manually.
		if result.FixPlan != nil {
			s.state.AddPendingFix(issueID, domain.PendingFix{
				IssueID:           issueID,
				ProposedAt:        time.Now().UTC().Format(time.RFC3339),
				FixPlan:           *result.FixPlan,
				TelegramMessageID: sentMsgID,
				AlertMessageID:    msg.MessageID,
			})
		}

	default:
		// Unknown/empty tier from Claude — never leave a report unanswered.
		// Always send the (acknowledgement) reply so a 👍 reaction is never
		// the only response a user/admin sees.
		log.Warnw("unhandled analysis tier — sending fallback acknowledgement",
			"tier", result.Tier, "issue_id", issueID)
		sendFunc(replyText)
	}
}

// applyFix executes a pending fix (autonomous or admin-approved), threads the
// result under replyToID, and records the outcome. approver labels the source:
// "auto(risk=low)" for autonomous fixes or "@username" for button approvals.
// Used by both the auto-apply path (handleResult) and the manual button path.
func (s *service) applyFix(ctx context.Context, replyToID int, issueID string, fix domain.PendingFix, approver string) {
	if replyToID != 0 {
		s.tg.SetReaction(replyToID, "👀")
	}
	log.Infow("executing fix", "issue_id", issueID, "approver", approver, "target", fix.FixPlan.Target, "fix_type", fix.FixPlan.Type)
	analyzeStart := time.Now()
	result, err := s.disp.ExecuteFix(ctx, fix)
	elapsed := time.Since(analyzeStart).Round(time.Second)

	reply := func(text string) {
		if replyToID != 0 {
			s.tg.SendReply(replyToID, text)
		} else {
			s.tg.SendMessage(text)
		}
	}

	if err != nil {
		log.Errorw("fix execution failed", "issue_id", issueID, "duration", elapsed, "error", err)
		if replyToID != 0 {
			s.tg.SetReaction(replyToID, "💔")
		}
		reply(fmt.Sprintf("*❌ Fix failed* (%s)\n%s", approver, truncateForTelegram(err.Error())))
		return
	}

	log.Infow("fix executed", "issue_id", issueID, "duration", elapsed, "tier", result.Tier)
	for _, a := range result.ActionsTaken {
		log.Infow("fix action", "action", a.Action, "result", a.Result)
	}
	if replyToID != 0 {
		s.tg.SetReaction(replyToID, "👍")
	}
	replyText := result.ReplyMarkdown
	if replyText == "" {
		replyText = fmt.Sprintf("*🔧 Fix Applied* (%s)\n\n*Issue:* %s", approver, issueID)
	}
	reply(replyText)
	log.Infow("telegram fix result sent", "issue_id", issueID)
	s.state.UpdateIssue(issueID, func(issue *domain.Issue) {
		issue.Status = domain.StatusResolved
		issue.Resolution = fmt.Sprintf("Fix applied by %s", approver)
		issue.Actions = append(issue.Actions, result.ActionsTaken...)
	})
	if fix.FixPlan.Target != "" {
		s.state.RecordFix(fix.FixPlan.Target, string(fix.FixPlan.Type))
		s.state.SetCooldown(string(fix.FixPlan.Type), fix.FixPlan.Target, 10*time.Minute)
	}
}

// decideAutoApply implements the risk-gated active auto-fix policy. It returns
// whether to apply the fix without an admin button, a short label for the
// notification, and (when not applying) the reason a button is required.
//
//	low    → always auto-apply
//	medium → auto-apply if it's a real bug OR the message is from an admin
//	high   → never auto-apply (button)
//
// Feature work is never auto-implemented. A per-target loop guard (recently-fixed
// or >2 attempts in 30m) downgrades to a button to prevent runaway fix loops.
func (s *service) decideAutoApply(msg domain.ClassifiedMessage, result *domain.AnalysisResult) (apply bool, label, reason string) {
	if result.Tier != domain.TierButtonFix || result.FixPlan == nil {
		return false, "", "not an applicable button fix"
	}
	// Feature requests always need explicit admin approval — never auto-implement.
	if strings.EqualFold(result.Issue.Category, string(domain.CategoryFeature)) {
		return false, "", "feature requires admin approval"
	}

	switch result.Risk {
	case domain.RiskLow:
		label = "auto(risk=low)"
	case domain.RiskMedium:
		switch {
		case isRealBug(result.Issue.Category):
			label = "auto(risk=medium,bug)"
		case s.isAdminMessage(msg):
			label = "auto(risk=medium,admin)"
		default:
			return false, "", "medium risk: needs admin button (not a bug, not admin-sent)"
		}
	default: // high or unset
		return false, "", "high/unknown risk: needs admin button"
	}

	// Loop guard — never auto-apply the same target in a tight window.
	target := result.FixPlan.Target
	if target != "" {
		if s.state.WasRecentlyFixed(target, 15*time.Minute) {
			return false, "", "loop guard: target fixed within 15m — needs admin button"
		}
		if n := s.state.IncrementFixAttempt(target, target); n > 2 {
			return false, "", fmt.Sprintf("loop guard: %d auto-fix attempts in 30m — needs admin button", n)
		}
	}
	return true, label, ""
}

// isAdminMessage reports whether the message was sent by a configured admin
// (ADMIN_USERNAMES). Grafana alerts and end-user reports are never admin messages.
func (s *service) isAdminMessage(msg domain.ClassifiedMessage) bool {
	if msg.From.Username == "" {
		return false
	}
	for _, a := range s.cfg.Admins {
		if strings.EqualFold(msg.From.Username, a) {
			return true
		}
	}
	return false
}

// isRealBug reports whether an issue category represents a genuine defect/outage
// (eligible for medium-risk auto-fix) as opposed to a feature request, a tuning
// concern, or a false-positive/misconfiguration alert.
func isRealBug(category string) bool {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "bug", "outage", "regression", "stability", "content-quality",
		"degradation", "parser_failure", "data-integrity", "crash":
		return true
	default:
		// feature, latency, capacity, alert_misconfiguration, false_positive_alert, … → not a real bug
		return false
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
		log.Infow("admin approved fix",
			"admin", msg.From.Username,
			"issue_id", issueID,
			"target", fix.FixPlan.Target,
			"fix_type", fix.FixPlan.Type,
		)
		s.tg.AnswerCallbackQuery(msg.CallbackID, "Applying fix...")
		s.applyFix(ctx, replyToID, issueID, *fix, "@"+msg.From.Username)
		s.state.RemovePendingFix(issueID)
		s.state.Save()
		log.Infow("button fix processing complete", "issue_id", issueID)

	case "dismiss":
		log.Infow("admin dismissed issue",
			"admin", msg.From.Username,
			"issue_id", issueID,
		)
		s.tg.AnswerCallbackQuery(msg.CallbackID, "Dismissed")
		s.tg.SetReaction(replyToID, "💔")
		s.state.RemovePendingFix(issueID)
		s.state.UpdateIssue(issueID, func(issue *domain.Issue) {
			issue.Status = domain.StatusWontFix
			issue.Resolution = fmt.Sprintf("Dismissed by @%s", msg.From.Username)
		})
		s.tg.SendReply(fix.AlertMessageID, fmt.Sprintf("*Issue %s dismissed* by @%s", issueID, msg.From.Username))
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
		"*⚠️ Multi-Service Outage Detected*\n\n"+
			"*Affected alerts:* %s\n"+
			"*Count:* 3+ services\n\n"+
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
	log.Infow("claude CLI version", "version", strings.TrimSpace(string(output)))
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

// escTelegram neutralises Telegram Markdown control characters in dynamic
// content. Backticks are swapped for apostrophes (a backslash escape is not
// honoured inside code spans), the rest are backslash-escaped.
func escTelegram(s string) string {
	s = strings.ReplaceAll(s, "`", "'")
	s = strings.ReplaceAll(s, "*", "\\*")
	s = strings.ReplaceAll(s, "_", "\\_")
	s = strings.ReplaceAll(s, "[", "\\[")
	return s
}

func truncateForTelegram(s string) string {
	if len(s) > 500 {
		return s[:497] + "..."
	}
	return s
}
