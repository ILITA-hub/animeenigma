// Package service holds the storage service's business logic: placement
// policy (this file) and the backend clients (backends.go).
package service

import (
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/storage/internal/domain"
)

// Placement resolves which backend a piece of content lands on, given its
// content class and an optional per-request override. It is the single
// authority for placement policy — see docs/superpowers/specs/
// 2026-07-10-storage-service-design.md "Placement policy".
type Placement struct {
	defaults map[string]string
	s3Absent bool
	log      *logger.Logger
}

// NewPlacement constructs a Placement. defaults maps a content class
// (domain.Class*) to its default backend id (domain.Backend*), normally
// config.Config.Defaults. s3Absent is true when STORAGE_S3_ENDPOINT is
// empty (no s3 backend configured) — a class/override resolving to "s3"
// then falls back to "minio" so dev environments without external S3 creds
// keep working. log may be nil (tests exercising the pure resolution path
// don't need one).
func NewPlacement(defaults map[string]string, s3Absent bool, log *logger.Logger) *Placement {
	return &Placement{defaults: defaults, s3Absent: s3Absent, log: log}
}

// Resolve returns the backend id ("minio" | "s3") that a class/override
// combination should land on.
//
//   - class must be one of the domain.Class* constants.
//   - override, when non-empty, must be domain.BackendMinio or
//     domain.BackendS3, and is only honored for domain.ClassLibraryManual —
//     any other class rejects a non-empty override.
func (p *Placement) Resolve(class, override string) (string, error) {
	switch class {
	case domain.ClassLibraryAuto, domain.ClassUpscaled, domain.ClassLibraryManual:
	default:
		return "", errors.InvalidInput("unknown content class: " + class)
	}

	backend := p.defaults[class]
	if override != "" {
		if class != domain.ClassLibraryManual {
			return "", errors.InvalidInput("override only allowed for library-manual")
		}
		if override != domain.BackendMinio && override != domain.BackendS3 {
			return "", errors.InvalidInput("unknown storage override: " + override)
		}
		backend = override
	}

	if backend == domain.BackendS3 && p.s3Absent {
		if p.log != nil {
			p.log.Warnw("s3 backend absent; falling back to minio", "class", class)
		}
		return domain.BackendMinio, nil
	}
	return backend, nil
}
