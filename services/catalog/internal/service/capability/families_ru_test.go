package capability

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// dbSeq names each in-memory shared-cache database uniquely so concurrent family
// builders within one test see a shared schema, while different tests stay
// isolated (a global "file::memory:?cache=shared" DSN would cross-contaminate).
var dbSeq atomic.Int64

type fakeCatalog struct {
	kodik     []domain.KodikTranslation
	kodikErr  error
	anilib    []domain.AnimeLibTranslation
	anilibErr error
	heps      []domain.HanimeEpisode
	hepsErr   error
	hstream   *domain.HanimeStream
	hstreamEr error
}

func (f fakeCatalog) GetKodikTranslations(_ context.Context, _ string) ([]domain.KodikTranslation, error) {
	return f.kodik, f.kodikErr
}
func (f fakeCatalog) GetAnimeLibTranslations(_ context.Context, _ string, _ int) ([]domain.AnimeLibTranslation, error) {
	return f.anilib, f.anilibErr
}
func (f fakeCatalog) GetHanimeEpisodes(_ context.Context, _ string) ([]domain.HanimeEpisode, error) {
	return f.heps, f.hepsErr
}
func (f fakeCatalog) GetHanimeStream(_ context.Context, _ string, _ string) (*domain.HanimeStream, error) {
	return f.hstream, f.hstreamEr
}

// newDB builds an in-memory sqlite DB seeded with the given provider rows. It is
// the internal-package twin of the package-external helper in service_test.go,
// usable from the RU/Hanime family tests which run in `package capability`.
//
// buildFamilies resolves the kodik/animelib/hanime families concurrently, and
// each now reads its DB row. A plain `:memory:` DSN gives every pooled
// connection its OWN empty database, so a concurrent reader can land on an
// unmigrated connection ("no such table"). A shared-cache DSN pinned to a single
// connection makes all goroutines see the one migrated schema.
func newDB(t *testing.T, rows ...domain.ScraperProvider) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:capdb%d?mode=memory&cache=shared", dbSeq.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1) // keep the shared in-memory schema on one connection
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func findVariant(vs []domain.Variant, cat string) *domain.Variant {
	for i := range vs {
		if vs[i].Category == cat {
			return &vs[i]
		}
	}
	return nil
}

func TestKodikFamily_MapsTeamsAndCategories(t *testing.T) {
	s := &Service{db: newDB(t, domain.ScraperProvider{Name: "kodik-noads", Status: domain.StatusEnabled, Group: "ru", SupportsSub: true, SupportsDub: true}),
		catalog: fakeCatalog{kodik: []domain.KodikTranslation{
			{ID: 610, Title: "AniLibria", Type: "voice"},
			{ID: 735, Title: "SovetRomantica", Type: "subtitles"},
		}}}
	fam, ok := s.kodikFamily(context.Background(), "uuid")
	if !ok || fam.Family != "kodik" || len(fam.Providers) != 1 {
		t.Fatalf("kodik family wrong: ok=%v fam=%+v", ok, fam)
	}
	vs := fam.Providers[0].Variants
	dub := findVariant(vs, "dub")
	sub := findVariant(vs, "sub")
	if dub == nil || sub == nil {
		t.Fatalf("want dub+sub, got %+v", vs)
	}
	if dub.Team == nil || dub.Team.Name != "AniLibria" || dub.Team.ID != "610" {
		t.Errorf("dub team wrong: %+v", dub.Team)
	}
	if dub.SubDelivery != "none" {
		t.Errorf("dub sub_delivery = %q, want none", dub.SubDelivery)
	}
	if sub.SubDelivery != "hard" { // iframe, no external soft subs
		t.Errorf("sub sub_delivery = %q, want hard", sub.SubDelivery)
	}
}

func TestKodikFamily_OmittedWhenEmptyOrError(t *testing.T) {
	if _, ok := (&Service{catalog: fakeCatalog{kodik: nil}}).kodikFamily(context.Background(), "u"); ok {
		t.Error("empty kodik should be omitted")
	}
	if _, ok := (&Service{catalog: fakeCatalog{kodikErr: errors.New("boom")}}).kodikFamily(context.Background(), "u"); ok {
		t.Error("errored kodik should be omitted")
	}
}

func TestAnimelibFamily_SoftHardFromHasSubtitles(t *testing.T) {
	s := &Service{db: newDB(t, domain.ScraperProvider{Name: "animelib", Status: domain.StatusEnabled, Group: "ru", SupportsSub: true, SupportsDub: true}),
		catalog: fakeCatalog{anilib: []domain.AnimeLibTranslation{
			{ID: 1, TeamName: "Crunchyroll", Type: "subtitles", HasSubtitles: true},
			{ID: 2, TeamName: "AniRise", Type: "subtitles", HasSubtitles: false},
			{ID: 3, TeamName: "AniDUB", Type: "voice", HasSubtitles: false},
		}}}
	fam, ok := s.animelibFamily(context.Background(), "uuid")
	if !ok || fam.Family != "animelib" {
		t.Fatalf("animelib family wrong: ok=%v fam=%+v", ok, fam)
	}
	vs := fam.Providers[0].Variants
	if len(vs) != 3 {
		t.Fatalf("want 3 variants, got %d", len(vs))
	}
	soft, hard, dub := vs[0], vs[1], vs[2]
	if soft.SubDelivery != "soft" || soft.Team.Name != "Crunchyroll" {
		t.Errorf("soft sub wrong: %+v", soft)
	}
	if hard.SubDelivery != "hard" {
		t.Errorf("hard sub wrong: %+v", hard)
	}
	if dub.Category != "dub" || dub.SubDelivery != "none" {
		t.Errorf("dub wrong: %+v", dub)
	}
}

func TestHanimeFamily_QualitiesFromStream(t *testing.T) {
	s := &Service{db: newDB(t, domain.ScraperProvider{Name: "hanime", Status: domain.StatusEnabled, Group: "adult", SupportsRaw: true}),
		catalog: fakeCatalog{
			heps: []domain.HanimeEpisode{{Name: "Ep 1", Slug: "ep-1"}},
			hstream: &domain.HanimeStream{Sources: []domain.HanimeSource{
				{Height: "720"}, {Height: "1080"}, {Height: "720"}, // dup dropped
			}},
		}}
	fam, ok := s.hanimeFamily(context.Background(), "uuid")
	if !ok || fam.Family != "hanime" {
		t.Fatalf("hanime family wrong: ok=%v fam=%+v", ok, fam)
	}
	v := fam.Providers[0].Variants[0]
	if v.Category != "raw" || v.QualitySource != "discrete" {
		t.Errorf("raw variant wrong: %+v", v)
	}
	if len(v.Qualities) != 2 || v.Qualities[0] != "720p" || v.Qualities[1] != "1080p" {
		t.Errorf("qualities wrong: %+v", v.Qualities)
	}
}

func TestHanimeFamily_StreamErrorKeepsFamilyWithoutQuality(t *testing.T) {
	s := &Service{db: newDB(t, domain.ScraperProvider{Name: "hanime", Status: domain.StatusEnabled, Group: "adult", SupportsRaw: true}),
		catalog: fakeCatalog{
			heps:      []domain.HanimeEpisode{{Slug: "ep-1"}},
			hstreamEr: errors.New("no stream"),
		}}
	fam, ok := s.hanimeFamily(context.Background(), "uuid")
	if !ok {
		t.Fatal("episodes present → family should survive a stream error")
	}
	v := fam.Providers[0].Variants[0]
	if v.QualitySource != "unknown" || len(v.Qualities) != 0 {
		t.Errorf("expected no qualities, got %+v", v)
	}
}

func TestBuildFamilies_OrderAndBestEffort(t *testing.T) {
	db := newDB(t,
		domain.ScraperProvider{Name: "allanime", Status: domain.StatusEnabled, Group: "en", SupportsSub: true, PreferenceWeight: 90},
		domain.ScraperProvider{Name: "kodik-noads", Status: domain.StatusEnabled, Group: "ru", SupportsSub: true, SupportsDub: true},
		domain.ScraperProvider{Name: "hanime", Status: domain.StatusEnabled, Group: "adult", SupportsRaw: true},
	)

	// kodik present, animelib errors (omitted), hanime present → order en,kodik,hanime
	s := NewService(db, nil, fakeCatalog{
		kodik:     []domain.KodikTranslation{{ID: 1, Title: "T", Type: "voice"}},
		anilibErr: errors.New("not on animelib"),
		heps:      []domain.HanimeEpisode{{Slug: "ep-1"}},
		hstream:   &domain.HanimeStream{Sources: []domain.HanimeSource{{Height: "1080"}}},
	}, nil, nil)

	fams, err := s.buildFamilies(context.Background(), "uuid")
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(fams))
	for i, f := range fams {
		got[i] = f.Family
	}
	want := []string{"ourenglish", "kodik", "hanime"}
	if len(got) != len(want) {
		t.Fatalf("families = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("families = %v, want %v", got, want)
		}
	}
}

func TestKodikFamilyOmittedWhenRowDisabled(t *testing.T) {
	s := &Service{db: newDB(t, domain.ScraperProvider{Name: "kodik-noads", Status: domain.StatusDisabled, Group: "ru"}),
		catalog: fakeCatalog{kodik: []domain.KodikTranslation{{ID: 1, Title: "Team", Type: "voice"}}}}
	if _, ok := s.kodikFamily(context.Background(), "uuid"); ok {
		t.Fatal("kodik family must be omitted when its DB row is disabled")
	}
}

func TestKodikFamilyCarriesFeedFields(t *testing.T) {
	s := &Service{db: newDB(t, domain.ScraperProvider{Name: "kodik-noads", Status: domain.StatusEnabled, Health: domain.HealthUp, Group: "ru", PreferenceWeight: 50, SupportsSub: true, SupportsDub: true}),
		catalog: fakeCatalog{kodik: []domain.KodikTranslation{{ID: 1, Title: "Team", Type: "voice"}}}}
	fam, ok := s.kodikFamily(context.Background(), "uuid")
	if !ok {
		t.Fatal("expected kodik family")
	}
	p := fam.Providers[0]
	if p.State != "active" || !p.Selectable || p.Group != "ru" || p.Order != 50 {
		t.Fatalf("kodik feed fields wrong: %+v", p)
	}
}

func TestBuildFamilies_NilCatalogENOnly(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}
	s := NewService(db, nil, nil, nil, nil)
	fams, err := s.buildFamilies(context.Background(), "uuid")
	if err != nil {
		t.Fatal(err)
	}
	if len(fams) != 1 || fams[0].Family != "ourenglish" {
		t.Fatalf("nil catalog should yield EN-only, got %+v", fams)
	}
}
