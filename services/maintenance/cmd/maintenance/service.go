package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/maintenancegate"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/config"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/dispatcher"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/feedback"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/grafana"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/state"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/telegram"
)

type service struct {
	tg       *telegram.Client
	gf       *grafana.Client
	disp     *dispatcher.Dispatcher
	state    *state.Manager
	cfg      *config.Config
	workChan chan workItem
	fb       *feedback.Client
	http     *http.Client
	maint    *maintenancegate.Client
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
