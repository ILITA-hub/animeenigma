package shikimori

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// bbcodeTagRe matches a single Shikimori bbcode tag, e.g. "[character=196826]"
// or "[/character]" or "[b]" — anything bracketed with no nested brackets.
// Removing every tag while keeping inner text turns
// "[character=1]Name[/character]" into "Name".
var bbcodeTagRe = regexp.MustCompile(`\[[^\[\]]*\]`)

// sanitizeDescription strips Shikimori bbcode/anchor markup to plain text.
// We store/return ONLY plain text — never raw descriptionHtml — so there is
// no external link or XSS surface.
func sanitizeDescription(s string) string {
	return strings.TrimSpace(bbcodeTagRe.ReplaceAllString(s, ""))
}

// normalizeRole collapses Shikimori's rolesEn (e.g. ["Main"], ["Supporting"])
// to our two-value role: "main" for Main, "supporting" for everything else.
func normalizeRole(rolesEn []string) string {
	if len(rolesEn) > 0 && strings.EqualFold(rolesEn[0], "Main") {
		return "main"
	}
	return "supporting"
}

// postRaw POSTs a raw GraphQL query and unmarshals the `data` field into out.
// Mirrors executeRawQuery (client.go) but is generic over the data shape.
func (c *Client) postRaw(ctx context.Context, query string, out interface{}) error {
	reqBody := map[string]string{"query": query}
	jsonBody, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.GraphQLURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return errors.ExternalAPI("shikimori", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.config.UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.ExternalAPI("shikimori", err)
	}
	defer resp.Body.Close()

	envelope := struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return errors.ExternalAPI("shikimori", err)
	}
	if len(envelope.Errors) > 0 {
		return errors.ExternalAPI("shikimori", fmt.Errorf("%s", envelope.Errors[0].Message))
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return errors.ExternalAPI("shikimori", err)
	}
	return nil
}

// CharacterRoleResult is one character + its role on a given anime.
type CharacterRoleResult struct {
	Character domain.Character
	Role      string // "main" / "supporting"
}

// GetAnimeCharacters fetches an anime's character roles from Shikimori.
// characterRoles returns characters only (not staff). Description is NOT
// fetched here (list view doesn't need it) — see GetCharacterByID.
func (c *Client) GetAnimeCharacters(ctx context.Context, shikimoriID string) ([]CharacterRoleResult, error) {
	c.rateLimiter.acquire()

	query := fmt.Sprintf(`{
		animes(ids: "%s", limit: 1) {
			characterRoles {
				rolesEn
				rolesRu
				character {
					id malId name russian japanese
					poster { originalUrl }
				}
			}
		}
	}`, shikimoriID)

	var data struct {
		Animes []struct {
			CharacterRoles []struct {
				RolesEn   []string `json:"rolesEn"`
				RolesRu   []string `json:"rolesRu"`
				Character struct {
					ID       string `json:"id"`
					MalID    string `json:"malId"`
					Name     string `json:"name"`
					Russian  string `json:"russian"`
					Japanese string `json:"japanese"`
					Poster   *struct {
						OriginalURL string `json:"originalUrl"`
					} `json:"poster"`
				} `json:"character"`
			} `json:"characterRoles"`
		} `json:"animes"`
	}

	if err := c.postRaw(ctx, query, &data); err != nil {
		return nil, err
	}
	if len(data.Animes) == 0 {
		return nil, errors.NotFound("anime")
	}

	roles := data.Animes[0].CharacterRoles
	results := make([]CharacterRoleResult, 0, len(roles))
	for _, r := range roles {
		ch := domain.Character{
			ShikimoriID: r.Character.ID,
			MalID:       r.Character.MalID,
			Name:        r.Character.Name,
			NameRU:      r.Character.Russian,
			NameJP:      r.Character.Japanese,
		}
		if r.Character.Poster != nil {
			ch.PosterURL = r.Character.Poster.OriginalURL
		}
		results = append(results, CharacterRoleResult{Character: ch, Role: normalizeRole(r.RolesEn)})
	}
	return results, nil
}

// GetCharacterByID fetches a single character's detail (with sanitized
// description) from Shikimori GraphQL.
func (c *Client) GetCharacterByID(ctx context.Context, shikimoriID string) (*domain.Character, error) {
	c.rateLimiter.acquire()

	query := fmt.Sprintf(`{
		characters(ids: "%s") {
			id malId name russian japanese synonyms url
			poster { originalUrl }
			description
		}
	}`, shikimoriID)

	var data struct {
		Characters []struct {
			ID          string   `json:"id"`
			MalID       string   `json:"malId"`
			Name        string   `json:"name"`
			Russian     string   `json:"russian"`
			Japanese    string   `json:"japanese"`
			Synonyms    []string `json:"synonyms"`
			URL         string   `json:"url"`
			Description string   `json:"description"`
			Poster      *struct {
				OriginalURL string `json:"originalUrl"`
			} `json:"poster"`
		} `json:"characters"`
	}

	if err := c.postRaw(ctx, query, &data); err != nil {
		return nil, err
	}
	if len(data.Characters) == 0 {
		return nil, errors.NotFound("character")
	}

	src := data.Characters[0]
	ch := &domain.Character{
		ShikimoriID: src.ID,
		MalID:       src.MalID,
		Name:        src.Name,
		NameRU:      src.Russian,
		NameJP:      src.Japanese,
		Synonyms:    strings.Join(src.Synonyms, " / "),
		URL:         src.URL,
		Description: sanitizeDescription(src.Description),
	}
	if src.Poster != nil {
		ch.PosterURL = src.Poster.OriginalURL
	}
	ch.Seyu = c.fetchCharacterSeyu(ctx, shikimoriID)
	return ch, nil
}

// absImageURL turns a Shikimori-relative image path into an absolute URL.
// REST /api/characters/{id} returns seyu image paths relative to the site
// root (e.g. "/system/people/original/1.jpg"); GraphQL poster originalUrls are
// already absolute. Pure — unit tested.
func absImageURL(base, path string) string {
	if path == "" || strings.HasPrefix(path, "http") {
		return path
	}
	return strings.TrimRight(base, "/") + path
}

// fetchCharacterSeyu pulls a character's voice cast from Shikimori REST
// (GraphQL's Character type has no seiyu field, and the anime-level /roles
// endpoint never pairs a character with a person — only this per-character
// endpoint does). Returns an empty slice (never an error) on failure so the
// caller can still return the character.
func (c *Client) fetchCharacterSeyu(ctx context.Context, shikimoriID string) []domain.CharacterSeyu {
	c.rateLimiter.acquire()

	url := fmt.Sprintf("%s/api/characters/%s", c.config.BaseURL, shikimoriID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", c.config.UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Warnw("shikimori seyu fetch failed", "shikimori_id", shikimoriID, "error", err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var body struct {
		Seyu []struct {
			ID      int    `json:"id"`
			Name    string `json:"name"`
			Russian string `json:"russian"`
			URL     string `json:"url"`
			Image   *struct {
				Original string `json:"original"`
			} `json:"image"`
		} `json:"seyu"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil
	}

	out := make([]domain.CharacterSeyu, 0, len(body.Seyu))
	for _, s := range body.Seyu {
		img := ""
		if s.Image != nil {
			img = absImageURL(c.config.BaseURL, s.Image.Original)
		}
		out = append(out, domain.CharacterSeyu{
			ShikimoriID: fmt.Sprintf("%d", s.ID),
			Name:        s.Name,
			NameRU:      s.Russian,
			ImageURL:    img,
			URL:         s.URL,
		})
	}
	return out
}
