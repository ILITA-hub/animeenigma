package handler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/go-chi/chi/v5"
)

// OpenGraphHandler renders HTML pages with Open Graph meta tags for social media crawlers.
type OpenGraphHandler struct {
	catalogURL string
	siteURL    string
	client     *http.Client
	tmpl       *template.Template
	log        *logger.Logger
	cache      sync.Map // map[string]*ogCacheEntry
}

type ogCacheEntry struct {
	html      []byte
	expiresAt time.Time
}

// ogAnime is a minimal struct for parsing the catalog API response.
type ogAnime struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	NameRU        string   `json:"name_ru"`
	NameJP        string   `json:"name_jp"`
	Description   string   `json:"description"`
	PosterURL     string   `json:"poster_url"`
	Score         float64  `json:"score"`
	Year          int      `json:"year"`
	Status        string   `json:"status"`
	EpisodesCount int      `json:"episodes_count"`
	EpisodesAired int      `json:"episodes_aired"`
	Genres        []ogGenre `json:"genres"`
}

type ogGenre struct {
	Name   string `json:"name"`
	NameRU string `json:"name_ru"`
}

type ogTemplateData struct {
	Title        string
	Description  string
	ImageURL     string
	CanonicalURL string
	SiteName     string
	OGType       string
}

var (
	htmlTagRe    = regexp.MustCompile(`<[^>]*>`)
	bbcodeTagRe  = regexp.MustCompile(`\[[^\]]*\]`)
)

const ogHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>{{.Title}}</title>
<meta name="title" content="{{.Title}}">
<meta name="description" content="{{.Description}}">
<meta property="og:type" content="{{.OGType}}">
<meta property="og:url" content="{{.CanonicalURL}}">
<meta property="og:title" content="{{.Title}}">
<meta property="og:description" content="{{.Description}}">
<meta property="og:image" content="{{.ImageURL}}">
<meta property="og:image:width" content="350">
<meta property="og:image:height" content="500">
<meta property="og:site_name" content="{{.SiteName}}">
<meta name="twitter:card" content="summary">
<meta name="twitter:title" content="{{.Title}}">
<meta name="twitter:description" content="{{.Description}}">
<meta name="twitter:image" content="{{.ImageURL}}">
<link rel="canonical" href="{{.CanonicalURL}}">
<meta http-equiv="refresh" content="0;url={{.CanonicalURL}}">
</head>
<body>
<p>Redirecting to <a href="{{.CanonicalURL}}">{{.Title}}</a>...</p>
</body>
</html>`

func NewOpenGraphHandler(catalogURL, siteURL string, log *logger.Logger) *OpenGraphHandler {
	tmpl := template.Must(template.New("og").Parse(ogHTMLTemplate))

	h := &OpenGraphHandler{
		catalogURL: catalogURL,
		siteURL:    siteURL,
		log:        log,
		tmpl:       tmpl,
		client: &http.Client{
			Timeout: 3 * time.Second,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   2 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 5,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}

	// Background cache cleanup every 15 minutes
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			h.cache.Range(func(key, value any) bool {
				if entry, ok := value.(*ogCacheEntry); ok && now.After(entry.expiresAt) {
					h.cache.Delete(key)
				}
				return true
			})
		}
	}()

	return h
}

// ServeAnime renders OG meta tags for an anime detail page.
func (h *OpenGraphHandler) ServeAnime(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		h.serveDefault(w)
		return
	}

	// Check cache
	if v, ok := h.cache.Load(animeID); ok {
		entry := v.(*ogCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(entry.html)
			return
		}
		h.cache.Delete(animeID)
	}

	// Fetch anime from catalog
	anime, err := h.fetchAnime(animeID)
	if err != nil {
		h.log.Warnw("og: catalog API call failed, using defaults",
			"anime_id", animeID,
			"error", err,
		)
		h.serveDefault(w)
		return
	}

	data := h.buildTemplateData(anime)
	html := h.renderTemplate(data)

	// Cache for 1 hour
	h.cache.Store(animeID, &ogCacheEntry{
		html:      html,
		expiresAt: time.Now().Add(1 * time.Hour),
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(html)
}

// ServeHome renders OG meta tags for the home page.
func (h *OpenGraphHandler) ServeHome(w http.ResponseWriter, r *http.Request) {
	data := ogTemplateData{
		Title:        "AnimeEnigma — Anime Streaming Platform",
		Description:  "Watch anime for free with subtitles. Stream the latest episodes and classic series in English and Russian.",
		ImageURL:     h.siteURL + "/logo.png",
		CanonicalURL: h.siteURL + "/",
		SiteName:     "AnimeEnigma",
		OGType:       "website",
	}
	html := h.renderTemplate(data)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(html)
}

func (h *OpenGraphHandler) fetchAnime(animeID string) (*ogAnime, error) {
	resp, err := h.client.Get(h.catalogURL + "/api/anime/" + animeID)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog returned %d", resp.StatusCode)
	}

	var result struct {
		Success bool    `json:"success"`
		Data    ogAnime `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode failed: %w", err)
	}
	if !result.Success {
		return nil, fmt.Errorf("catalog returned success=false")
	}

	return &result.Data, nil
}

func (h *OpenGraphHandler) buildTemplateData(anime *ogAnime) ogTemplateData {
	// Title: EN / RU combined
	title := anime.Name
	if title != "" && anime.NameRU != "" && anime.NameRU != title {
		title = title + " / " + anime.NameRU
	} else if title == "" {
		title = anime.NameRU
	}
	if title == "" {
		title = anime.NameJP
	}
	if title == "" {
		title = "Anime"
	}

	// Description: metadata line + synopsis
	metaParts := []string{}
	if anime.Score > 0 {
		metaParts = append(metaParts, fmt.Sprintf("★ %.2f", anime.Score))
	}
	if anime.EpisodesCount > 0 {
		if anime.Status == "ongoing" && anime.EpisodesAired > 0 && anime.EpisodesAired < anime.EpisodesCount {
			metaParts = append(metaParts, fmt.Sprintf("%d/%d ep", anime.EpisodesAired, anime.EpisodesCount))
		} else {
			metaParts = append(metaParts, fmt.Sprintf("%d ep", anime.EpisodesCount))
		}
	}
	if anime.Year > 0 {
		metaParts = append(metaParts, fmt.Sprintf("%d", anime.Year))
	}
	if len(anime.Genres) > 0 {
		genreNames := make([]string, 0, len(anime.Genres))
		for _, g := range anime.Genres {
			if g.Name != "" {
				genreNames = append(genreNames, g.Name)
			}
		}
		if len(genreNames) > 0 {
			metaParts = append(metaParts, strings.Join(genreNames, ", "))
		}
	}

	metaLine := strings.Join(metaParts, " · ")

	synopsis := sanitizeHTML(anime.Description)
	synopsis = truncateDescription(synopsis, 160)

	var description string
	if metaLine != "" && synopsis != "" {
		description = metaLine + "\n\n" + synopsis
	} else if metaLine != "" {
		description = metaLine
	} else if synopsis != "" {
		description = synopsis
	} else {
		description = "Watch on AnimeEnigma"
	}

	imageURL := anime.PosterURL
	if imageURL == "" {
		imageURL = h.siteURL + "/logo.png"
	}

	return ogTemplateData{
		Title:        title,
		Description:  description,
		ImageURL:     imageURL,
		CanonicalURL: h.siteURL + "/anime/" + anime.ID,
		SiteName:     "AnimeEnigma",
		OGType:       "video.tv_show",
	}
}

func (h *OpenGraphHandler) serveDefault(w http.ResponseWriter) {
	data := ogTemplateData{
		Title:        "AnimeEnigma — Anime Streaming Platform",
		Description:  "Watch anime for free with subtitles. Stream the latest episodes and classic series.",
		ImageURL:     h.siteURL + "/logo.png",
		CanonicalURL: h.siteURL + "/",
		SiteName:     "AnimeEnigma",
		OGType:       "website",
	}
	html := h.renderTemplate(data)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(html)
}

func (h *OpenGraphHandler) renderTemplate(data ogTemplateData) []byte {
	var buf strings.Builder
	if err := h.tmpl.Execute(&buf, data); err != nil {
		h.log.Errorw("og: template execution failed", "error", err)
		return []byte("<!DOCTYPE html><html><head><title>AnimeEnigma</title></head><body></body></html>")
	}
	return []byte(buf.String())
}

// sanitizeHTML strips HTML and BBCode tags from a string.
func sanitizeHTML(s string) string {
	s = htmlTagRe.ReplaceAllString(s, "")
	s = bbcodeTagRe.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

// truncateDescription truncates text to maxLen characters at a sentence or word boundary.
func truncateDescription(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	// Try to cut at a sentence boundary
	truncated := s[:maxLen]
	if idx := strings.LastIndexAny(truncated, ".!?"); idx > maxLen/2 {
		return strings.TrimSpace(truncated[:idx+1])
	}

	// Fall back to word boundary
	if idx := strings.LastIndex(truncated, " "); idx > maxLen/2 {
		return strings.TrimSpace(truncated[:idx]) + "..."
	}

	return truncated + "..."
}
