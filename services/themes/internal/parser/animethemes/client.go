package animethemes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/themes/internal/domain"
)

const (
	baseURL      = "https://api.animethemes.moe"
	requestDelay = 700 * time.Millisecond // Rate limit: ~90 req/min
)

type Client struct {
	httpClient *http.Client
	log        *logger.Logger
}

func NewClient(log *logger.Logger) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		log:        log,
	}
}

// apiResponse represents the top-level AnimeThemes API response.
type apiResponse struct {
	Anime []animeData     `json:"anime"`
	Links paginationLinks `json:"links"`
}

type paginationLinks struct {
	Next string `json:"next"`
}

type animeData struct {
	ID          int            `json:"id"`
	Name        string         `json:"name"`
	Slug        string         `json:"slug"`
	Images      []imageData    `json:"images"`
	AnimeThemes []themeData    `json:"animethemes"`
	Resources   []resourceData `json:"resources"`
}

type resourceData struct {
	ExternalID *int   `json:"external_id"`
	Site       string `json:"site"`
}

type imageData struct {
	Facet string `json:"facet"`
	Link  string `json:"link"`
}

type themeData struct {
	ID                int          `json:"id"`
	Type              string       `json:"type"`   // "OP" or "ED"
	Sequence          *int         `json:"sequence"`
	Slug              string       `json:"slug"`   // "OP1", "ED2"
	Song              *songData    `json:"song"`
	AnimeThemeEntries []entryData  `json:"animethemeentries"`
}

type songData struct {
	Title   string       `json:"title"`
	Artists []artistData `json:"artists"`
}

type artistData struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type entryData struct {
	Videos []videoData `json:"videos"`
}

type videoData struct {
	ID         int    `json:"id"`
	Basename   string `json:"basename"`
	Resolution int    `json:"resolution"`
	NC         bool   `json:"nc"` // creditless
	Link       string `json:"link"`
	Audio      *audioData `json:"audio"`
}

type audioData struct {
	Basename string `json:"basename"`
}

// FetchSeason fetches all anime themes for a given year and season.
// It paginates through all results automatically.
func (c *Client) FetchSeason(year int, season string) ([]domain.AnimeTheme, error) {
	var allThemes []domain.AnimeTheme

	url := fmt.Sprintf(
		"%s/anime?filter[year]=%d&filter[season]=%s&include=animethemes.song.artists,animethemes.animethemeentries.videos,images,resources&page[size]=25&page[number]=1",
		baseURL, year, season,
	)

	for url != "" {
		c.log.Infow("fetching anime themes page", "url", url)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch page: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
		}

		var apiResp apiResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode response: %w", err)
		}
		resp.Body.Close()

		for _, anime := range apiResp.Anime {
			themes := c.extractThemes(anime, year, season)
			allThemes = append(allThemes, themes...)
		}

		url = apiResp.Links.Next
		if url != "" {
			time.Sleep(requestDelay)
		}
	}

	c.log.Infow("finished fetching season", "year", year, "season", season, "themes", len(allThemes))
	return allThemes, nil
}

func (c *Client) extractThemes(anime animeData, year int, season string) []domain.AnimeTheme {
	posterURL := ""
	for _, img := range anime.Images {
		if img.Facet == "Large Cover" || img.Facet == "Small Cover" {
			posterURL = img.Link
			break
		}
	}
	// Fallback to any image
	if posterURL == "" && len(anime.Images) > 0 {
		posterURL = anime.Images[0].Link
	}

	// Extract MAL ID from resources
	malID := 0
	for _, res := range anime.Resources {
		if res.Site == "MyAnimeList" && res.ExternalID != nil {
			malID = *res.ExternalID
			break
		}
	}

	var themes []domain.AnimeTheme
	for _, t := range anime.AnimeThemes {
		seq := 0
		if t.Sequence != nil {
			seq = *t.Sequence
		}

		songTitle := ""
		artistName := ""
		if t.Song != nil {
			songTitle = t.Song.Title
			if len(t.Song.Artists) > 0 {
				var names []string
				for _, a := range t.Song.Artists {
					names = append(names, a.Name)
				}
				artistName = strings.Join(names, ", ")
			}
		}

		video := c.selectBestVideo(t.AnimeThemeEntries)

		videoBasename := ""
		videoResolution := 0
		audioBasename := ""
		if video != nil {
			videoBasename = video.Basename
			videoResolution = video.Resolution
			if video.Audio != nil {
				audioBasename = video.Audio.Basename
			}
		}

		themes = append(themes, domain.AnimeTheme{
			ExternalID:      t.ID,
			AnimeName:       anime.Name,
			AnimeSlug:       anime.Slug,
			PosterURL:       posterURL,
			ThemeType:       t.Type,
			Sequence:        seq,
			Slug:            t.Slug,
			SongTitle:       songTitle,
			ArtistName:      artistName,
			VideoBasename:   videoBasename,
			VideoResolution: videoResolution,
			AudioBasename:   audioBasename,
			MALID:           malID,
			Year:            year,
			Season:          season,
		})
	}
	return themes
}

// selectBestVideo picks the best video from all entries.
// Prefers NC (creditless), then highest resolution.
func (c *Client) selectBestVideo(entries []entryData) *videoData {
	var best *videoData
	for _, entry := range entries {
		for i := range entry.Videos {
			v := &entry.Videos[i]
			if best == nil {
				best = v
				continue
			}
			// Prefer NC versions
			if v.NC && !best.NC {
				best = v
				continue
			}
			if !v.NC && best.NC {
				continue
			}
			// Then prefer higher resolution
			if v.Resolution > best.Resolution {
				best = v
			}
		}
	}
	return best
}
