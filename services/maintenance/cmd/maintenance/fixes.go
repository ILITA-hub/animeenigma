package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/feedback"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/telegram"
)

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

	var issueID string
	if msg.Type == domain.MessageAlertFiring && len(msg.Alerts) > 0 {
		if existing := s.state.FindOpenIssueByAlert(msg.Alerts[0].Name, msg.Alerts[0].Service); existing != nil {
			log.Infow("reusing open issue instead of duplicate",
				"issue", existing.ID,
				"service", msg.Alerts[0].Service,
			)
			s.state.UpdateIssue(existing.ID, func(i *domain.Issue) {
				i.Status = domain.IssueStatus(result.Issue.Status)
			})
			issueID = existing.ID
		} else {
			issueID = s.state.CreateIssue(domain.Issue{
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
		}
	} else {
		issueID = s.state.CreateIssue(domain.Issue{
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
	}
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
	if msg.FeedbackID != "" {
		replyText += fmt.Sprintf("\n*Feedback:* [%s](%s/admin/feedback?id=%s)",
			escTelegram(msg.FeedbackID), s.cfg.FeedbackBaseURL, url.QueryEscape(msg.FeedbackID))
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
		s.fb.TrySetStatus(msg.FeedbackID, feedbackStatusFor(result))

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
		sentMsgID, err := s.tg.SendReplyWithButtons(msg.MessageID, replyText, buttons)
		if err != nil {
			// Never let an approval request vanish silently (AUTO-422 root
			// cause, regressed by 9b26d6a8): log the failure and fall back to
			// a buttonless reply so the admin still sees the diagnosis and the
			// issue ID.
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
				FeedbackID:        msg.FeedbackID,
			})
		}

	default:
		// Unknown/empty tier from Claude — never leave a report unanswered.
		// Always send the (acknowledgement) reply so a 👍 reaction is never
		// the only response a user/admin sees.
		log.Warnw("unhandled analysis tier — sending fallback acknowledgement",
			"tier", result.Tier, "issue_id", issueID)
		sendFunc(replyText)
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

	reply := func(text string) {
		if replyToID != 0 {
			s.tg.SendReply(replyToID, text)
		} else {
			s.tg.SendMessage(text)
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
	// AI believes the fix landed — "ai_done" awaits human verification in the
	// admin feedback board before someone promotes it to "resolved".
	s.fb.TrySetStatus(fix.FeedbackID, feedback.StatusAIDone)
	if s.maint != nil {
		s.maint.PostStatus(context.Background(), "maintenance_bot", true, "auto-fixed "+fix.FixPlan.Target)
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
	// Maintenance gate: when the bot routine is paused, never auto-apply (fall
	// back to the button path; the poller/analysis keep running).
	if s.maint != nil && !s.maint.Enabled(context.Background(), "maintenance_bot") {
		return false, "", "maintenance_bot paused via /admin/policy — needs admin button"
	}
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

	// Cap by the admin-configured max auto-apply risk (none<low<medium). A gate
	// error/unset knob ⇒ "" ⇒ no cap (fail-open).
	if s.maint != nil {
		if ceiling := s.maint.MaxRisk(context.Background(), "maintenance_bot"); ceiling != "" &&
			riskRank(result.Risk) > ceilingRank(ceiling) {
			return false, "", "auto_apply_max_risk ceiling — needs admin button"
		}
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

// riskRank orders domain.FixRisk for the auto_apply_max_risk ceiling
// comparison (low < medium < high/unset).
func riskRank(r domain.FixRisk) int {
	switch r {
	case domain.RiskLow:
		return 1
	case domain.RiskMedium:
		return 2
	default: // RiskHigh or unset
		return 3
	}
}

// ceilingRank orders the auto_apply_max_risk knob value (none < low < medium).
// An empty or unrecognized ceiling ranks above every risk level, i.e. no cap
// (fail-open — matches maintenancegate.MaxRisk's ""-on-error contract).
func ceilingRank(c string) int {
	switch c {
	case "none":
		return 0
	case "low":
		return 1
	case "medium":
		return 2
	default: // "" (gate error/unset) or unknown ⇒ no cap
		return 3
	}
}

// isGrafanaAlert reports whether the message originated from our own Grafana
// alerting webhook — a trusted internal source, which sets From to the
// grafana-webhook bot identity; end-user reports never do. The bare "grafana"
// identity is the retired API poller's; it is still accepted so an alert
// replayed from pre-2026-07-15 persisted state is not mistaken for user input.
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

	if !s.isAdminMessage(msg) {
		// Keep this log: silent rejections hid a broken admin list (AUTO-624).
		log.Warnw("button click rejected: not an admin",
			"username", msg.From.Username,
			"action", action,
			"issue_id", issueID,
		)
		s.tg.AnswerCallbackQuery(msg.CallbackID, "Admin only")
		return
	}

	fix := s.state.GetPendingFix(issueID)
	if fix == nil {
		log.Warnw("button click on expired/handled fix",
			"admin", msg.From.Username,
			"action", action,
			"issue_id", issueID,
		)
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
		s.tg.SendReply(fix.AlertMessageID, fmt.Sprintf("*Issue %s dismissed* by @%s", issueID, msg.From.Username))
	}
}
