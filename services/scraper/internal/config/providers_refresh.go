package config

import (
	"context"
	"time"
)

// Logger is the minimal logging surface the refresher needs (satisfied by
// libs/logger's SugaredLogger via *logger.Logger which embeds *zap.SugaredLogger).
type Logger interface {
	Infow(msg string, kv ...any)
	Warnw(msg string, kv ...any)
}

// StartProvidersRefresher periodically re-fetches provider config from catalog
// and atomically swaps it into target via Replace. Runs until ctx is canceled.
// A failed refresh keeps the last-good config (logged at WARN). No-op if
// catalogURL is empty or interval <= 0.
//
// onRefresh (nil-safe) is invoked AFTER each successful Replace, on the
// refresher goroutine. The scraper wires it to Orchestrator.ApplyStatuses so a
// catalog status change re-gates the failover roster without a restart. It is
// called every poll (idempotent) — ApplyStatuses is a cheap map walk.
func StartProvidersRefresher(ctx context.Context, target *ProvidersConfig, catalogURL string, interval time.Duration, log Logger, onRefresh func()) {
	if catalogURL == "" || interval <= 0 || target == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pc, err := LoadProvidersRemote(ctx, catalogURL, nil, 5*time.Second)
				if err != nil {
					if log != nil {
						log.Warnw("provider config refresh failed; keeping last-good", "error", err)
					}
					continue
				}
				entries := make([]ProviderMeta, 0)
				for _, m := range pc.load() {
					entries = append(entries, m)
				}
				target.Replace(entries)
				if log != nil {
					log.Infow("provider config refreshed", "disabled", target.DisabledNames())
				}
				if onRefresh != nil {
					onRefresh()
				}
			}
		}
	}()
}
