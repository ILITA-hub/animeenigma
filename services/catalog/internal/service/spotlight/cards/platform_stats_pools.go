package cards

import (
	_ "embed"
	"encoding/json"
)

//go:embed platform_stats_jokes.json
var jokesJSON []byte

//go:embed platform_stats_prom.json
var promJSON []byte

// vibe is one canned UXΔ/CDI/MVQ value-set (language-neutral).
type vibe struct {
	UXDelta string `json:"ux_delta"`
	CDI     string `json:"cdi"`
	MVQ     string `json:"mvq"`
}

// jokePool is the hero joke content. Each slice is mixed-language (~80/10/10
// RU/EN/JP); a uniform random pick makes exposure track that ratio.
type jokePool struct {
	Taglines    []string `json:"taglines"`
	UptimeQuips []string `json:"uptime_quips"`
	Vibes       []vibe   `json:"vibes"`
}

// promTile is one allowlisted Prometheus tile query. Metric is wrapped in
// sum(...) by windowPromQL so each tile aggregates across instances.
type promTile struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	Metric  string   `json:"metric"`
	Format  string   `json:"format"`
	Windows []string `json:"windows"`
}

var (
	parsedJokes jokePool
	parsedTiles []promTile
	poolErr     error
)

func init() {
	if poolErr = json.Unmarshal(jokesJSON, &parsedJokes); poolErr != nil {
		return
	}
	poolErr = json.Unmarshal(promJSON, &parsedTiles)
}
