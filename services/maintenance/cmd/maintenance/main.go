package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/maintenancegate"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/config"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/dispatcher"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/feedback"
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

	// Send startup message
	tg.SendMessage("🤖 *Maintenance service started*\nMonitoring alerts (Grafana webhook) + user messages (Telegram).")
	log.Infow("startup message sent")

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start polling loop
	svc := &service{
		tg:       tg,
		disp:     disp,
		state:    stateMgr,
		cfg:      cfg,
		workChan: workChan,
		fb:       feedback.NewClient(cfg.PlayerInternalURL, log),
		http:     &http.Client{Timeout: 10 * time.Second},
		maint:    maintenancegate.New(cfg.PolicyURL, 3*time.Second),
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
