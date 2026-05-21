// Package main is the notifications service entrypoint (port 8090).
//
// v1.0 Notifications Engine — workstream notifications, Phase 1.
// Task 1 ships only the scaffold; Task 3 fleshes this file out with
// repo/service/handler wiring + AutoMigrate + EnsureIndexes + graceful
// shutdown. The intermediate version below only validates that the
// module compiles and the scaffold is structurally sound.
package main

import (
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/config"
)

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	log.Infow("notifications scaffold booted (Task 1 placeholder)",
		"address", cfg.Server.Address(),
		"db_name", cfg.Database.Database,
	)
}
