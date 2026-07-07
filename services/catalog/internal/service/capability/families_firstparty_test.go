package capability

import (
	"context"
	"errors"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
)

type fakeLibrary struct {
	aeInfo    service.AeInfo
	aeInfoErr error
}

func (f fakeLibrary) AeTitleInfo(context.Context, string) (service.AeInfo, error) {
	return f.aeInfo, f.aeInfoErr
}

func TestAeFamilyPresent(t *testing.T) {
	db := newDB(t, domain.ScraperProvider{
		Name: "ae", Status: domain.StatusEnabled, Health: domain.HealthUp,
		Group: "firstparty", PreferenceWeight: 100, SupportsSub: true, SupportsRaw: true,
	})
	s := &Service{db: db, library: fakeLibrary{aeInfo: service.AeInfo{Present: true, Track: "raw", AudioLang: "jpn"}}}
	fam, ok := s.aeFamily(context.Background(), "uuid")
	if !ok {
		t.Fatal("ae family expected")
	}
	p := fam.Providers[0]
	if p.State != "active" || !p.Selectable || p.Group != "firstparty" || p.Order != 100 {
		t.Fatalf("ae present feed wrong: %+v", p)
	}
}

// EN dub → real dub variant + Audios:["dub"] + Lang:"en" — the fabricated
// "sub" (trait-only) audio no longer masks a real self-hosted English dub.
func TestAeFamilyRealDubEnglish(t *testing.T) {
	db := newDB(t, domain.ScraperProvider{
		Name: "ae", Status: domain.StatusEnabled, Health: domain.HealthUp,
		Group: "firstparty", PreferenceWeight: 100, SupportsSub: true, QualityCeiling: "720p",
	})
	s := &Service{db: db, library: fakeLibrary{aeInfo: service.AeInfo{
		Present: true, Track: "dub", AudioLang: "eng", Quality: "1080p",
	}}}
	fam, ok := s.aeFamily(context.Background(), "uuid")
	if !ok {
		t.Fatal("ae family expected")
	}
	p := fam.Providers[0]
	if len(p.Audios) != 1 || p.Audios[0] != "dub" {
		t.Fatalf("expected Audios=[dub], got %+v", p.Audios)
	}
	if p.Lang != "en" {
		t.Fatalf("expected Lang=en, got %q", p.Lang)
	}
	if p.State != "active" || !p.Selectable {
		t.Fatalf("real dub content must be active/selectable: %+v", p)
	}
	if len(p.Variants) != 1 {
		t.Fatalf("expected exactly 1 variant, got %+v", p.Variants)
	}
	v := p.Variants[0]
	if v.Category != "dub" || v.SubDelivery != "none" {
		t.Fatalf("expected dub/none variant, got %+v", v)
	}
	if v.QualitySource != "probed" || len(v.Qualities) != 1 || v.Qualities[0] != "1080p" {
		t.Fatalf("expected probed quality from AeInfo, got %+v", v)
	}
}

// RU dub → Audios:["dub"] + Lang:"ru".
func TestAeFamilyRealDubRussian(t *testing.T) {
	db := newDB(t, domain.ScraperProvider{
		Name: "ae", Status: domain.StatusEnabled, Health: domain.HealthUp, Group: "firstparty",
	})
	s := &Service{db: db, library: fakeLibrary{aeInfo: service.AeInfo{
		Present: true, Track: "dub", AudioLang: "rus",
	}}}
	fam, ok := s.aeFamily(context.Background(), "uuid")
	if !ok {
		t.Fatal("ae family expected")
	}
	p := fam.Providers[0]
	if len(p.Audios) != 1 || p.Audios[0] != "dub" || p.Lang != "ru" {
		t.Fatalf("expected Audios=[dub] Lang=ru, got audios=%+v lang=%q", p.Audios, p.Lang)
	}
}

// JP original audio → Audios:["sub"], Category:"sub", no Lang override (the
// content family still routes on `group`, not a fabricated per-title lang).
func TestAeFamilyRealRawJapanese(t *testing.T) {
	db := newDB(t, domain.ScraperProvider{
		Name: "ae", Status: domain.StatusEnabled, Health: domain.HealthUp,
		Group: "firstparty", SubDelivery: "soft",
	})
	s := &Service{db: db, library: fakeLibrary{aeInfo: service.AeInfo{
		Present: true, Track: "raw", AudioLang: "jpn", Quality: "1080p",
	}}}
	fam, ok := s.aeFamily(context.Background(), "uuid")
	if !ok {
		t.Fatal("ae family expected")
	}
	p := fam.Providers[0]
	if len(p.Audios) != 1 || p.Audios[0] != "sub" {
		t.Fatalf("expected Audios=[sub], got %+v", p.Audios)
	}
	if p.Lang != "" {
		t.Fatalf("expected no Lang override for raw/original audio, got %q", p.Lang)
	}
	if len(p.Variants) != 1 || p.Variants[0].Category != "sub" || p.Variants[0].SubDelivery != "soft" {
		t.Fatalf("expected sub/soft variant, got %+v", p.Variants)
	}
}

// A present library that does NOT hold episode 1 (a late-only auto-cache) is
// flagged PartialLibrary so the FE keeps it out of the fresh-open smart default.
func TestAeFamilyLateOnlyLibraryIsPartial(t *testing.T) {
	db := newDB(t, domain.ScraperProvider{
		Name: "ae", Status: domain.StatusEnabled, Health: domain.HealthUp,
		Group: "firstparty", SupportsSub: true, SupportsRaw: true,
	})
	s := &Service{db: db, library: fakeLibrary{aeInfo: service.AeInfo{
		Present: true, Track: "raw", CoversFirstEpisode: false, // holds only a late episode
	}}}
	fam, ok := s.aeFamily(context.Background(), "uuid")
	if !ok {
		t.Fatal("ae family expected")
	}
	if p := fam.Providers[0]; !p.PartialLibrary || p.State != "active" {
		t.Fatalf("late-only ae must be active + PartialLibrary=true: %+v", p)
	}
}

// A complete library (covers ep 1) is NOT partial — it stays the preferred
// smart default.
func TestAeFamilyCompleteLibraryNotPartial(t *testing.T) {
	db := newDB(t, domain.ScraperProvider{
		Name: "ae", Status: domain.StatusEnabled, Health: domain.HealthUp,
		Group: "firstparty", SupportsSub: true, SupportsRaw: true,
	})
	s := &Service{db: db, library: fakeLibrary{aeInfo: service.AeInfo{
		Present: true, Track: "raw", CoversFirstEpisode: true,
	}}}
	fam, ok := s.aeFamily(context.Background(), "uuid")
	if !ok {
		t.Fatal("ae family expected")
	}
	if p := fam.Providers[0]; p.PartialLibrary {
		t.Fatalf("complete ae (covers ep 1) must not be PartialLibrary: %+v", p)
	}
}

// Absent content (no_content) is never flagged partial — nothing to keep out of
// the default; the FE already filters no_content out.
func TestAeFamilyAbsentNotPartial(t *testing.T) {
	db := newDB(t, domain.ScraperProvider{
		Name: "ae", Status: domain.StatusEnabled, Health: domain.HealthUp, Group: "firstparty",
	})
	s := &Service{db: db, library: fakeLibrary{}}
	fam, _ := s.aeFamily(context.Background(), "uuid")
	if p := fam.Providers[0]; p.PartialLibrary {
		t.Fatalf("absent ae must not be PartialLibrary: %+v", p)
	}
}

func TestAeFamilyAbsentIsNoContent(t *testing.T) {
	db := newDB(t, domain.ScraperProvider{
		Name: "ae", Status: domain.StatusEnabled, Health: domain.HealthUp, Group: "firstparty",
	})
	s := &Service{db: db, library: fakeLibrary{}}
	fam, ok := s.aeFamily(context.Background(), "uuid")
	if !ok {
		t.Fatal("ae family still emitted (tinted), not omitted")
	}
	if p := fam.Providers[0]; p.State != "no_content" || p.Selectable {
		t.Fatalf("ae absent must be no_content/not-selectable: %+v", p)
	}
}

// Present:false (not self-hosted) falls back to the provider's static trait
// variants — unchanged from before this task's real-content rewrite.
func TestAeFamilyPresentFalseFallsBackToTraits(t *testing.T) {
	row := domain.ScraperProvider{
		Name: "ae", Status: domain.StatusEnabled, Health: domain.HealthUp,
		Group: "firstparty", SupportsSub: true, SubDelivery: "soft", QualityCeiling: "1080p",
	}
	db := newDB(t, row)
	s := &Service{db: db, library: fakeLibrary{}} // zero-value AeInfo: Present:false
	fam, ok := s.aeFamily(context.Background(), "uuid")
	if !ok {
		t.Fatal("ae family expected")
	}
	p := fam.Providers[0]
	if p.State != "no_content" {
		t.Fatalf("expected no_content, got %+v", p)
	}
	want := variantsFromTraits(row)
	if len(p.Variants) != len(want) || p.Variants[0].Category != "sub" || p.Variants[0].QualitySource != "trait" {
		t.Fatalf("expected trait-fallback variants %+v, got %+v", want, p.Variants)
	}
	if p.Lang != "" {
		t.Fatalf("expected no Lang on trait fallback, got %q", p.Lang)
	}
}

// Present but the dub language doesn't normalize to a known audience (e.g. an
// unexpected audio_lang value) — falls back to the trait variants rather than
// emitting a Lang-less/nonsensical "dub".
func TestAeFamilyUnusableDubLangFallsBackToTraits(t *testing.T) {
	db := newDB(t, domain.ScraperProvider{
		Name: "ae", Status: domain.StatusEnabled, Health: domain.HealthUp,
		Group: "firstparty", SupportsDub: true, QualityCeiling: "1080p",
	})
	s := &Service{db: db, library: fakeLibrary{aeInfo: service.AeInfo{
		Present: true, Track: "dub", AudioLang: "und",
	}}}
	fam, ok := s.aeFamily(context.Background(), "uuid")
	if !ok {
		t.Fatal("ae family expected")
	}
	p := fam.Providers[0]
	if p.State != "active" { // content IS present, just not classifiable
		t.Fatalf("expected active (content present), got %+v", p)
	}
	if p.Lang != "" {
		t.Fatalf("expected no Lang on unusable-dub fallback, got %q", p.Lang)
	}
	if len(p.Variants) != 1 || p.Variants[0].Category != "dub" || p.Variants[0].QualitySource != "trait" {
		t.Fatalf("expected trait-fallback dub variant, got %+v", p.Variants)
	}
}

// A library lookup failure must NOT drop the family — it tints to no_content.
func TestAeFamilyLibraryErrorTints(t *testing.T) {
	db := newDB(t, domain.ScraperProvider{
		Name: "ae", Status: domain.StatusEnabled, Health: domain.HealthUp, Group: "firstparty",
	})
	s := &Service{db: db, library: fakeLibrary{aeInfoErr: errors.New("library down")}}
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
	if _, ok := (&Service{db: newDB(t), library: fakeLibrary{aeInfo: service.AeInfo{Present: true}}}).aeFamily(context.Background(), "u"); ok {
		t.Error("ae family must be omitted when DB row is absent")
	}
	// disabled row
	db := newDB(t, domain.ScraperProvider{Name: "ae", Status: domain.StatusDisabled, Group: "firstparty"})
	if _, ok := (&Service{db: db, library: fakeLibrary{aeInfo: service.AeInfo{Present: true}}}).aeFamily(context.Background(), "u"); ok {
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
