package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
)

// PoolAnimeRef is a thin alias so the picker signature reads clearly.
type PoolAnimeRef = domain.PoolAnime

type tokenStore interface {
	PutToken(ctx context.Context, token, animeID string) error
	GetToken(ctx context.Context, token string) (animeID string, ok bool, err error)
}

// Picker chooses the secret for a new endless round (injectable for tests).
type Picker func(pool []PoolAnimeRef) PoolAnimeRef

type EndlessService struct {
	pool   poolReader
	tokens tokenStore
	pick   Picker
}

func NewEndlessService(p poolReader, ts tokenStore, pick Picker) *EndlessService {
	if pick == nil {
		pick = randomPicker
	}
	return &EndlessService{pool: p, tokens: ts, pick: pick}
}

type EndlessRound struct {
	RoundToken string `json:"round_token"`
}

func (s *EndlessService) NewRound(ctx context.Context) (*EndlessRound, error) {
	pool, err := s.pool.All(ctx)
	if err != nil {
		return nil, err
	}
	if len(pool) == 0 {
		return nil, errors.New("anidle: empty pool")
	}
	secret := s.pick(pool)
	token := newToken()
	if err := s.tokens.PutToken(ctx, token, secret.ID); err != nil {
		return nil, err
	}
	return &EndlessRound{RoundToken: token}, nil
}

func (s *EndlessService) Guess(ctx context.Context, token, animeID string) (*GuessOutcome, error) {
	secretID, ok, err := s.tokens.GetToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("anidle: round expired or not found")
	}
	guess, gok := s.pool.Lookup(animeID)
	if !gok {
		return nil, errors.New("anidle: unknown anime")
	}
	secret, sok := s.pool.Lookup(secretID)
	if !sok {
		return nil, errors.New("anidle: secret missing from pool")
	}
	out := &GuessOutcome{Anime: visible(guess), Result: Compare(secret, guess), Solved: animeID == secretID}
	if out.Solved {
		a := visible(secret)
		out.Answer = &a
	}
	return out, nil
}

func newToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func randomPicker(pool []PoolAnimeRef) PoolAnimeRef {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	n := (uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24)
	return pool[int(n%uint32(len(pool)))]
}
