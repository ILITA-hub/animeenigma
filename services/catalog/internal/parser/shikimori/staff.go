package shikimori

import (
	"context"
	"fmt"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// staffRoleWhitelist is the ordered set of headline crew roles we keep from
// Shikimori personRoles. Order IS the display rank (index). Everything else
// (Key Animation, In-Between, etc.) is dropped — it is 90%+ of the payload and
// pure noise for a viewer. Match is exact against Shikimori's rolesEn strings.
var staffRoleWhitelist = []string{
	"Director",
	"Original Creator",
	"Series Composition",
	"Script",
	"Character Design",
	"Chief Animation Director",
	"Art Director",
	"Music",
	"Sound Director",
	"Director of Photography",
	"Producer",
	"Executive Producer",
}

// staffRoleRank maps a whitelisted role to its display rank. Built once.
var staffRoleRank = func() map[string]int {
	m := make(map[string]int, len(staffRoleWhitelist))
	for i, r := range staffRoleWhitelist {
		m[r] = i
	}
	return m
}()

type staffRole struct {
	Role   string // canonical EN
	RoleRU string // Shikimori's parallel rolesRu entry, "" if absent
	Rank   int
}

// filterStaffRoles keeps only whitelisted roles from a person's parallel
// rolesEn/rolesRu arrays, one staffRole per kept role. Pure — unit tested.
func filterStaffRoles(rolesEn, rolesRu []string) []staffRole {
	out := make([]staffRole, 0, len(rolesEn))
	for i, en := range rolesEn {
		rank, ok := staffRoleRank[en]
		if !ok {
			continue
		}
		ru := ""
		if i < len(rolesRu) {
			ru = rolesRu[i]
		}
		out = append(out, staffRole{Role: en, RoleRU: ru, Rank: rank})
	}
	return out
}

// GetAnimeStaff fetches an anime's crew from Shikimori personRoles, flattened
// to one domain.AnimePersonRole per (person, whitelisted role). AnimeID is left
// blank for the service to fill. personRoles is the STAFF list — it does NOT
// contain the voice cast (that lives on each character; see GetCharacterByID).
func (c *Client) GetAnimeStaff(ctx context.Context, shikimoriID string) ([]domain.AnimePersonRole, error) {
	c.rateLimiter.acquire()

	query := fmt.Sprintf(`{
		animes(ids: "%s", limit: 1) {
			personRoles {
				rolesEn
				rolesRu
				person {
					id name russian japanese isProducer isMangaka
					poster { originalUrl }
				}
			}
		}
	}`, shikimoriID)

	var data struct {
		Animes []struct {
			PersonRoles []struct {
				RolesEn []string `json:"rolesEn"`
				RolesRu []string `json:"rolesRu"`
				Person  struct {
					ID         string `json:"id"`
					Name       string `json:"name"`
					Russian    string `json:"russian"`
					Japanese   string `json:"japanese"`
					IsProducer bool   `json:"isProducer"`
					IsMangaka  bool   `json:"isMangaka"`
					Poster     *struct {
						OriginalURL string `json:"originalUrl"`
					} `json:"poster"`
				} `json:"person"`
			} `json:"personRoles"`
		} `json:"animes"`
	}

	if err := c.postRaw(ctx, query, &data); err != nil {
		return nil, err
	}
	if len(data.Animes) == 0 {
		return nil, errors.NotFound("anime")
	}

	rolesRaw := data.Animes[0].PersonRoles
	out := make([]domain.AnimePersonRole, 0, len(rolesRaw))
	for _, pr := range rolesRaw {
		kept := filterStaffRoles(pr.RolesEn, pr.RolesRu)
		if len(kept) == 0 {
			continue
		}
		poster := ""
		if pr.Person.Poster != nil {
			poster = pr.Person.Poster.OriginalURL
		}
		for _, k := range kept {
			out = append(out, domain.AnimePersonRole{
				ShikimoriPersonID: pr.Person.ID,
				Name:              pr.Person.Name,
				NameRU:            pr.Person.Russian,
				NameJP:            pr.Person.Japanese,
				PosterURL:         poster,
				Role:              k.Role,
				RoleRU:            k.RoleRU,
				IsProducer:        pr.Person.IsProducer,
				IsMangaka:         pr.Person.IsMangaka,
				Position:          k.Rank,
			})
		}
	}
	return out, nil
}
