package animeparser

import (
	"context"
	"fmt"
)

// Source represents an external anime database
type Source string

const (
	SourceMAL       Source = "mal"
	SourceShikimori Source = "shikimori"
	SourceAniList   Source = "anilist"
	SourceAniDB     Source = "anidb"
	SourceKitsu     Source = "kitsu"
)

// ExternalID represents an ID mapping to an external source
type ExternalID struct {
	Source Source
	ID     string
}

// IDMapping represents a complete ID mapping for an anime
type IDMapping struct {
	InternalID  string
	MAL         string
	Shikimori   string
	AniList     string
	AniDB       string
	Kitsu       string
	Title       string // For logging/debugging
}

// IDResolver resolves anime IDs across different sources
type IDResolver interface {
	// ResolveToInternal converts an external ID to internal ID
	ResolveToInternal(ctx context.Context, source Source, externalID string) (string, error)

	// ResolveFromInternal gets all external IDs for an internal ID
	ResolveFromInternal(ctx context.Context, internalID string) (*IDMapping, error)

	// GetExternalID gets a specific external ID for an internal ID
	GetExternalID(ctx context.Context, internalID string, targetSource Source) (string, error)

	// StoreMapping stores a new ID mapping
	StoreMapping(ctx context.Context, mapping IDMapping) error

	// FindByTitle attempts to resolve IDs by title matching
	FindByTitle(ctx context.Context, title string) ([]IDMapping, error)
}

// IDMappingStore is the storage interface for ID mappings
type IDMappingStore interface {
	Get(ctx context.Context, internalID string) (*IDMapping, error)
	GetByExternal(ctx context.Context, source Source, externalID string) (*IDMapping, error)
	Save(ctx context.Context, mapping IDMapping) error
	Search(ctx context.Context, title string, limit int) ([]IDMapping, error)
}

// DefaultIDResolver implements IDResolver with caching
type DefaultIDResolver struct {
	store IDMappingStore
}

func NewIDResolver(store IDMappingStore) *DefaultIDResolver {
	return &DefaultIDResolver{store: store}
}

func (r *DefaultIDResolver) ResolveToInternal(ctx context.Context, source Source, externalID string) (string, error) {
	mapping, err := r.store.GetByExternal(ctx, source, externalID)
	if err != nil {
		return "", fmt.Errorf("resolve to internal: %w", err)
	}
	return mapping.InternalID, nil
}

func (r *DefaultIDResolver) ResolveFromInternal(ctx context.Context, internalID string) (*IDMapping, error) {
	return r.store.Get(ctx, internalID)
}

func (r *DefaultIDResolver) GetExternalID(ctx context.Context, internalID string, targetSource Source) (string, error) {
	mapping, err := r.store.Get(ctx, internalID)
	if err != nil {
		return "", err
	}

	switch targetSource {
	case SourceMAL:
		return mapping.MAL, nil
	case SourceShikimori:
		return mapping.Shikimori, nil
	case SourceAniList:
		return mapping.AniList, nil
	case SourceAniDB:
		return mapping.AniDB, nil
	case SourceKitsu:
		return mapping.Kitsu, nil
	default:
		return "", fmt.Errorf("unknown source: %s", targetSource)
	}
}

func (r *DefaultIDResolver) StoreMapping(ctx context.Context, mapping IDMapping) error {
	return r.store.Save(ctx, mapping)
}

func (r *DefaultIDResolver) FindByTitle(ctx context.Context, title string) ([]IDMapping, error) {
	return r.store.Search(ctx, title, 10)
}

// MergeIDs merges two ID mappings, preferring non-empty values from the second
func MergeIDs(existing, new IDMapping) IDMapping {
	result := existing
	if new.MAL != "" && result.MAL == "" {
		result.MAL = new.MAL
	}
	if new.Shikimori != "" && result.Shikimori == "" {
		result.Shikimori = new.Shikimori
	}
	if new.AniList != "" && result.AniList == "" {
		result.AniList = new.AniList
	}
	if new.AniDB != "" && result.AniDB == "" {
		result.AniDB = new.AniDB
	}
	if new.Kitsu != "" && result.Kitsu == "" {
		result.Kitsu = new.Kitsu
	}
	return result
}

// ValidateMapping checks if a mapping has at least one external ID
func ValidateMapping(m IDMapping) error {
	if m.InternalID == "" {
		return fmt.Errorf("internal ID is required")
	}
	if m.MAL == "" && m.Shikimori == "" && m.AniList == "" && m.AniDB == "" && m.Kitsu == "" {
		return fmt.Errorf("at least one external ID is required")
	}
	return nil
}
