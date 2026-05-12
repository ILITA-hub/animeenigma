package gogoanime

import (
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// Goldens live under services/scraper/testdata/gogoanime/ — resolved by the
// goldenPath helper in client_test.go (same package).

// TestSearchResult_GoldenParse exercises the search-result row DTO against
// the captured search_attack_on_titan.html golden (Plan 18-01 Task 3).
// SCRAPER-9ANI-01: each <p class="name"><a href="/category/<slug>"> entry
// yields one searchResult with Slug + Title populated.
func TestSearchResult_GoldenParse(t *testing.T) {
	p := goldenPath(t, "search_attack_on_titan.html")
	f, err := os.Open(p)
	if err != nil {
		t.Fatalf("open golden: %v", err)
	}
	defer f.Close()
	doc, err := goquery.NewDocumentFromReader(f)
	if err != nil {
		t.Fatalf("parse golden: %v", err)
	}

	rows := make([]searchResult, 0, 16)
	doc.Find("p.name a[href^='/category/']").Each(func(_ int, sel *goquery.Selection) {
		href, _ := sel.Attr("href")
		slug := strings.TrimPrefix(href, "/category/")
		slug = strings.TrimSuffix(slug, "/")
		title := strings.TrimSpace(sel.Text())
		if slug == "" || title == "" {
			return
		}
		rows = append(rows, searchResult{Slug: slug, Title: title})
	})
	if len(rows) < 5 {
		t.Fatalf("len(rows) = %d; want >= 5 (search_attack_on_titan should produce many)", len(rows))
	}
	// Spot-check the first row resembles what we expect.
	sawAOT := false
	for _, r := range rows {
		if r.Slug == "attack-on-titan" && strings.EqualFold(r.Title, "Attack on Titan") {
			sawAOT = true
			break
		}
	}
	if !sawAOT {
		t.Errorf("expected an entry with Slug=attack-on-titan, Title=Attack on Titan; got %v", rows[:min(5, len(rows))])
	}
}

// TestEpisodeRow_GoldenParse exercises the episode-row DTO against the
// captured category_one_piece.html golden. SCRAPER-9ANI-02: each
// <a href="/<slug>-episode-N"> yields one episodeRow with Number parsed
// from the trailing -episode-<N> path segment.
func TestEpisodeRow_GoldenParse(t *testing.T) {
	p := goldenPath(t, "category_one_piece.html")
	f, err := os.Open(p)
	if err != nil {
		t.Fatalf("open golden: %v", err)
	}
	defer f.Close()
	doc, err := goquery.NewDocumentFromReader(f)
	if err != nil {
		t.Fatalf("parse golden: %v", err)
	}

	re := regexp.MustCompile(`^/(.*)-episode-(\d+)$`)
	rows := make([]episodeRow, 0, 1024)
	seen := make(map[int]bool)
	doc.Find(`a[href*="-episode-"]`).Each(func(_ int, sel *goquery.Selection) {
		href, _ := sel.Attr("href")
		m := re.FindStringSubmatch(href)
		if len(m) != 3 {
			return
		}
		n, err := strconv.Atoi(m[2])
		if err != nil {
			return
		}
		if seen[n] {
			return
		}
		seen[n] = true
		rows = append(rows, episodeRow{
			Number:  n,
			URLSlug: strings.TrimPrefix(href, "/"),
			Title:   strings.TrimSpace(sel.Text()),
		})
	})
	if len(rows) < 100 {
		t.Fatalf("len(rows) = %d; want >= 100 (One Piece has > 1000 episodes; the canonical anitaku.to mirror surfaces them all inline)", len(rows))
	}
	// Spot-check the lowest-numbered row.
	hasEp1 := false
	for _, r := range rows {
		if r.Number == 1 {
			hasEp1 = true
			break
		}
	}
	if !hasEp1 {
		t.Errorf("expected an episode with Number=1 in golden; rows[0:3]=%v", rows[:min(3, len(rows))])
	}
}

// TestServerRow_GoldenParse exercises the server-row DTO against the
// captured one_piece_episode_1.html golden. SCRAPER-9ANI-03: each
// <ul class="muti_link"> <li><a data-video> within <div class="anime_muti_link">
// yields one serverRow with Name (visible label) + EmbedURL (raw data-video,
// already absolute on anitaku.to but normalize-tolerant per RESEARCH.md).
func TestServerRow_GoldenParse(t *testing.T) {
	p := goldenPath(t, "one_piece_episode_1.html")
	f, err := os.Open(p)
	if err != nil {
		t.Fatalf("open golden: %v", err)
	}
	defer f.Close()
	doc, err := goquery.NewDocumentFromReader(f)
	if err != nil {
		t.Fatalf("parse golden: %v", err)
	}

	rows := make([]serverRow, 0, 8)
	doc.Find(".anime_muti_link a[data-video]").Each(func(_ int, sel *goquery.Selection) {
		dv, _ := sel.Attr("data-video")
		if dv == "" {
			return
		}
		// Protocol-relative tolerance.
		if strings.HasPrefix(dv, "//") {
			dv = "https:" + dv
		}
		u, perr := url.Parse(dv)
		if perr != nil || u.Hostname() == "" {
			return
		}
		// Server label = anchor text minus the trailing "Choose this server"
		// helper-span and the leading icon.
		label := strings.TrimSpace(sel.Text())
		label = strings.TrimSpace(strings.TrimSuffix(label, "Choose this server"))
		rows = append(rows, serverRow{Name: label, EmbedURL: dv})
	})
	if len(rows) < 4 {
		t.Fatalf("len(rows) = %d; want >= 4; golden has at least HD-1, HD-2, StreamHG, Earnvids", len(rows))
	}
	// All EmbedURLs must parse with a non-empty host (the parser ran already
	// but assert defensively to anchor the contract).
	for i, r := range rows {
		u, err := url.Parse(r.EmbedURL)
		if err != nil {
			t.Errorf("rows[%d] EmbedURL = %q is not a parseable URL: %v", i, r.EmbedURL, err)
			continue
		}
		if u.Hostname() == "" {
			t.Errorf("rows[%d] EmbedURL = %q has empty host", i, r.EmbedURL)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
