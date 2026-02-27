package domain

// ErrorReport represents a user-submitted error report from the video player.
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
	Description string `json:"description"`
	// Browser context
	URL        string `json:"url"`
	UserAgent  string `json:"user_agent"`
	ScreenSize string `json:"screen_size"`
	Language   string `json:"language"`
	Timestamp  string `json:"timestamp"`
	// Verbose diagnostics (JSON strings)
	ConsoleLogs string `json:"console_logs"`
	NetworkLogs string `json:"network_logs"`
	PageHTML    string `json:"page_html"`
}
