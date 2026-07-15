package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/classifier"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/feedback"
)

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

	// Defer suppressed alerts (e.g. transient streaming/gateway HLS-proxy
	// 5xx bursts): drop silently here, before the multi-service triage below,
	// so a suppressed alert can never inflate the outage escalation count nor
	// be named in escalateBatch's Telegram page. Data is preserved in
	// Prometheus + ClickHouse events; this only stops the Telegram page +
	// Claude run. Keyed alertName:service.
	batch.Relevant = s.dropSuppressedAlerts(batch.Relevant)

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
			fireMsg := fmt.Sprintf("*🔴 Firing*\n*%s*\n%s\n%s",
				escTelegram(alert.Name), escTelegram(alert.Summary), escTelegram(alert.Description))
			// For scraper/parser alerts, append a one-line summary of which EN
			// failover providers are currently unhealthy and why, so the on-call
			// sees the likely culprit without digging into Grafana or the scraper.
			if isScraperAlert(alert) {
				if line := s.scraperProviderFaultLine(); line != "" {
					fireMsg += "\n" + line
				}
			}
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
	if report.Version != "" {
		// The deployed build (VITE_GIT_COMMIT) the user was running — the bot
		// diagnoses against THIS commit first (see "Version-Anchored Diagnosis"
		// in .claude/maintenance-prompt.md).
		b.WriteString(fmt.Sprintf("\n🏷 *Build:* `%s`", escTelegram(report.Version)))
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
