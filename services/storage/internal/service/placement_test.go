package service

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/storage/internal/domain"
)

// defaultDefaults mirrors the spec defaults (STORAGE_CLASS_LIBRARY_AUTO=s3,
// STORAGE_CLASS_LIBRARY_MANUAL=minio, STORAGE_CLASS_UPSCALED=s3).
func defaultDefaults() map[string]string {
	return map[string]string{
		domain.ClassLibraryAuto:   domain.BackendS3,
		domain.ClassLibraryManual: domain.BackendMinio,
		domain.ClassUpscaled:      domain.BackendS3,
	}
}

func TestPlacement_Resolve(t *testing.T) {
	cases := []struct {
		name        string
		class       string
		override    string
		s3Absent    bool
		wantStorage string
		wantErr     bool
	}{
		{
			name:        "library-auto default resolves to s3",
			class:       domain.ClassLibraryAuto,
			wantStorage: domain.BackendS3,
		},
		{
			name:        "library-manual default resolves to minio",
			class:       domain.ClassLibraryManual,
			wantStorage: domain.BackendMinio,
		},
		{
			name:        "library-manual override s3 resolves to s3",
			class:       domain.ClassLibraryManual,
			override:    domain.BackendS3,
			wantStorage: domain.BackendS3,
		},
		{
			name:    "unknown content class is rejected",
			class:   "bogus-class",
			wantErr: true,
		},
		{
			name:     "override on library-auto is rejected",
			class:    domain.ClassLibraryAuto,
			override: domain.BackendMinio,
			wantErr:  true,
		},
		{
			name:     "unknown override value is rejected",
			class:    domain.ClassLibraryManual,
			override: "glacier",
			wantErr:  true,
		},
		{
			name:        "s3 absent falls back to minio with a warn log",
			class:       domain.ClassUpscaled,
			s3Absent:    true,
			wantStorage: domain.BackendMinio,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewPlacement(defaultDefaults(), tc.s3Absent, nil)
			storage, err := p.Resolve(tc.class, tc.override)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got storage=%q", storage)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if storage != tc.wantStorage {
				t.Fatalf("storage = %q, want %q", storage, tc.wantStorage)
			}
		})
	}
}
