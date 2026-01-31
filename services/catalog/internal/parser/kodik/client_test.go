package kodik

import (
	"testing"
)

func TestGetToken(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if client.token == "" {
		t.Fatal("token is empty")
	}

	t.Logf("Got token: %s", client.token)
}

func TestSearchByTitle(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	results, err := client.SearchByTitle("Наруто")
	if err != nil {
		t.Fatalf("failed to search: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("no results found")
	}

	t.Logf("Found %d results", len(results))
	for i, r := range results[:min(3, len(results))] {
		t.Logf("Result %d: %s (Shikimori: %s, Episodes: %d)", i+1, r.Title, r.ShikimoriID, r.EpisodesCount)
	}
}

func TestSearchByShikimoriID(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Search for Naruto (shikimori_id = 20)
	results, err := client.SearchByShikimoriID("20")
	if err != nil {
		t.Fatalf("failed to search: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("no results found")
	}

	t.Logf("Found %d translations for Naruto", len(results))
	for i, r := range results[:min(5, len(results))] {
		t.Logf("Translation %d: %s (ID: %d, Episodes: %d)", i+1, r.Translation.Title, r.Translation.ID, r.EpisodesCount)
	}
}

func TestGetTranslations(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Get translations for Naruto (shikimori_id = 20)
	translations, err := client.GetTranslations("20")
	if err != nil {
		t.Fatalf("failed to get translations: %v", err)
	}

	if len(translations) == 0 {
		t.Fatal("no translations found")
	}

	t.Logf("Found %d unique translations", len(translations))
	for i, tr := range translations {
		t.Logf("Translation %d: %s (ID: %d, Type: %s)", i+1, tr.Title, tr.ID, tr.Type)
	}
}

func TestGetEpisodeLink(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Get episode 1 link for Naruto with AniDUB (609)
	link, err := client.GetEpisodeLink("20", 1, 609)
	if err != nil {
		t.Fatalf("failed to get episode link: %v", err)
	}

	if link == "" {
		t.Fatal("episode link is empty")
	}

	t.Logf("Episode 1 link: %s", link)
}
