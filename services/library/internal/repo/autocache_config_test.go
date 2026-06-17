package repo

import (
	"context"
	"errors"
	"testing"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// TestAutocacheConfig_TableName pins the GORM table name so a future
// refactor that accidentally pluralizes is caught.
func TestAutocacheConfig_TableName(t *testing.T) {
	if got := (domain.AutocacheConfig{}).TableName(); got != "autocache_config" {
		t.Fatalf("TableName() = %q, want autocache_config", got)
	}
}

// TestAutocacheConfig_Patch_EmptyMapRejected asserts Patch refuses an
// empty field map with InvalidInput before touching the DB — so the nil
// *gorm.DB here is never dereferenced. This guards against issuing an
// empty UPDATE (which would bump updated_at for no reason / could touch
// the wrong rows).
func TestAutocacheConfig_Patch_EmptyMapRejected(t *testing.T) {
	r := NewAutocacheConfigRepository(nil) // DB never reached on the empty-map path

	_, err := r.Patch(context.Background(), map[string]any{})
	if err == nil {
		t.Fatalf("Patch(empty) = nil error, want InvalidInput")
	}
	var appErr *liberrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != liberrors.CodeInvalidInput {
		t.Fatalf("Patch(empty) error = %v, want CodeInvalidInput", err)
	}

	// nil map is the same no-writable-keys case.
	if _, err := r.Patch(context.Background(), nil); err == nil {
		t.Fatalf("Patch(nil) = nil error, want InvalidInput")
	}
}
