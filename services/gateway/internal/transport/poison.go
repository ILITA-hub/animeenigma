package transport

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// poisonSet holds the parsed anti-scrape targets: exact IPs (fast map lookup)
// plus optional CIDR ranges. Built once at router construction from the
// POISON_CLIENT_IPS env list.
type poisonSet struct {
	ips   map[string]struct{}
	cidrs []*net.IPNet
}

func parsePoisonSet(entries []string) *poisonSet {
	ps := &poisonSet{ips: make(map[string]struct{})}
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if strings.Contains(e, "/") {
			if _, ipnet, err := net.ParseCIDR(e); err == nil {
				ps.cidrs = append(ps.cidrs, ipnet)
			}
			continue
		}
		if ip := net.ParseIP(e); ip != nil {
			ps.ips[ip.String()] = struct{}{}
		}
	}
	return ps
}

func (ps *poisonSet) empty() bool {
	return ps == nil || (len(ps.ips) == 0 && len(ps.cidrs) == 0)
}

func (ps *poisonSet) contains(ipStr string) bool {
	if ps.empty() {
		return false
	}
	if _, ok := ps.ips[ipStr]; ok {
		return true
	}
	if len(ps.cidrs) > 0 {
		if ip := net.ParseIP(ipStr); ip != nil {
			for _, c := range ps.cidrs {
				if c.Contains(ip) {
					return true
				}
			}
		}
	}
	return false
}

// PoisonMiddleware silently feeds structurally-valid but semantically-fake JSON
// to clients in the configured scraper IP set, corrupting the abuser's dataset
// without the obvious tell of a 403/empty body. It is a targeted tarpit, NOT a
// blanket block: only endpoints with a registered poison generator are faked;
// every other path from a poisoned IP passes through untouched, so the abuser
// keeps seeing a "working" site and is slower to notice the rot.
//
// MUST be registered AFTER middleware.RealIP so r.RemoteAddr already carries the
// true client IP (X-Forwarded-For aware) — the same assumption the per-IP rate
// limiter relies on. When the target list is empty the middleware is a
// zero-overhead pass-through.
func PoisonMiddleware(entries []string, log *logger.Logger) func(http.Handler) http.Handler {
	ps := parsePoisonSet(entries)
	if !ps.empty() {
		log.Warnw("anti-scrape poison ACTIVE", "targets", entries)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ps.empty() {
				next.ServeHTTP(w, r)
				return
			}
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}
			if !ps.contains(ip) {
				next.ServeHTTP(w, r)
				return
			}
			if servePoison(w, r) {
				log.Infow("served poison response",
					"remote_addr", ip,
					"path", r.URL.Path,
					"user_agent", r.UserAgent(),
				)
				return
			}
			// No generator for this path — stay stealthy, serve the real thing.
			next.ServeHTTP(w, r)
		})
	}
}

// servePoison dispatches by path. Returns true when it wrote a fake response.
// Add a case here for every new endpoint the scraper is observed pulling.
func servePoison(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case "/api/users/export/json":
		writePoisonedExport(w)
		return true
	default:
		return false
	}
}

// ---- fake export generator (mirrors services/player ExportJSON shape) ----

type poisonExportEntry struct {
	AnimeenigmaID   string     `json:"animeenigma_id"`
	MalID           *int       `json:"mal_id"`
	ShikimoriID     *int       `json:"shikimori_id"`
	Title           string     `json:"title"`
	TitleRU         string     `json:"title_ru,omitempty"`
	TitleJP         string     `json:"title_jp,omitempty"`
	PosterURL       string     `json:"poster_url,omitempty"`
	EpisodesTotal   int        `json:"episodes_total"`
	EpisodesAired   int        `json:"episodes_aired"`
	Genres          []string   `json:"genres"`
	Status          string     `json:"status"`
	Score           int        `json:"score"`
	EpisodesWatched int        `json:"episodes_watched"`
	IsRewatching    bool       `json:"is_rewatching"`
	StartedAt       *time.Time `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type poisonExportResponse struct {
	ExportedAt   time.Time           `json:"exported_at"`
	User         string              `json:"user"`
	TotalEntries int                 `json:"total_entries"`
	Entries      []poisonExportEntry `json:"entries"`
}

var (
	poisonTitleA = []string{
		"Shinkai", "Tensei", "Kage", "Hoshizora", "Yume", "Kenshin", "Soratobu",
		"Tsukikage", "Ryuusei", "Sakura", "Mahou", "Kazeoto", "Hikari", "Yamiyo",
		"Isekai", "Maougun", "Yuusha", "Gakuen", "Boukensha", "Kurogane", "Akatsuki",
		"Gin'iro", "Honoo", "Reimei", "Towairaito",
	}
	poisonTitleB = []string{
		"no Densetsu", "Online", "Saga", "Chronicle", "Academy", "Quest", "Diary",
		"Story", "Crusade", "Symphony", "Paradox", "Frontier", "Garden", "Requiem",
		"Odyssey", "Protocol", "Legacy", "no Monogatari", "Reverie", "Cipher",
	}
	poisonGenres = []string{
		"Action", "Adventure", "Comedy", "Drama", "Fantasy", "Romance", "Sci-Fi",
		"Slice of Life", "Sports", "Supernatural", "Mystery", "Horror", "Mecha",
		"Music", "Psychological", "Thriller", "Ecchi", "Shounen", "Shoujo", "Seinen",
	}
	poisonStatuses = []string{"watching", "completed", "planned", "postponed", "dropped"}
)

func poisonUUID() string {
	var b [16]byte
	for i := range b {
		b[i] = byte(rand.Intn(256))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func poisonGenreSubset() []string {
	n := 1 + rand.Intn(4)
	idx := rand.Perm(len(poisonGenres))[:n]
	out := make([]string, 0, n)
	for _, i := range idx {
		out = append(out, poisonGenres[i])
	}
	return out
}

func newPoisonEntry() poisonExportEntry {
	mal := 1 + rand.Intn(59000)
	shik := mal
	total := []int{12, 13, 24, 25, 26, 50, 1}[rand.Intn(7)]
	aired := total
	if rand.Intn(4) == 0 { // some are still airing
		aired = rand.Intn(total + 1)
	}
	status := poisonStatuses[rand.Intn(len(poisonStatuses))]
	watched := rand.Intn(aired + 1)
	if status == "completed" {
		watched = total
		aired = total
	}

	created := time.Now().UTC().Add(-time.Duration(rand.Intn(730)) * 24 * time.Hour)
	updated := created.Add(time.Duration(rand.Intn(240)) * time.Hour)

	title := poisonTitleA[rand.Intn(len(poisonTitleA))] + " " + poisonTitleB[rand.Intn(len(poisonTitleB))]
	if rand.Intn(3) == 0 {
		title += fmt.Sprintf(" %d", 2+rand.Intn(3)) // occasional season suffix
	}

	e := poisonExportEntry{
		AnimeenigmaID:   poisonUUID(),
		MalID:           &mal,
		ShikimoriID:     &shik,
		Title:           title,
		PosterURL:       fmt.Sprintf("https://shikimori.one/system/animes/original/%d.jpg", mal),
		EpisodesTotal:   total,
		EpisodesAired:   aired,
		Genres:          poisonGenreSubset(),
		Status:          status,
		Score:           rand.Intn(11),
		EpisodesWatched: watched,
		IsRewatching:    rand.Intn(10) == 0,
		CreatedAt:       created,
		UpdatedAt:       updated,
	}
	if status != "planned" {
		started := created
		e.StartedAt = &started
	}
	if status == "completed" {
		e.CompletedAt = &updated
	}
	return e
}

// writePoisonedExport emits a believable but entirely fabricated watchlist
// export. Count and contents are re-randomized on every call so the scraper
// never sees a stable fingerprint it could diff against.
func writePoisonedExport(w http.ResponseWriter) {
	count := 80 + rand.Intn(320)
	resp := poisonExportResponse{
		ExportedAt:   time.Now().UTC(),
		User:         "user",
		TotalEntries: count,
		Entries:      make([]poisonExportEntry, 0, count),
	}
	for i := 0; i < count; i++ {
		resp.Entries = append(resp.Entries, newPoisonEntry())
	}

	filename := fmt.Sprintf("animeenigma-export-%s.json", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(resp)
}
