package service

import (
	"context"
	"strings"

	"github.com/anacrolix/torrent/metainfo"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/repo"
)

// activeJobStatuses are the states in which a job's payload may still occupy the
// torrent working dir (download → encode reads the same {infohash}/ dir).
var activeJobStatuses = []domain.JobStatus{
	domain.JobStatusQueued,
	domain.JobStatusDownloading,
	domain.JobStatusEncoding,
	domain.JobStatusTranscoding,
	domain.JobStatusUploading,
}

type jobLister interface {
	List(ctx context.Context, f repo.JobFilter) ([]domain.Job, error)
}

// ActiveTorrents answers "is this infohash still in use by an in-flight job?" so the
// file manager refuses to delete a working-dir that a job is actively writing/reading.
type ActiveTorrents struct{ jobs jobLister }

func NewActiveTorrents(jobs jobLister) *ActiveTorrents { return &ActiveTorrents{jobs: jobs} }

func (a *ActiveTorrents) Infohashes(ctx context.Context) (map[string]struct{}, error) {
	rows, err := a.jobs.List(ctx, repo.JobFilter{Statuses: activeJobStatuses, Limit: 500})
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(rows))
	for _, j := range rows {
		m, err := metainfo.ParseMagnetUri(j.Magnet)
		if err != nil {
			continue // a malformed magnet can't map to a working dir
		}
		set[strings.ToLower(m.InfoHash.HexString())] = struct{}{}
	}
	return set, nil
}
