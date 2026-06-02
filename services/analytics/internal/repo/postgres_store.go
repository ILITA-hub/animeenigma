package repo

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
	"gorm.io/gorm"
)

// PostgresStore implements domain.EventStore against GORM (Postgres in
// prod, sqlite in tests).
type PostgresStore struct{ db *gorm.DB }

func NewPostgresStore(db *gorm.DB) *PostgresStore { return &PostgresStore{db: db} }

func toModel(e domain.Event) Event {
	m := Event{
		EventID: e.EventID, EventType: string(e.EventType), EventName: e.EventName,
		AnonymousID: e.AnonymousID, SessionID: e.SessionID,
		Timestamp: e.Timestamp, ReceivedAt: e.ReceivedAt,
		URL: e.URL, Path: e.Path, Referrer: e.Referrer, Title: e.Title,
		ElSelector: e.ElSelector, ElText: e.ElText, ElTag: e.ElTag, ElAttrs: e.ElAttrs,
		ActiveMS: e.ActiveMS, UserAgent: e.UserAgent, DeviceType: e.DeviceType,
		ScreenW: e.ScreenW, ScreenH: e.ScreenH, IPHash: e.IPHash,
		TraceID: e.TraceID, Properties: e.Properties,
	}
	if e.UserID != "" {
		uid := e.UserID
		m.UserID = &uid
	}
	if m.ElAttrs == "" {
		m.ElAttrs = "{}"
	}
	if m.Properties == "" {
		m.Properties = "{}"
	}
	return m
}

func (s *PostgresStore) InsertBatch(ctx context.Context, events []domain.Event) error {
	if len(events) == 0 {
		return nil
	}
	rows := make([]Event, 0, len(events))
	for _, e := range events {
		rows = append(rows, toModel(e))
	}
	return s.db.WithContext(ctx).CreateInBatches(rows, 200).Error
}

func (s *PostgresStore) UpsertIdentity(ctx context.Context, anonymousID, userID string, ts time.Time) error {
	if anonymousID == "" || userID == "" {
		return nil
	}
	return s.db.WithContext(ctx).Create(&Identity{
		AnonymousID: anonymousID, UserID: userID, Timestamp: ts,
	}).Error
}
