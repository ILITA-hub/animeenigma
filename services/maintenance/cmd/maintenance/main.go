package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/classifier"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/config"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/dispatcher"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/feedback"
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
		cfg.ReportsAuthToken,
	)
	if cfg.ReportsAuthToken == "" {
		log.Warnw("/api/reports is UNAUTHENTICATED — set REPORTS_AUTH_TOKEN to require the X-Maintenance-Token shared secret (player is the sole legitimate caller)")
	} else {
		log.Infow("/api/reports requires X-Maintenance-Token shared secret")
	}
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
	gf := grafana.NewClient(cfg.Grafana.URL, cfg.Grafana.APIUser, cfg.Grafana.APIPass)
	grafanaPollEnabled := cfg.Grafana.APIPass != ""
	// Preflight: verify Grafana connectivity (only when the poll is configured;
	// the alertmanager API needs auth, so without GRAFANA_API_PASS it just 401s).
	if !grafanaPollEnabled {
		log.Infow("grafana reconcile poll disabled — set GRAFANA_API_PASS to enable the safety-net poll (primary alert delivery is via webhook)")
	} else if alerts, err := gf.GetFiringAlerts(); err != nil {
		log.Warnw("grafana check failed (will retry)", "error", err)
	} else {
		log.Infow("grafana connected", "active_alerts", len(alerts))
	}

	// Send startup message
	tg.SendMessage("🤖 <b>Maintenance service started</b>\nMonitoring alerts (Grafana API) + user messages (Telegram).")
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
		fb:       feedback.NewClient(cfg.PlayerInternalURL, log),
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

	tg.SendMessage("🤖 <b>Maintenance service stopping</b>")
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
	fb       *feedback.Client
	mu       sync.Mutex

	// interrupts maps a source message ID (the message wearing the 👀 reaction)
	// → *interruptEntry. Each long-running Claude invocation registers its
	// context.CancelFunc here; an admin aborts by reacting 💔 to that message
	// (detected in the Telegram poller). Entries are removed on completion, on
	// interrupt, or by the TTL sweeper.
	interrupts sync.Map // map[int]*interruptEntry
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

				// Handle message_reaction updates HERE, in the poller, and never
				// queue them to the processor. A 💔 reaction on a message with a
				// live analysis aborts it: the processor goroutine is blocked
				// inside that very analysis, so the cancel must act out-of-band.
				// The bot flips its own 👀→💔 reaction as the silent confirmation.
				kept := updates[:0]
				for _, u := range updates {
					if u.MessageReaction != nil {
						if msgID, ok := isReactionAbort(u, s.tg.BotUserID()); ok && s.tryInterrupt(msgID) {
							s.tg.SetReaction(msgID, heartBreak)
							log.Infow("analysis aborted by admin 💔 reaction", "message_id", msgID)
						}
						continue
					}
					kept = append(kept, u)
				}
				updates = kept

				// Send updates grouped by media_group_id (Telegram album), one
				// group per workItem, so an album reaches ClassifyBatch as a
				// unit and merges into a single relevant message.
				var group []telegram.Update
				flush := func() bool {
					if len(group) == 0 {
						return true
					}
					select {
					case <-ctx.Done():
						return false
					case workChan <- workItem{telegramUpdates: group}:
						group = nil
						return true
					}
				}
				for _, u := range updates {
					if len(group) > 0 {
						prev := group[len(group)-1]
						sameAlbum := u.Message != nil && prev.Message != nil &&
							u.Message.MediaGroupID != "" &&
							u.Message.MediaGroupID == prev.Message.MediaGroupID
						if !sameAlbum && !flush() {
							return
						}
					}
					group = append(group, u)
				}
				if !flush() {
					return
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
		if s.cfg.Grafana.APIPass == "" {
			return // no GRAFANA_API_PASS: poll cannot authenticate; webhook still delivers.
		}
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

	// Goroutine 2b: interrupt-registry TTL sweeper (AUTO-456 safety net).
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				s.sweepInterrupts(now)
			}
		}
	}()

	// Goroutine 3: Processor (sequential, handles both sources)
	for {
		select {
		case <-ctx.Done():
			return
		case work := <-workChan:
			// Separate sources while draining. Telegram updates stay grouped
			// per workItem (one group == one message or one album).
			var telegramGroups [][]telegram.Update
			var grafanaAlerts []domain.ClassifiedMessage
			var reports []domain.ReportRequest
			var webhookEvents []domain.GrafanaWebhookPayload

			if len(work.telegramUpdates) > 0 {
				telegramGroups = append(telegramGroups, work.telegramUpdates)
			}
			grafanaAlerts = append(grafanaAlerts, work.grafanaAlerts...)
			reports = append(reports, work.reports...)
			webhookEvents = append(webhookEvents, work.webhookEvents...)

		drainLoop:
			for {
				select {
				case more := <-workChan:
					if len(more.telegramUpdates) > 0 {
						telegramGroups = append(telegramGroups, more.telegramUpdates)
					}
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

			// Process Telegram update groups one at a time
			for i, group := range telegramGroups {
				select {
				case <-ctx.Done():
					return
				default:
				}
				s.processWork(ctx, workItem{telegramUpdates: group})

				// Cooldown between messages (pacing + allows Grafana interleave)
				if i < len(telegramGroups)-1 {
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
				"<b>✅ Alert Resolved</b>\n<b>Alert:</b> %s (%s)\n<b>Duration:</b> %s\n<b>Issue:</b> %s",
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
		"<b>✅ Alert Resolved</b>\n<b>Alert:</b> %s (%s)\n<b>Duration:</b> %s\n<b>Issue:</b> %s",
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

		// FIRST THING for a human Telegram message: mirror it into the
		// /admin/feedback store (entry + attachments, status=new) before any
		// analysis, so the admin database always has the raw report even if
		// Claude later fails or times out.
		if msg.Type == domain.MessageAdminMessage || msg.Type == domain.MessageUserIssue {
			s.mirrorToFeedback(&msg)
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
			fireHTML := fmt.Sprintf("<b>🔴 Firing</b>\n<b>%s</b>\n%s\n%s",
				escTelegram(alert.Name), escTelegram(alert.Summary), escTelegram(alert.Description))
			if sentID, err := s.tg.SendMessage(fireHTML); err == nil {
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
		s.fb.TrySetStatus(msg.FeedbackID, feedback.StatusInProgress)
		analyzeStart := time.Now()
		result, err := s.runInterruptible(ctx, msg.MessageID, "Analyzing "+messageLabel(msg), func(c context.Context) (*domain.AnalysisResult, error) {
			return s.disp.Analyze(c, msg)
		})
		elapsed := time.Since(analyzeStart).Round(time.Second)

		if errors.Is(err, errInterrupted) {
			// Admin aborted via 💔 — the poller already confirmed. Return the
			// entry to "new" and skip the failure reply (no double message).
			log.Infow("analysis interrupted by admin", "message_id", msg.MessageID, "duration", elapsed)
			s.fb.TrySetStatus(msg.FeedbackID, feedback.StatusNew)
			continue
		}

		if err != nil {
			log.Errorw("claude analysis failed",
				"duration", elapsed,
				"error", err,
			)
			// Analysis never ran to completion — the entry goes back to "new"
			// so it isn't stranded as in_progress in the admin board.
			s.fb.TrySetStatus(msg.FeedbackID, feedback.StatusNew)
			if !s.tg.SetReaction(msg.MessageID, "💔") {
				log.Warnw("failed to set reaction",
					"emoji", "💔",
					"message_id", msg.MessageID,
				)
			}
			s.tg.SendReply(msg.MessageID, fmt.Sprintf("<b>⚠️ Analysis failed</b>\n%s", truncateForTelegram(err.Error())))
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
		b.WriteString(fmt.Sprintf("%s <b>%s</b>\n\n", emoji, label))
	} else {
		b.WriteString("🚨 <b>Player Error Report</b>\n\n")
	}
	b.WriteString(fmt.Sprintf("👤 <b>User:</b> %s (ID: %s)\n", escTelegram(report.Username), escTelegram(report.UserID)))
	if !isFeedback {
		b.WriteString(fmt.Sprintf("🎬 <b>Player:</b> %s\n", escTelegram(report.PlayerType)))
	}
	if report.AnimeName != "" {
		b.WriteString(fmt.Sprintf("📺 <b>Anime:</b> %s\n", escTelegram(report.AnimeName)))
	}
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
	if report.Version != "" {
		// The deployed build (VITE_GIT_COMMIT) the user was running — the bot
		// diagnoses against THIS commit first (see "Version-Anchored Diagnosis"
		// in .claude/maintenance-prompt.md).
		b.WriteString(fmt.Sprintf("\n🏷 <b>Build:</b> <code>%s</code>", escTelegram(report.Version)))
	}
	if report.ReportFile != "" {
		b.WriteString(fmt.Sprintf("\n📁 <code>%s</code>", escTelegram(report.ReportFile)))
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

	// HTTP reports already live in the feedback store — the player service
	// saved the entry before forwarding here. Derive its id from the report
	// file path so the bot can drive that entry's status through the same
	// lifecycle as Telegram-born entries.
	feedbackID := ""
	if report.ReportFile != "" {
		feedbackID = strings.TrimSuffix(filepath.Base(report.ReportFile), ".json")
	}

	// Build ClassifiedMessage for Claude analysis
	msg := domain.ClassifiedMessage{
		MessageID:  msgID,
		Type:       domain.MessageErrorReport,
		Priority:   domain.P2,
		Text:       b.String(),
		From:       domain.User{Username: report.Username},
		FeedbackID: feedbackID,
		RawJSON:    fmt.Sprintf(`{"report": {"player":"%s","category":"%s","anime":"%s","error":"%s","description":"%s","server":"%s","url":"%s","version":"%s","file":"%s"}}`, report.PlayerType, report.Category, report.AnimeName, report.ErrorMessage, report.Description, report.ServerName, report.URL, report.Version, report.ReportFile),
	}

	// Invoke Claude
	log.Infow("analyzing report", "message_id", msgID, "category", report.Category)
	s.fb.TrySetStatus(feedbackID, feedback.StatusInProgress)
	analyzeStart := time.Now()
	result, err := s.runInterruptible(ctx, msgID, "Analyzing report from @"+report.Username, func(c context.Context) (*domain.AnalysisResult, error) {
		return s.disp.Analyze(c, msg)
	})
	elapsed := time.Since(analyzeStart).Round(time.Second)

	if errors.Is(err, errInterrupted) {
		log.Infow("report analysis interrupted by admin", "message_id", msgID, "duration", elapsed)
		s.fb.TrySetStatus(feedbackID, feedback.StatusNew)
		return
	}

	if err != nil {
		log.Errorw("claude report analysis failed",
			"duration", elapsed,
			"error", err,
		)
		s.fb.TrySetStatus(feedbackID, feedback.StatusNew)
		if !s.tg.SetReaction(msgID, "💔") {
			log.Warnw("failed to set reaction",
				"emoji", "💔",
				"message_id", msgID,
			)
		}
		s.tg.SendReply(msgID, fmt.Sprintf("<b>⚠️ Analysis failed</b>\n%s", truncateForTelegram(err.Error())))
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

// mirrorToFeedback creates the /admin/feedback entry for a human Telegram
// message — the FIRST step of handling, before any analysis. It downloads the
// message's attachments from Telegram to the host (so the Claude dispatcher
// can Read them) and uploads each into the entry. All failures are
// WARN-and-continue: the feedback store must never block message handling.
func (s *service) mirrorToFeedback(msg *domain.ClassifiedMessage) {
	username := msg.From.Username
	if username == "" {
		username = fmt.Sprintf("id%d", msg.From.ID)
	}

	meta := map[string]interface{}{
		"message_id": msg.MessageID,
		"chat_id":    msg.ChatID,
	}
	if msg.ForwardedFrom != "" {
		meta["forwarded_from"] = msg.ForwardedFrom
	}
	if msg.ReplyToText != "" {
		meta["reply_to"] = msg.ReplyToText
	}
	if s.isAdminMessage(*msg) {
		meta["from_admin"] = true
	}

	description := msg.Text
	if strings.TrimSpace(description) == "" {
		description = "<no text — see attachments>"
	}

	id, err := s.fb.Create(feedback.CreateRequest{
		Username:     username,
		UserID:       fmt.Sprintf("tg:%d", msg.From.ID),
		PlayerType:   "telegram",
		Category:     classifier.GuessCategory(msg.Text),
		Description:  description,
		Source:       "telegram",
		TelegramMeta: meta,
	})
	if err != nil {
		log.Warnw("feedback entry creation failed — continuing without mirror",
			"message_id", msg.MessageID,
			"error", err,
		)
		return
	}
	msg.FeedbackID = id
	log.Infow("feedback entry created", "feedback_id", id, "message_id", msg.MessageID, "attachments", len(msg.Attachments))

	s.downloadAndAttach(msg)
}

// downloadAndAttach pulls each Telegram attachment to the host attachments
// dir (for Claude) and uploads a copy into the feedback entry (for the admin
// UI). Per-file failures are logged and skipped.
func (s *service) downloadAndAttach(msg *domain.ClassifiedMessage) {
	if len(msg.Attachments) == 0 {
		return
	}
	baseDir := resolveProjectPath(s.cfg.Claude.ProjectRoot, s.cfg.AttachmentsDir)
	dir := filepath.Join(baseDir, msg.FeedbackID)
	if msg.FeedbackID == "" {
		dir = filepath.Join(baseDir, fmt.Sprintf("msg-%d", msg.MessageID))
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		log.Warnw("attachments dir creation failed", "dir", dir, "error", err)
		return
	}

	for i := range msg.Attachments {
		a := &msg.Attachments[i]
		f, err := s.tg.GetFile(a.FileID)
		if err != nil {
			log.Warnw("telegram getFile failed", "file_id", a.FileID, "name", a.FileName, "error", err)
			continue
		}
		data, err := s.tg.DownloadFile(f.FilePath)
		if err != nil {
			log.Warnw("telegram download failed", "file_id", a.FileID, "name", a.FileName, "error", err)
			continue
		}

		name := sanitizeLocalFilename(a.FileName)
		local := filepath.Join(dir, name)
		if err := os.WriteFile(local, data, 0600); err != nil {
			log.Warnw("attachment local write failed", "path", local, "error", err)
		} else {
			a.LocalPath = local
		}

		if msg.FeedbackID != "" {
			stored, err := s.fb.UploadAttachment(msg.FeedbackID, name, data)
			if err != nil {
				log.Warnw("attachment upload to feedback store failed",
					"feedback_id", msg.FeedbackID, "name", name, "error", err)
			} else {
				a.StoredName = stored
			}
		}
		log.Infow("attachment saved",
			"feedback_id", msg.FeedbackID,
			"name", name,
			"kind", a.Kind,
			"bytes", len(data),
			"local_path", a.LocalPath,
		)
	}
}

// sanitizeLocalFilename keeps a filesystem-safe basename, preserving the extension.
func sanitizeLocalFilename(s string) string {
	s = filepath.Base(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, s)
	s = strings.Trim(s, "._")
	if s == "" {
		s = "attachment"
	}
	if len(s) > 128 {
		s = s[len(s)-128:]
	}
	return s
}

// feedbackStatusFor maps an analysis outcome onto the feedback-store
// lifecycle. The bot is an AI, so it NEVER sets "resolved" — that is
// human-only (CLAUDE.md): a person promotes ai_done → resolved after
// verifying. The terminal the bot may set is "ai_done".
//
// Special case: an admin "add to todo / capture for later" request (e.g.
// «Добавь в туду: …») only RECORDS future work — it is NOT done. Such a
// captured backlog item stays a single OPEN ("new") task on the board; the
// bot must not split it into a "done" acknowledgement plus the task itself.
func feedbackStatusFor(result *domain.AnalysisResult) string {
	switch result.Tier {
	case domain.TierAutoFix:
		// Fix applied — AI believes done, human verifies.
		return feedback.StatusAIDone
	case domain.TierInfoOnly, domain.TierResolved:
		if isCapturedTodo(result.Issue.Status) {
			// Recorded-but-not-done future work → one open task.
			return feedback.StatusNew
		}
		// Answered/confirmed — AI believes done, human promotes to resolved.
		return feedback.StatusAIDone
	default: // button_fix (pending), escalate, unknown
		return feedback.StatusInProgress
	}
}

// captureMarkers are the substrings that signal the LLM recorded the request
// as future work to do, rather than a completed action. They are kept tight
// (capture-intent words only) so legitimate completion/lifecycle statuses like
// "auto_fixed", "open", "resolved", "escalated" are never mis-captured.
var captureMarkers = []string{"captured", "backlog", "todo", "to do"}

// isCapturedTodo reports whether the LLM recorded the request as a backlog /
// todo item to be worked on later (issue status the analyzer emits for
// "captured this for the future" rather than a completed action).
//
// The Claude CLI's --json-schema enum is best-effort, not a hard validator, so
// the analyzer may drift to a longer phrase ("captured for later", "backlog
// this item", "TODO: investigate"). We therefore match any capture marker as a
// substring of the normalized status, not just the bare tokens.
func isCapturedTodo(issueStatus string) bool {
	norm := strings.ToLower(strings.TrimSpace(issueStatus))
	for _, m := range captureMarkers {
		if strings.Contains(norm, m) {
			return true
		}
	}
	return false
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

	var attachmentPaths []string
	for _, a := range msg.Attachments {
		if a.LocalPath != "" {
			attachmentPaths = append(attachmentPaths, a.LocalPath)
		}
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
		FeedbackID:        msg.FeedbackID,
		Attachments:       attachmentPaths,
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
	replyHTML := result.ReplyHTML
	if strings.TrimSpace(replyHTML) == "" {
		replyHTML = fmt.Sprintf("<b>✅ Acknowledged</b>\n%s — logged and categorised as <b>%s</b>. No automatic action was taken.",
			escTelegram(result.Issue.Title), escTelegram(result.Issue.Category))
	}
	if !strings.Contains(replyHTML, issueID) {
		replyHTML += fmt.Sprintf("\n\n<b>Issue:</b> %s", issueID)
	}
	if msg.FeedbackID != "" {
		replyHTML += fmt.Sprintf("\n<b>Feedback:</b> <a href=\"%s/admin/feedback?id=%s\">%s</a>",
			s.cfg.FeedbackBaseURL, url.QueryEscape(msg.FeedbackID), escTelegram(msg.FeedbackID))
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
		sendFunc(replyHTML)
		s.fb.TrySetStatus(msg.FeedbackID, feedbackStatusFor(result))

	case domain.TierButtonFix:
		// Active auto-fix: when the risk gate allows (see decideAutoApply), apply the
		// fix autonomously instead of waiting for an admin button. The diagnosis is
		// still posted first (no buttons), then applyFix executes + reports the result.
		if apply, label, _ := s.decideAutoApply(msg, result); apply && result.FixPlan != nil {
			replyToID, _ := sendFunc(replyHTML)
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
				FeedbackID:     msg.FeedbackID,
			}
			s.applyFix(ctx, replyToID, issueID, fix, label)
			return
		}

		// Pending admin approval — work is still open.
		s.fb.TrySetStatus(msg.FeedbackID, feedback.StatusInProgress)

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
				FeedbackID:        msg.FeedbackID,
			})
		}

	default:
		// Unknown/empty tier from Claude — never leave a report unanswered.
		// Always send the (acknowledgement) reply so a 👍 reaction is never
		// the only response a user/admin sees.
		log.Warnw("unhandled analysis tier — sending fallback acknowledgement",
			"tier", result.Tier, "issue_id", issueID)
		sendFunc(replyHTML)
		s.fb.TrySetStatus(msg.FeedbackID, feedback.StatusInProgress)
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
	result, err := s.runInterruptible(ctx, replyToID, "Applying fix "+issueID, func(c context.Context) (*domain.AnalysisResult, error) {
		return s.disp.ExecuteFix(c, fix)
	})
	elapsed := time.Since(analyzeStart).Round(time.Second)

	reply := func(html string) {
		if replyToID != 0 {
			s.tg.SendReply(replyToID, html)
		} else {
			s.tg.SendMessage(html)
		}
	}

	if errors.Is(err, errInterrupted) {
		// Admin aborted the fix mid-flight; poller already confirmed. Leave the
		// feedback entry open and reset the reaction so it's visibly not-done.
		log.Infow("fix execution interrupted by admin", "issue_id", issueID, "duration", elapsed)
		if replyToID != 0 {
			s.tg.SetReaction(replyToID, "💔")
		}
		s.fb.TrySetStatus(fix.FeedbackID, feedback.StatusInProgress)
		return
	}

	if err != nil {
		log.Errorw("fix execution failed", "issue_id", issueID, "duration", elapsed, "error", err)
		if replyToID != 0 {
			s.tg.SetReaction(replyToID, "💔")
		}
		// Work attempted but not done — keep the feedback entry open.
		s.fb.TrySetStatus(fix.FeedbackID, feedback.StatusInProgress)
		reply(fmt.Sprintf("<b>❌ Fix failed</b> (%s)\n%s", approver, truncateForTelegram(err.Error())))
		return
	}

	log.Infow("fix executed", "issue_id", issueID, "duration", elapsed, "tier", result.Tier)
	for _, a := range result.ActionsTaken {
		log.Infow("fix action", "action", a.Action, "result", a.Result)
	}
	if replyToID != 0 {
		s.tg.SetReaction(replyToID, "👍")
	}
	replyHTML := result.ReplyHTML
	if replyHTML == "" {
		replyHTML = fmt.Sprintf("<b>🔧 Fix Applied</b> (%s)\n\n<b>Issue:</b> %s", approver, issueID)
	}
	reply(replyHTML)
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
	// AI believes the fix landed — "ai_done" awaits human verification in the
	// admin feedback board before someone promotes it to "resolved".
	s.fb.TrySetStatus(fix.FeedbackID, feedback.StatusAIDone)
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
		// A medium-risk auto-apply writes code, redeploys, and git-pushes. Only
		// TRUSTED sources may trigger that autonomously — an admin message or a
		// Grafana alert (our own monitoring). Previously a "real bug" auto-applied
		// regardless of source, so unauthenticated end-user report content
		// (error_report / user_issue) classified as a bug could drive a
		// write+deploy+push with no human in the loop (audit #5). End-user-sourced
		// fixes now always require the admin button.
		switch {
		case isRealBug(result.Issue.Category) && (s.isAdminMessage(msg) || isGrafanaAlert(msg)):
			label = "auto(risk=medium,bug)"
		case s.isAdminMessage(msg):
			label = "auto(risk=medium,admin)"
		default:
			return false, "", "medium risk: needs admin button (end-user-sourced, or not admin/Grafana)"
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

// isGrafanaAlert reports whether the message originated from our own Grafana
// alerting (API poller or webhook) — a trusted internal source. Both paths set
// From to the grafana/grafana-webhook bot identity; end-user reports never do.
func isGrafanaAlert(msg domain.ClassifiedMessage) bool {
	if !msg.From.IsBot {
		return false
	}
	switch strings.ToLower(msg.From.Username) {
	case "grafana", "grafana-webhook":
		return true
	}
	return false
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
		s.fb.TrySetStatus(fix.FeedbackID, feedback.StatusNotRelevant)
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

// --- Emoji interrupt protocol (AUTO-456) ---
//
// A long-running Claude invocation is interrupted by the admin reacting 💔 to
// the source message. Detection happens in the Telegram poller (NOT the
// processor), because the processor goroutine is blocked inside the very
// analysis we want to cancel — so an update queued behind it could never reach
// a busy processor in time.
const (
	heartBreak = "\U0001F494" // 💔
	// interruptTTL bounds how long a cancel func lingers in the registry if a
	// computation neither completes nor is interrupted (safety net only —
	// runInterruptible always deregisters on return). Must exceed the claude
	// analysis timeout (1h) so the sweeper never kills a legitimately running
	// analysis before the admin can react 💔.
	interruptTTL = 90 * time.Minute
)

// errInterrupted is returned by runInterruptible when the computation's context
// was cancelled by an admin 💔 reply (as opposed to a timeout or shutdown).
// Callers skip their normal failure reply for it — the poller already sent the
// abort confirmation.
var errInterrupted = errors.New("computation interrupted by admin")

// interruptEntry pairs a computation's cancel func with its expiry for TTL sweep.
type interruptEntry struct {
	cancel  context.CancelFunc
	expires time.Time
}

// registerInterrupt records the cancel func for a 👁️ watch message.
func (s *service) registerInterrupt(watchMsgID int, cancel context.CancelFunc) {
	s.interrupts.Store(watchMsgID, &interruptEntry{
		cancel:  cancel,
		expires: time.Now().Add(interruptTTL),
	})
}

// clearInterrupt removes a registry entry (idempotent). Does NOT call cancel —
// the owning runInterruptible defers its own cancel().
func (s *service) clearInterrupt(watchMsgID int) {
	s.interrupts.Delete(watchMsgID)
}

// tryInterrupt cancels the computation registered under watchMsgID and removes
// the entry. Returns false if nothing was registered (already finished/unknown).
func (s *service) tryInterrupt(watchMsgID int) bool {
	v, ok := s.interrupts.LoadAndDelete(watchMsgID)
	if !ok {
		return false
	}
	if e, ok := v.(*interruptEntry); ok && e.cancel != nil {
		e.cancel()
	}
	return true
}

// sweepInterrupts cancels and drops registry entries past their TTL. A pure
// safety net against a leaked entry; the happy path deregisters on return.
func (s *service) sweepInterrupts(now time.Time) {
	s.interrupts.Range(func(k, v any) bool {
		if e, ok := v.(*interruptEntry); ok && now.After(e.expires) {
			if e.cancel != nil {
				e.cancel()
			}
			s.interrupts.Delete(k)
		}
		return true
	})
}

// runInterruptible runs fn under a cancellable context registered against the
// source message (srcMsgID — the message already wearing the 👀 reaction). An
// admin aborts by reacting 💔 to that message; the Telegram poller detects the
// reaction out-of-band (the processor goroutine is blocked inside fn) and
// cancels this context, SIGKILLing the Claude subprocess. If fn was cancelled
// by an admin interrupt (and not by service shutdown) it returns errInterrupted
// so the caller suppresses its normal failure reply. The "watching" status is
// shown entirely by the 👀→👍/💔 reaction lifecycle the callers drive on
// srcMsgID — runInterruptible sends no message of its own.
func (s *service) runInterruptible(ctx context.Context, srcMsgID int, label string, fn func(context.Context) (*domain.AnalysisResult, error)) (*domain.AnalysisResult, error) {
	aCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if srcMsgID <= 0 {
		// No source message to react to → no abort handle; run anyway.
		return fn(aCtx)
	}

	if log != nil {
		log.Infow("running interruptible analysis", "label", label, "src_msg_id", srcMsgID)
	}
	s.registerInterrupt(srcMsgID, cancel)
	defer s.clearInterrupt(srcMsgID)

	result, err := fn(aCtx)

	// Distinguish an admin interrupt (only aCtx cancelled) from a shutdown
	// (parent ctx cancelled too). Only the former gets the dedicated sentinel.
	if err != nil && aCtx.Err() != nil && ctx.Err() == nil {
		return nil, errInterrupted
	}
	return result, err
}

// messageLabel returns a short human-readable label for a classified message,
// used as the label argument to runInterruptible (which uses it only for a
// status log line). Examples: "alert HighErrorRate", "report from @user", "admin request".
func messageLabel(msg domain.ClassifiedMessage) string {
	switch msg.Type {
	case domain.MessageAlertFiring:
		if len(msg.Alerts) > 0 {
			return "alert " + msg.Alerts[0].Name
		}
		return "alert"
	case domain.MessageAdminMessage:
		return "admin request"
	case domain.MessageUserIssue, domain.MessageErrorReport:
		if msg.From.Username != "" {
			return "report from @" + msg.From.Username
		}
		return "user report"
	default:
		return "message"
	}
}

// reactionsContain reports whether rs has an emoji reaction containing emoji
// (bare-codepoint substring match, tolerant of VS16 variants).
func reactionsContain(rs []telegram.ReactionType, emoji string) bool {
	for _, r := range rs {
		if r.Type == "emoji" && strings.Contains(r.Emoji, emoji) {
			return true
		}
	}
	return false
}

// isReactionAbort reports whether update u is a 💔 reaction added by a human
// (not the bot itself), returning the reacted message ID. The bot runs in a
// single trusted admin chat and abort is fail-safe (it only stops work), so
// this is intentionally not gated beyond the structural match. Whether that
// message has a live analysis is decided by tryInterrupt at the call site.
func isReactionAbort(u telegram.Update, botID int64) (msgID int, ok bool) {
	r := u.MessageReaction
	if r == nil {
		return 0, false
	}
	if r.User != nil && r.User.ID == botID { // belt-and-suspenders: Telegram never delivers bot-set reactions back, so this guard is defense-in-depth
		return 0, false
	}
	if !reactionsContain(r.NewReaction, heartBreak) {
		return 0, false
	}
	return r.MessageID, true
}
