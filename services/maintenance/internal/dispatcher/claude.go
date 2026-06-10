package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
)

// Dispatcher invokes Claude Code CLI and parses structured JSON responses.
type Dispatcher struct {
	claudePath  string
	projectRoot string
	promptPath  string
	model       string
	codeModel   string
	timeoutSec  int
	log         *logger.Logger
}

func New(claudePath, projectRoot, promptPath, model, codeModel string, timeoutSec int, log *logger.Logger) *Dispatcher {
	return &Dispatcher{
		claudePath:  claudePath,
		projectRoot: projectRoot,
		promptPath:  promptPath,
		model:       model,
		codeModel:   codeModel,
		timeoutSec:  timeoutSec,
		log:         log,
	}
}

// analysisSchema is the JSON schema for Claude's structured response.
var analysisSchema = `{
  "type": "object",
  "properties": {
    "tier": {"type": "string", "enum": ["auto_fix", "button_fix", "escalate", "info_only", "resolved"]},
    "risk": {"type": "string", "enum": ["low", "medium", "high"]},
    "diagnosis": {
      "type": "object",
      "properties": {
        "root_cause": {"type": "string"},
        "evidence": {"type": "string"},
        "known_pattern": {"type": "string"}
      },
      "required": ["root_cause", "evidence"]
    },
    "actions_taken": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "action": {"type": "string"},
          "result": {"type": "string"},
          "details": {"type": "string"}
        },
        "required": ["action", "result"]
      }
    },
    "fix_plan": {
      "type": "object",
      "properties": {
        "type": {"type": "string", "enum": ["restart", "redeploy", "docker_pull", "code_fix", "retry_job"]},
        "target": {"type": "string"},
        "description": {"type": "string"},
        "context": {"type": "string"},
        "verification": {"type": "string"}
      },
      "required": ["type", "target", "description"]
    },
    "reply_markdown": {"type": "string"},
    "issue": {
      "type": "object",
      "properties": {
        "title": {"type": "string"},
        "category": {"type": "string"},
        "priority": {"type": "string"},
        "status": {"type": "string"}
      },
      "required": ["title", "category", "priority", "status"]
    }
  },
  "required": ["tier", "diagnosis", "reply_markdown", "issue"]
}`

// Analyze invokes Claude to analyze a maintenance message and return a structured response.
func (d *Dispatcher) Analyze(ctx context.Context, msg domain.ClassifiedMessage) (*domain.AnalysisResult, error) {
	prompt := d.buildAnalysisPrompt(msg)
	return d.invoke(ctx, prompt, d.model, nil)
}

// allowedFixTools are the tools Claude can use when executing admin-approved fixes.
// Analysis uses --permission-mode auto (read-only). Fix execution needs writes + deploys.
var allowedFixTools = []string{
	"Edit",
	"Write",
	"Bash(make:*)",
	"Bash(docker:*)",
	"Bash(docker compose:*)",
	"Bash(curl:*)",
	"Bash(go build:*)",
	"Bash(go test:*)",
	"Bash(go mod tidy:*)",
	"Bash(go work sync:*)",
	"Bash(git add:*)",
	"Bash(git commit:*)",
	"Bash(git push:*)",
	"Bash(git checkout:*)",
	"Bash(git revert:*)",
	"Bash(git diff:*)",
	"Bash(git status:*)",
	"Bash(git log:*)",
	"Bash(bun:*)",
	"Bash(bunx:*)",
	// Skill lets the fix run /animeenigma-after-update (lint → redeploy → changelog
	// → commit + push → health) as the canonical apply path for auto-applied fixes.
	"Skill",
	"Bash(docker pull:*)",
	"Bash(docker restart:*)",
	"Bash(docker exec:*)",
	"Bash(docker logs:*)",
	"Bash(golangci-lint:*)",
	"Bash(systemctl restart:*)",
	"Bash(ls:*)",
	"Bash(cat:*)",
	"Bash(head:*)",
	"Bash(echo:*)",
}

// ExecuteFix invokes Claude to execute an admin-approved fix plan.
// Uses --allowedTools for specific write/deploy operations (admin already approved via button).
func (d *Dispatcher) ExecuteFix(ctx context.Context, fix domain.PendingFix) (*domain.AnalysisResult, error) {
	prompt := d.buildFixPrompt(fix)
	model := d.model
	if fix.FixPlan.Type == domain.FixCodeFix {
		model = d.codeModel
	}
	return d.invoke(ctx, prompt, model, allowedFixTools)
}

// invoke runs claude -p with the given prompt and parses the structured output.
// extraAllowedTools: if non-nil, adds --allowedTools for specific write/deploy operations.
func (d *Dispatcher) invoke(ctx context.Context, prompt, model string, extraAllowedTools []string) (*domain.AnalysisResult, error) {
	timeout := time.Duration(d.timeoutSec) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{
		"-p", prompt,
		"--output-format", "json",
		"--json-schema", analysisSchema,
		"--permission-mode", "auto",
		"--no-session-persistence",
		"--model", model,
	}
	if len(extraAllowedTools) > 0 {
		args = append(args, "--allowedTools")
		args = append(args, strings.Join(extraAllowedTools, " "))
	}

	d.log.Infow("invoking claude",
		"model", model,
		"prompt_bytes", len(prompt),
		"timeout", timeout,
	)

	cmd := exec.Command(d.claudePath, args...)
	cmd.Dir = d.projectRoot
	// Create process group so we can kill the entire tree on timeout
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Use pipes with size limit (not CombinedOutput which buffers everything in memory)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start claude: %w", err)
	}
	d.log.Infow("claude started", "pid", cmd.Process.Pid)

	// Read output with 1MB cap
	outputCh := make(chan []byte, 1)
	errCh := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(io.LimitReader(stdout, 1<<20))
		outputCh <- data
	}()
	go func() {
		data, _ := io.ReadAll(io.LimitReader(stderr, 1<<20))
		errCh <- string(data)
	}()

	// Wait for completion or timeout
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-ctx.Done():
		// Timeout: kill the entire process group
		d.log.Errorw("claude timeout — killing",
			"pid", cmd.Process.Pid,
			"timeout", timeout,
		)
		if cmd.Process != nil {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil, fmt.Errorf("claude timed out after %v", timeout)
	case err := <-done:
		output := <-outputCh
		stderrOut := <-errCh
		if err != nil {
			d.log.Errorw("claude exited with error",
				"output_bytes", len(output),
				"stderr_bytes", len(stderrOut),
				"error", err,
			)
			if len(stderrOut) > 0 {
				d.log.Errorw("claude stderr", "stderr", truncate(stderrOut, 1000))
			}
			return nil, fmt.Errorf("claude exited with error: %w, stderr: %s", err, truncate(stderrOut, 500))
		}
		d.log.Infow("claude finished", "output_bytes", len(output))
		return parseResponse(output)
	}
}

// parseResponse extracts the structured_output from Claude's JSON envelope.
func parseResponse(output []byte) (*domain.AnalysisResult, error) {
	if len(output) == 0 {
		return nil, fmt.Errorf("claude returned empty output")
	}

	// Claude --output-format json wraps response in an envelope.
	// The structured data is in "structured_output", NOT "result".
	var envelope struct {
		Result           string          `json:"result"`
		StructuredOutput json.RawMessage `json:"structured_output"`
	}
	if err := json.Unmarshal(output, &envelope); err != nil {
		// Maybe Claude returned raw JSON without envelope (fallback)
		var result domain.AnalysisResult
		if err2 := json.Unmarshal(output, &result); err2 != nil {
			return nil, fmt.Errorf("parse claude output: %w (raw: %s)", err, truncate(string(output), 500))
		}
		return &result, nil
	}

	if len(envelope.StructuredOutput) == 0 {
		// Try parsing from result field as fallback
		if envelope.Result != "" {
			var result domain.AnalysisResult
			if err := json.Unmarshal([]byte(envelope.Result), &result); err != nil {
				return nil, fmt.Errorf("parse result field: %w", err)
			}
			return &result, nil
		}
		return nil, fmt.Errorf("claude returned no structured_output (raw: %s)", truncate(string(output), 500))
	}

	var result domain.AnalysisResult
	if err := json.Unmarshal(envelope.StructuredOutput, &result); err != nil {
		return nil, fmt.Errorf("parse structured_output: %w", err)
	}
	return &result, nil
}

// buildAnalysisPrompt constructs the prompt for analyzing a maintenance message.
func (d *Dispatcher) buildAnalysisPrompt(msg domain.ClassifiedMessage) string {
	// Read the maintenance prompt file
	promptContent := d.readPromptFile()

	var sb strings.Builder
	sb.WriteString(promptContent)
	sb.WriteString("\n\n---\n\n")
	sb.WriteString("## Current Message to Handle\n\n")
	sb.WriteString(fmt.Sprintf("**Type:** %s\n", messageTypeName(msg.Type)))
	sb.WriteString(fmt.Sprintf("**Priority:** %s\n", priorityName(msg.Priority)))
	sb.WriteString(fmt.Sprintf("**From:** @%s (ID: %d, bot: %v)\n", msg.From.Username, msg.From.ID, msg.From.IsBot))
	sb.WriteString(fmt.Sprintf("**Message text:**\n```\n%s\n```\n\n", msg.Text))

	if msg.ForwardedFrom != "" {
		sb.WriteString(fmt.Sprintf("**Forwarded from:** %s\n", msg.ForwardedFrom))
	}
	if msg.ReplyToText != "" {
		sb.WriteString(fmt.Sprintf("**In reply to:** %s\n", msg.ReplyToText))
	}
	if msg.FeedbackID != "" {
		sb.WriteString(fmt.Sprintf("**Feedback entry:** %s (already created in /admin/feedback — its status is driven automatically; do not create another)\n", msg.FeedbackID))
	}

	if downloaded := attachmentLines(msg.Attachments); len(downloaded) > 0 {
		sb.WriteString("\n**Attachments (downloaded to disk — use the Read tool to inspect images/files):**\n")
		for _, line := range downloaded {
			sb.WriteString(line)
		}
	}

	if len(msg.Alerts) > 0 {
		sb.WriteString("**Parsed alerts:**\n")
		for _, a := range msg.Alerts {
			sb.WriteString(fmt.Sprintf("- Alert: %s | Service: %s | Severity: %s\n", a.Name, a.Service, a.Severity))
			sb.WriteString(fmt.Sprintf("  Summary: %s\n", a.Summary))
		}
	}

	sb.WriteString("\nAnalyze this issue. Follow the maintenance guide above. Return your response as structured JSON.")
	return sb.String()
}

// attachmentLines renders downloaded attachments as prompt bullet lines.
func attachmentLines(attachments []domain.Attachment) []string {
	var out []string
	for _, a := range attachments {
		if a.LocalPath == "" {
			continue
		}
		out = append(out, fmt.Sprintf("- %s (%s, %s)\n", a.LocalPath, a.Kind, a.MimeType))
	}
	return out
}

// buildFixPrompt constructs the prompt for executing an admin-approved fix.
func (d *Dispatcher) buildFixPrompt(fix domain.PendingFix) string {
	promptContent := d.readPromptFile()

	var sb strings.Builder
	sb.WriteString(promptContent)
	sb.WriteString("\n\n---\n\n")
	sb.WriteString("## Admin-Approved Fix to Execute\n\n")
	sb.WriteString(fmt.Sprintf("**Issue:** %s\n", fix.IssueID))
	sb.WriteString(fmt.Sprintf("**Fix type:** %s\n", fix.FixPlan.Type))
	sb.WriteString(fmt.Sprintf("**Target:** %s\n", fix.FixPlan.Target))
	sb.WriteString(fmt.Sprintf("**Description:** %s\n", fix.FixPlan.Description))
	sb.WriteString(fmt.Sprintf("**Context:** %s\n", fix.FixPlan.Context))
	if fix.FixPlan.Verification != "" {
		sb.WriteString(fmt.Sprintf("**Verification:** %s\n", fix.FixPlan.Verification))
	}
	sb.WriteString("\nAn admin has approved this fix. Execute it now. Verify the result. Return your response as structured JSON.")
	return sb.String()
}

func (d *Dispatcher) readPromptFile() string {
	path := filepath.Join(d.projectRoot, d.promptPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return "# Maintenance Prompt\nNo maintenance prompt file found. Analyze the issue based on your knowledge of the AnimeEnigma project."
	}
	return string(data)
}

func messageTypeName(t domain.MessageType) string {
	switch t {
	case domain.MessageAlertFiring:
		return "ALERT_FIRING"
	case domain.MessageAlertResolved:
		return "ALERT_RESOLVED"
	case domain.MessageErrorReport:
		return "ERROR_REPORT"
	case domain.MessageAdminMessage:
		return "ADMIN_MESSAGE"
	case domain.MessageUserIssue:
		return "USER_ISSUE"
	case domain.MessageButtonClick:
		return "BUTTON_CLICK"
	default:
		return "IGNORE"
	}
}

func priorityName(p domain.Priority) string {
	switch p {
	case domain.P0:
		return "P0 (Critical)"
	case domain.P1:
		return "P1 (High)"
	case domain.P2:
		return "P2 (Medium)"
	default:
		return "P3 (Low)"
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
