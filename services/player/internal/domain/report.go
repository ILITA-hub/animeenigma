package domain

// ErrorReport represents a user-submitted feedback or error report.
// PlayerType="feedback" denotes a generic footer report (no player context);
// other PlayerType values come from the now-removed per-player buttons and
// any future surfaces that want to attach player diagnostics.
type ErrorReport struct {
	// Player context
	PlayerType    string `json:"player_type"`
	AnimeID       string `json:"anime_id"`
	AnimeName     string `json:"anime_name"`
	EpisodeNumber *int   `json:"episode_number,omitempty"`
	ServerName    string `json:"server_name,omitempty"`
	StreamURL     string `json:"stream_url,omitempty"`
	ErrorMessage  string `json:"error_message,omitempty"`
	// User input
	Category string `json:"category,omitempty"` // bug | issue | feature
	Kind     string `json:"kind,omitempty"`     // feedback | todo | idea
	Source   string `json:"source,omitempty"`   // feedback_form | telegram | api | manual
	Description string `json:"description"`
	// Browser context
	URL        string `json:"url"`
	Version    string `json:"version,omitempty"` // VITE_GIT_COMMIT — deployed build SHA the user was running
	UserAgent  string `json:"user_agent"`
	ScreenSize string `json:"screen_size"`
	Language   string `json:"language"`
	Timestamp  string `json:"timestamp"`
	// Verbose diagnostics (JSON strings)
	ConsoleLogs string `json:"console_logs"`
	NetworkLogs string `json:"network_logs"`
	PageHTML    string `json:"page_html"`
}
