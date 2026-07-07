package capability

import (
	"context"
	"errors"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
)

type fakeLibrary struct {
	has bool
	err error

	aeInfo    service.AeInfo
	aeInfoErr error
}

func (f fakeLibrary) HasLibraryTitle(context.Context, string) (bool, error) { return f.has, f.err }

func (f fakeLibrary) AeTitleInfo(context.Context, string) (service.AeInfo, error) {
	return f.aeInfo, f.aeInfoErr
}

func TestAeFamilyPresent(t *testing.T) {
	db := newDB(t, domain.ScraperProvider{
		Name: "ae", Status: domain.StatusEnabled, Health: domain.HealthUp,
		Group: "firstparty", PreferenceWeight: 100, SupportsSub: true, SupportsRaw: true,
	})
	s := &Service{db: db, library: fakeLibrary{has: true}}
	fam, ok := s.aeFamily(context.Background(), "uuid")
	if !ok {
		t.Fatal("ae family expected")
	}
	p := fam.Providers[0]
	if p.State != "active" || !p.Selectable || p.Group != "firstparty" || p.Order != 100 {
		t.Fatalf("ae present feed wrong: %+v", p)
	}
}

func TestAeFamilyAbsentIsNoContent(t *testing.T) {
	db := newDB(t, domain.ScraperProvider{
		Name: "ae", Status: domain.StatusEnabled, Health: domain.HealthUp, Group: "firstparty",
	})
	s := &Service{db: db, library: fakeLibrary{has: false}}
	fam, ok := s.aeFamily(context.Background(), "uuid")
	if !ok {
		t.Fatal("ae family still emitted (tinted), not omitted")
	}
	if p := fam.Providers[0]; p.State != "no_content" || p.Selectable {
		t.Fatalf("ae absent must be no_content/not-selectable: %+v", p)
	}
}

// A library lookup failure must NOT drop the family — it tints to no_content.
func TestAeFamilyLibraryErrorTints(t *testing.T) {
	db := newDB(t, domain.ScraperProvider{
		Name: "ae", Status: domain.StatusEnabled, Health: domain.HealthUp, Group: "firstparty",
	})
	s := &Service{db: db, library: fakeLibrary{err: errors.New("library down")}}
	fam, ok := s.aeFamily(context.Background(), "uuid")
	if !ok {
		t.Fatal("library error must still emit ae family (tinted)")
	}
	if p := fam.Providers[0]; p.State != "no_content" || p.Selectable {
		t.Fatalf("library error must tint to no_content: %+v", p)
	}
}

// A nil LibrarySource behaves like absent content (no_content), never panics.
func TestAeFamilyNilLibraryIsNoContent(t *testing.T) {
	db := newDB(t, domain.ScraperProvider{
		Name: "ae", Status: domain.StatusEnabled, Health: domain.HealthUp, Group: "firstparty",
	})
	s := &Service{db: db, library: nil}
	fam, ok := s.aeFamily(context.Background(), "uuid")
	if !ok {
		t.Fatal("nil library must still emit ae family (tinted)")
	}
	if p := fam.Providers[0]; p.State != "no_content" {
		t.Fatalf("nil library must be no_content: %+v", p)
	}
}

func TestAeFamilyOmittedWhenRowAbsentOrDisabled(t *testing.T) {
	// absent row
	if _, ok := (&Service{db: newDB(t), library: fakeLibrary{has: true}}).aeFamily(context.Background(), "u"); ok {
		t.Error("ae family must be omitted when DB row is absent")
	}
	// disabled row
	db := newDB(t, domain.ScraperProvider{Name: "ae", Status: domain.StatusDisabled, Group: "firstparty"})
	if _, ok := (&Service{db: db, library: fakeLibrary{has: true}}).aeFamily(context.Background(), "u"); ok {
		t.Error("ae family must be omitted when DB row is disabled")
	}
}

// dbRowFamily is the generic DB-row→family builder shared by the catalog-side
// providers (18anime/adult here; the JP "raw" provider it also fronted was
// removed 2026-06-30). Exercise its present / absent / disabled branches.
func TestDBRowFamilyPresentAndOmitted(t *testing.T) {
	db := newDB(t, domain.ScraperProvider{
		Name: "18anime", Status: domain.StatusEnabled, Health: domain.HealthUp,
		Group: "adult", PreferenceWeight: 70,
	})
	s := &Service{db: db}
	fam, ok := s.dbRowFamily(context.Background(), "18anime", "18anime", "adult")
	if !ok || fam.Family != "adult" {
		t.Fatalf("adult family wrong: ok=%v fam=%+v", ok, fam)
	}
	p := fam.Providers[0]
	if p.State != "active" || !p.Selectable || p.Group != "adult" || p.Order != 70 {
		t.Fatalf("adult feed fields wrong: %+v", p)
	}
	// absent row → omitted
	if _, ok := (&Service{db: newDB(t)}).dbRowFamily(context.Background(), "18anime", "18anime", "adult"); ok {
		t.Error("adult family must be omitted when DB row is absent")
	}
	// disabled row → omitted
	dbd := newDB(t, domain.ScraperProvider{Name: "18anime", Status: domain.StatusDisabled, Group: "adult"})
	if _, ok := (&Service{db: dbd}).dbRowFamily(context.Background(), "18anime", "18anime", "adult"); ok {
		t.Error("adult family must be omitted when DB row is disabled")
	}
}

func TestAdult18animeFamilyPresentAndOmitted(t *testing.T) {
	db := newDB(t, domain.ScraperProvider{
		Name: "18anime", Status: domain.StatusEnabled, Health: domain.HealthUp,
		Group: "adult", PreferenceWeight: 20, SupportsRaw: true,
	})
	s := &Service{db: db}
	fam, ok := s.dbRowFamily(context.Background(), "18anime", "18anime", "adult")
	if !ok || fam.Family != "adult" {
		t.Fatalf("adult family wrong: ok=%v fam=%+v", ok, fam)
	}
	p := fam.Providers[0]
	if p.Provider != "18anime" || p.State != "active" || !p.Selectable || p.Group != "adult" || p.Order != 20 {
		t.Fatalf("18anime feed fields wrong: %+v", p)
	}
	// absent row → omitted
	if _, ok := (&Service{db: newDB(t)}).dbRowFamily(context.Background(), "18anime", "18anime", "adult"); ok {
		t.Error("adult family must be omitted when DB row is absent")
	}
	// disabled row → omitted
	dbd := newDB(t, domain.ScraperProvider{Name: "18anime", Status: domain.StatusDisabled, Group: "adult"})
	if _, ok := (&Service{db: dbd}).dbRowFamily(context.Background(), "18anime", "18anime", "adult"); ok {
		t.Error("adult family must be omitted when DB row is disabled")
	}
}
