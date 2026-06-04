package repo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
)

// fakeStore is a Docker-free in-memory domain.EventStore double that records
// every call and can be programmed to fail.
type fakeStore struct {
	insertErr   error
	upsertErr   error
	insertCalls int
	upsertCalls int
	gotEvents   []domain.Event
	gotAnon     []string
}

func (f *fakeStore) InsertBatch(_ context.Context, events []domain.Event) error {
	f.insertCalls++
	f.gotEvents = append(f.gotEvents, events...)
	return f.insertErr
}

func (f *fakeStore) UpsertIdentity(_ context.Context, anonymousID, _ string, _ time.Time) error {
	f.upsertCalls++
	f.gotAnon = append(f.gotAnon, anonymousID)
	return f.upsertErr
}

func TestDualWriteStore_InsertBatch(t *testing.T) {
	primaryErr := errors.New("pg down")
	secondaryErr := errors.New("ch down")
	batch := []domain.Event{{AnonymousID: "anon-1"}, {AnonymousID: "anon-2"}}

	tests := []struct {
		name              string
		primaryErr        error
		secondaryErr      error
		wantErr           error
		wantSecondaryCall bool
	}{
		{
			name:              "both succeed: returns nil and both receive the batch",
			wantSecondaryCall: true,
		},
		{
			name:              "secondary error is swallowed: returns nil, primary succeeded",
			secondaryErr:      secondaryErr,
			wantErr:           nil,
			wantSecondaryCall: true,
		},
		{
			name:              "primary error is returned verbatim; secondary not attempted",
			primaryErr:        primaryErr,
			wantErr:           primaryErr,
			wantSecondaryCall: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			primary := &fakeStore{insertErr: tt.primaryErr}
			secondary := &fakeStore{insertErr: tt.secondaryErr}
			s := NewDualWriteStore(primary, secondary, nil)

			err := s.InsertBatch(context.Background(), batch)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("InsertBatch err = %v, want %v", err, tt.wantErr)
			}
			if primary.insertCalls != 1 {
				t.Fatalf("primary.insertCalls = %d, want 1", primary.insertCalls)
			}
			wantSec := 0
			if tt.wantSecondaryCall {
				wantSec = 1
			}
			if secondary.insertCalls != wantSec {
				t.Fatalf("secondary.insertCalls = %d, want %d", secondary.insertCalls, wantSec)
			}
			// When both were attempted, both must have received the full batch.
			if tt.primaryErr == nil {
				if len(primary.gotEvents) != len(batch) {
					t.Fatalf("primary got %d events, want %d", len(primary.gotEvents), len(batch))
				}
				if tt.wantSecondaryCall && len(secondary.gotEvents) != len(batch) {
					t.Fatalf("secondary got %d events, want %d", len(secondary.gotEvents), len(batch))
				}
			}
		})
	}
}

func TestDualWriteStore_UpsertIdentity(t *testing.T) {
	primaryErr := errors.New("pg down")
	secondaryErr := errors.New("ch down")

	tests := []struct {
		name              string
		primaryErr        error
		secondaryErr      error
		wantErr           error
		wantSecondaryCall bool
	}{
		{name: "both succeed", wantSecondaryCall: true},
		{name: "secondary swallowed", secondaryErr: secondaryErr, wantErr: nil, wantSecondaryCall: true},
		{name: "primary returned, secondary skipped", primaryErr: primaryErr, wantErr: primaryErr, wantSecondaryCall: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			primary := &fakeStore{upsertErr: tt.primaryErr}
			secondary := &fakeStore{upsertErr: tt.secondaryErr}
			s := NewDualWriteStore(primary, secondary, nil)

			err := s.UpsertIdentity(context.Background(), "anon-1", "user-1", time.Now())

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("UpsertIdentity err = %v, want %v", err, tt.wantErr)
			}
			if primary.upsertCalls != 1 {
				t.Fatalf("primary.upsertCalls = %d, want 1", primary.upsertCalls)
			}
			wantSec := 0
			if tt.wantSecondaryCall {
				wantSec = 1
			}
			if secondary.upsertCalls != wantSec {
				t.Fatalf("secondary.upsertCalls = %d, want %d", secondary.upsertCalls, wantSec)
			}
		})
	}
}
