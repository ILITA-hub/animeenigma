package domain

// MessageType represents the classification of a Telegram message.
type MessageType int

const (
	MessageIgnore MessageType = iota
	MessageAlertFiring
	MessageAlertResolved
	MessageErrorReport
	MessageAdminMessage
	MessageUserIssue
	MessageButtonClick
)

// Priority levels for issues.
type Priority int

const (
	P0 Priority = iota // Critical
	P1                 // High
	P2                 // Medium
	P3                 // Low
)

// FixTier determines what action the Go service takes with Claude's response.
type FixTier string

const (
	TierAutoFix   FixTier = "auto_fix"
	TierButtonFix FixTier = "button_fix"
	TierEscalate  FixTier = "escalate"
	TierInfoOnly  FixTier = "info_only"
	TierResolved  FixTier = "resolved"
)

// FixType for fix plans.
type FixType string

const (
	FixRestart    FixType = "restart"
	FixRedeploy   FixType = "redeploy"
	FixDockerPull FixType = "docker_pull"
	FixCodeFix    FixType = "code_fix"
	FixRetryJob   FixType = "retry_job"
)

// IssueStatus lifecycle.
type IssueStatus string

const (
	StatusOpen          IssueStatus = "open"
	StatusInvestigating IssueStatus = "investigating"
	StatusAutoFixed     IssueStatus = "auto_fixed"
	StatusEscalated     IssueStatus = "escalated"
	StatusResolved      IssueStatus = "resolved"
	StatusWontFix       IssueStatus = "wont_fix"
)

// IssueCategory for classification.
type IssueCategory string

const (
	CategoryOutage        IssueCategory = "outage"
	CategoryDegradation   IssueCategory = "degradation"
	CategoryParserFailure IssueCategory = "parser_failure"
	CategoryLatency       IssueCategory = "latency"
	CategoryCapacity      IssueCategory = "capacity"
	CategoryFeature       IssueCategory = "feature"
	CategoryBug           IssueCategory = "bug"
)

// User info from Telegram.
type User struct {
	ID       int64
	Username string
	IsBot    bool
}

// AlertInfo parsed from a Grafana alert message.
type AlertInfo struct {
	Name        string // alertname (e.g., "Service Unreachable")
	Summary     string
	Description string
	Service     string // extracted service name
	Severity    string // "critical" or "warning"
}

// ClassifiedMessage is the result of classifying a Telegram update.
type ClassifiedMessage struct {
	UpdateID     int64
	MessageID    int
	ChatID       int64
	Type         MessageType
	Priority     Priority
	Text         string
	From         User
	Alerts       []AlertInfo // parsed Grafana alerts (can be multiple per message)
	CallbackData string      // for button clicks
	CallbackID   string      // for answering callbacks
	RawJSON      string      // original update JSON for passing to Claude
}

// ClassifiedBatch groups classified messages by type.
type ClassifiedBatch struct {
	Relevant     []ClassifiedMessage
	Resolved     []ClassifiedMessage
	ButtonClicks []ClassifiedMessage
	Ignored      int
}

// AnalysisResult is what Claude returns (structured JSON).
type AnalysisResult struct {
	Tier         FixTier       `json:"tier"`
	Diagnosis    Diagnosis     `json:"diagnosis"`
	ActionsTaken []Action      `json:"actions_taken"`
	FixPlan      *FixPlan      `json:"fix_plan,omitempty"`
	ReplyHTML    string        `json:"reply_html"`
	Issue        IssueInfo     `json:"issue"`
}

type Diagnosis struct {
	RootCause    string `json:"root_cause"`
	Evidence     string `json:"evidence"`
	KnownPattern string `json:"known_pattern,omitempty"`
}

type Action struct {
	Action  string `json:"action"`
	Result  string `json:"result"`
	Details string `json:"details,omitempty"`
}

type FixPlan struct {
	Type         FixType `json:"type"`
	Target       string  `json:"target"`
	Description  string  `json:"description"`
	Context      string  `json:"context,omitempty"`
	Verification string  `json:"verification,omitempty"`
}

type IssueInfo struct {
	Title    string `json:"title"`
	Category string `json:"category"`
	Priority string `json:"priority"`
	Status   string `json:"status"`
}

// ReportRequest is a player error report received via HTTP from the player service.
type ReportRequest struct {
	Username      string `json:"username"`
	UserID        string `json:"user_id"`
	PlayerType    string `json:"player_type"`
	AnimeName     string `json:"anime_name"`
	EpisodeNumber *int   `json:"episode_number,omitempty"`
	ServerName    string `json:"server_name"`
	ErrorMessage  string `json:"error_message"`
	Description   string `json:"description"`
	URL           string `json:"url"`
	ReportFile    string `json:"report_file"`
}

// Issue stored in issues.json.
type Issue struct {
	ID                string      `json:"id"`
	CreatedAt         string      `json:"created_at"`
	ResolvedAt        string      `json:"resolved_at,omitempty"`
	Source            string      `json:"source"`
	Category          IssueCategory `json:"category"`
	Priority          string      `json:"priority"`
	Status            IssueStatus `json:"status"`
	Title             string      `json:"title"`
	Reporter          string      `json:"reporter"`
	TelegramMessageID int         `json:"telegram_message_id"`
	AffectedService   string      `json:"affected_service,omitempty"`
	Actions           []Action    `json:"actions"`
	Resolution        string      `json:"resolution,omitempty"`
}

// IssueDB is the issues.json file structure.
type IssueDB struct {
	LastID int     `json:"last_id"`
	Issues []Issue `json:"issues"`
}

// State is the maintenance-state.json file structure.
type State struct {
	LastUpdateID       int64                       `json:"last_update_id"`
	LastPollAt         string                      `json:"last_poll_at,omitempty"`
	SessionStarted     string                      `json:"session_started,omitempty"`
	ReactionsSupported bool                        `json:"reactions_supported"`
	BotUserID          int64                       `json:"bot_user_id"`
	ActiveAlerts       map[string]ActiveAlert       `json:"active_alerts"`
	Cooldowns          map[string]string            `json:"cooldowns"`
	FixAttemptCounts   map[string]FixAttemptCount   `json:"fix_attempt_counts"`
	LastFixPerService  map[string]LastFix           `json:"last_fix_per_service"`
	PendingFixes       map[string]PendingFix        `json:"pending_fixes"`
}

type ActiveAlert struct {
	AlertUID  string `json:"alert_uid"`
	Service   string `json:"service"`
	FirstSeen string `json:"first_seen"`
	LastSeen  string `json:"last_seen"`
	IssueID   string `json:"issue_id"`
	Status    string `json:"status"`
}

type FixAttemptCount struct {
	Count   int    `json:"count"`
	FirstAt string `json:"first_at"`
}

type LastFix struct {
	Action string `json:"action"`
	At     string `json:"at"`
}

type PendingFix struct {
	IssueID           string  `json:"issue_id"`
	ProposedAt        string  `json:"proposed_at"`
	FixPlan           FixPlan `json:"fix_plan"`
	TelegramMessageID int     `json:"telegram_message_id"`
	AlertMessageID    int     `json:"alert_message_id"`
}
