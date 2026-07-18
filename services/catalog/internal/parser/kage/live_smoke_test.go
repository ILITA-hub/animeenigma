package kage

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLiveKageEndToEnd(t *testing.T) {
	if os.Getenv("KAGE_LIVE") == "" {
		t.Skip("set KAGE_LIVE=1 for the live smoke")
	}
	c := NewClient(Config{Enabled: true, Timeout: 30 * time.Second})
	ctx := context.Background()

	refs, err := c.SearchSeries(ctx, "Sousou no Frieren")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	t.Logf("search refs: %+v", refs)
	if len(refs) == 0 {
		t.Fatal("no search results")
	}

	rels, err := c.GetReleases(ctx, refs[0].ID)
	if err != nil {
		t.Fatalf("releases: %v", err)
	}
	for _, r := range rels {
		t.Logf("release: %+v", r)
	}
	if len(rels) == 0 {
		t.Fatal("no releases")
	}

	payload, name, err := c.DownloadArchive(ctx, rels[0].SrtID)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	t.Logf("archive %q, %d bytes", name, len(payload))

	body, format, err := ExtractEpisode(payload, name, 12)
	if err != nil {
		t.Fatalf("extract ep12: %v", err)
	}
	t.Logf("extracted format=%s bytes=%d head=%q", format, len(body), string(body[:80]))
	if !strings.Contains(string(body), "Dialogue:") && !strings.Contains(string(body), "-->") {
		t.Fatal("extracted body does not look like a subtitle")
	}
}
