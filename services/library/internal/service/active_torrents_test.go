package service

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/repo"
)

type stubJobs struct{ jobs []domain.Job }

func (s *stubJobs) List(_ context.Context, _ repo.JobFilter) ([]domain.Job, error) {
	return s.jobs, nil
}

func TestActiveTorrents_Infohashes(t *testing.T) {
	// A valid v1 magnet (40-hex infohash).
	const ih = "0123456789abcdef0123456789abcdef01234567"
	s := &stubJobs{jobs: []domain.Job{
		{Magnet: "magnet:?xt=urn:btih:" + ih + "&dn=x", Status: domain.JobStatusDownloading},
		{Magnet: "not-a-magnet", Status: domain.JobStatusQueued}, // skipped, not fatal
	}}
	a := NewActiveTorrents(s)
	set, err := a.Infohashes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := set[ih]; !ok {
		t.Fatalf("expected %s in active set, got %v", ih, set)
	}
	if len(set) != 1 {
		t.Fatalf("bad magnet must be skipped, got %v", set)
	}
}
