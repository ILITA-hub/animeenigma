package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
)

const maxReportBodySize = 2 * 1024 * 1024 // 2MB

// allowedPlayerTypes is a whitelist of valid player types for report filenames.
var allowedPlayerTypes = map[string]bool{
	"hianime": true, "consumet": true, "kodik": true, "animelib": true,
}

type ReportHandler struct {
	log             *logger.Logger
	telegramToken   string
	telegramChatID  string
	telegramEnabled bool
	reportsDir      string
}

func NewReportHandler(log *logger.Logger, telegramToken, telegramChatID, reportsDir string) *ReportHandler {
	enabled := telegramToken != "" && telegramChatID != ""
	if !enabled {
		log.Warnw("telegram notifications disabled for error reports", "reason", "missing TELEGRAM_BOT_TOKEN or TELEGRAM_ADMIN_CHAT_ID")
	} else {
		log.Infow("telegram notifications enabled for error reports", "chat_id", telegramChatID)
	}

	// Ensure reports directory exists
	if reportsDir != "" {
		if err := os.MkdirAll(reportsDir, 0755); err != nil {
			log.Errorw("failed to create reports directory", "path", reportsDir, "error", err)
		} else {
			log.Infow("error reports will be saved to disk", "path", reportsDir)
		}
	}

	return &ReportHandler{
		log:             log,
		telegramToken:   telegramToken,
		telegramChatID:  telegramChatID,
		telegramEnabled: enabled,
		reportsDir:      reportsDir,
	}
}

// SubmitReport handles user-submitted error reports from video players.
func (h *ReportHandler) SubmitReport(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	// Limit body size to prevent abuse
	r.Body = http.MaxBytesReader(w, r.Body, maxReportBodySize)

	var report domain.ErrorReport
	if err := httputil.Bind(r, &report); err != nil {
		h.log.Errorw("failed to bind error report", "error", err, "user_id", claims.UserID)
		httputil.BadRequest(w, "invalid report data")
		return
	}

	// Truncate page HTML if still too large after binding
	if len(report.PageHTML) > 500*1024 {
		report.PageHTML = report.PageHTML[:500*1024] + "...[truncated]"
	}

	h.log.Infow("error report received",
		"user_id", claims.UserID,
		"username", claims.Username,
		"player_type", report.PlayerType,
		"anime_id", report.AnimeID,
		"anime_name", report.AnimeName,
		"episode_number", report.EpisodeNumber,
		"server_name", report.ServerName,
		"stream_url", report.StreamURL,
		"error_message", report.ErrorMessage,
		"description", report.Description,
		"page_url", report.URL,
		"user_agent", report.UserAgent,
		"screen_size", report.ScreenSize,
		"language", report.Language,
		"timestamp", report.Timestamp,
		"console_logs_size", len(report.ConsoleLogs),
		"network_logs_size", len(report.NetworkLogs),
		"page_html_size", len(report.PageHTML),
	)

	// Save report to disk
	reportFile := h.saveReportToDisk(claims, &report)

	// Send Telegram notification to admin
	if h.telegramEnabled {
		go h.sendTelegramNotification(claims, &report, reportFile)
	}

	httputil.OK(w, map[string]string{"status": "received"})
}

// saveReportToDisk persists the full report as a JSON file.
func (h *ReportHandler) saveReportToDisk(claims *authz.Claims, report *domain.ErrorReport) string {
	if h.reportsDir == "" {
		return ""
	}

	ts := time.Now().UTC().Format("2006-01-02T15-04-05")
	username := claims.Username
	if username == "" {
		username = claims.UserID
	}
	// Sanitize username for filename
	username = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, username)

	// Validate player type against allowlist to prevent path traversal
	if !allowedPlayerTypes[report.PlayerType] {
		h.log.Warnw("invalid player type in report", "player_type", report.PlayerType)
		return ""
	}

	filename := fmt.Sprintf("%s_%s_%s.json", ts, username, report.PlayerType)
	filePath := filepath.Join(h.reportsDir, filename)

	// Build full report with user info
	fullReport := map[string]interface{}{
		"user_id":        claims.UserID,
		"username":       claims.Username,
		"player_type":    report.PlayerType,
		"anime_id":       report.AnimeID,
		"anime_name":     report.AnimeName,
		"episode_number": report.EpisodeNumber,
		"server_name":    report.ServerName,
		"stream_url":     report.StreamURL,
		"error_message":  report.ErrorMessage,
		"description":    report.Description,
		"url":            report.URL,
		"user_agent":     report.UserAgent,
		"screen_size":    report.ScreenSize,
		"language":       report.Language,
		"timestamp":      report.Timestamp,
		"console_logs":   json.RawMessage(report.ConsoleLogs),
		"network_logs":   json.RawMessage(report.NetworkLogs),
		"page_html":      report.PageHTML,
	}

	data, err := json.MarshalIndent(fullReport, "", "  ")
	if err != nil {
		h.log.Errorw("failed to marshal report", "error", err)
		return ""
	}

	if err := os.WriteFile(filePath, data, 0600); err != nil {
		h.log.Errorw("failed to write report file", "path", filePath, "error", err)
		return ""
	}

	h.log.Infow("report saved to disk", "path", filePath, "size", len(data))
	return filePath
}

func (h *ReportHandler) sendTelegramNotification(claims *authz.Claims, report *domain.ErrorReport, reportFile string) {
	var b strings.Builder

	b.WriteString("🚨 <b>Player Error Report</b>\n\n")

	// User info
	b.WriteString(fmt.Sprintf("👤 <b>User:</b> %s (ID: %s)\n", escapeHTML(claims.Username), escapeHTML(claims.UserID)))

	// Player & content
	b.WriteString(fmt.Sprintf("🎬 <b>Player:</b> %s\n", escapeHTML(report.PlayerType)))
	b.WriteString(fmt.Sprintf("📺 <b>Anime:</b> %s\n", escapeHTML(report.AnimeName)))

	if report.EpisodeNumber != nil {
		b.WriteString(fmt.Sprintf("📋 <b>Episode:</b> %d\n", *report.EpisodeNumber))
	}
	if report.ServerName != "" {
		b.WriteString(fmt.Sprintf("🖥 <b>Server:</b> %s\n", escapeHTML(report.ServerName)))
	}

	// Error
	if report.ErrorMessage != "" {
		msg := report.ErrorMessage
		if len(msg) > 200 {
			msg = msg[:200] + "..."
		}
		b.WriteString(fmt.Sprintf("\n⚠️ <b>Error:</b> <code>%s</code>\n", escapeHTML(msg)))
	}

	// User description
	if report.Description != "" {
		desc := report.Description
		if len(desc) > 500 {
			desc = desc[:500] + "..."
		}
		b.WriteString(fmt.Sprintf("\n💬 <b>Description:</b>\n%s\n", escapeHTML(desc)))
	}

	// Browser info
	b.WriteString(fmt.Sprintf("\n🌐 %s | %s", escapeHTML(report.ScreenSize), escapeHTML(report.Language)))

	// Page URL
	if report.URL != "" {
		b.WriteString(fmt.Sprintf("\n🔗 %s", escapeHTML(report.URL)))
	}

	// Report file reference
	if reportFile != "" {
		b.WriteString(fmt.Sprintf("\n\n📁 <code>%s</code>", escapeHTML(reportFile)))
	}

	text := b.String()

	// Send via Telegram Bot API
	resp, err := http.PostForm(
		fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", h.telegramToken),
		url.Values{
			"chat_id":    {h.telegramChatID},
			"text":       {text},
			"parse_mode": {"HTML"},
		},
	)
	if err != nil {
		h.log.Errorw("failed to send telegram notification", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		h.log.Errorw("telegram API error",
			"status", resp.StatusCode,
			"response", string(body),
		)
	}
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
