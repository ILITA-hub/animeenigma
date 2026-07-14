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
	kodik      []domain.KodikTranslation
	kodikErr   error
	anilib     []domain.AnimeLibTranslation
	anilibErr  error
	heps       []domain.HanimeEpisode
	hepsErr    error
	hstream    *domain.HanimeStream
	hstreamEr  error
	ajTeams    []domain.AnimejoyTeam
	ajTeamsErr error
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
func (f fakeCatalog) GetAnimejoyTeams(_ context.Context, _ string) ([]domain.AnimejoyTeam, error) {
	return f.ajTeams, f.ajTeamsErr
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

func TestKodikFamily_EmptyNoContent(t *testing.T) {
	s := &Service{db: newDB(t, domain.ScraperProvider{Name: "kodik-noads", Status: domain.StatusEnabled, Group: "ru", SupportsSub: true, SupportsDub: true}),
		catalog: fakeCatalog{kodik: nil}}
	fam, ok := s.kodikFamily(context.Background(), "u")
	if !ok || fam.Family != "kodik" || len(fam.Providers) != 1 {
		t.Fatalf("empty kodik should surface a present no_content family: ok=%v fam=%+v", ok, fam)
	}
	p := fam.Providers[0]
	if p.State != "no_content" || p.Selectable {
		t.Errorf("kodik no_content feed wrong: %+v", p)
	}
	if p.Reason != "No content for this title on Kodik" {
		t.Errorf("kodik no_content reason = %q", p.Reason)
	}
}

func TestKodikFamily_ErrorOmitted(t *testing.T) {
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

func TestAnimelibFamily_EmptyNoContent(t *testing.T) {
	s := &Service{db: newDB(t, domain.ScraperProvider{Name: "animelib", Status: domain.StatusEnabled, Group: "ru", SupportsSub: true, SupportsDub: true}),
		catalog: fakeCatalog{anilib: nil}}
	fam, ok := s.animelibFamily(context.Background(), "uuid")
	if !ok || fam.Family != "animelib" || len(fam.Providers) != 1 {
		t.Fatalf("empty animelib should surface a present no_content family: ok=%v fam=%+v", ok, fam)
	}
	p := fam.Providers[0]
	if p.State != "no_content" || p.Selectable {
		t.Errorf("animelib no_content feed wrong: %+v", p)
	}
	if p.Reason != "No content for this title on AniLib" {
		t.Errorf("animelib no_content reason = %q", p.Reason)
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

func TestHanimeFamily_EmptyNoContent(t *testing.T) {
	s := &Service{db: newDB(t, domain.ScraperProvider{Name: "hanime", Status: domain.StatusEnabled, Group: "adult", SupportsRaw: true}),
		catalog: fakeCatalog{heps: nil}}
	fam, ok := s.hanimeFamily(context.Background(), "uuid")
	if !ok || fam.Family != "hanime" || len(fam.Providers) != 1 {
		t.Fatalf("empty hanime should surface a present no_content family: ok=%v fam=%+v", ok, fam)
	}
	p := fam.Providers[0]
	if p.State != "no_content" || p.Selectable {
		t.Errorf("hanime no_content feed wrong: %+v", p)
	}
	if p.Reason != "No content for this title on Hanime" {
		t.Errorf("hanime no_content reason = %q", p.Reason)
	}
}

func TestBuildFamilies_OrderAndBestEffort(t *testing.T) {
	db := newDB(t,
		domain.ScraperProvider{Name: "allanime", Status: domain.StatusEnabled, Group: "en", SupportsSub: true, PreferenceWeight: 90},
		domain.ScraperProvider{Name: "kodik-noads", Status: domain.StatusEnabled, Group: "ru", SupportsSub: true, SupportsDub: true},
		domain.ScraperProvider{Name: "hanime", Status: domain.StatusEnabled, Group: "adult", SupportsRaw: true},
	)

	// kodik present, animelib errors (omitted), hanime present → regrouped into
	// wire families ["others" (en+kodik), "18+" (hanime)]
	s := NewService(db, nil, fakeCatalog{
		kodik:     []domain.KodikTranslation{{ID: 1, Title: "T", Type: "voice"}},
		anilibErr: errors.New("not on animelib"),
		heps:      []domain.HanimeEpisode{{Slug: "ep-1"}},
		hstream:   &domain.HanimeStream{Sources: []domain.HanimeSource{{Height: "1080"}}},
	}, nil, nil, nil, nil)

	fams, err := s.buildFamilies(context.Background(), "uuid")
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(fams))
	for i, f := range fams {
		got[i] = f.Family
	}
	want := []string{"others", "18+"}
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

// ajRow builds a DEGRADED animejoy leg row (policy=manual → hacker-only via
// deriveProviderView), mirroring the seeded animejoy-sibnet (w25) /
// animejoy-allvideo (w20) DB rows.
func ajRow(name string, weight int) domain.ScraperProvider {
	return domain.ScraperProvider{
		Name: name, Status: domain.StatusDegraded, Policy: domain.PolicyManual,
		Group: "ru", PreferenceWeight: weight, SupportsSub: true,
	}
}

func TestAnimejoyLegFamily_BothLegsPresentDegraded(t *testing.T) {
	s := &Service{db: newDB(t, ajRow("animejoy-sibnet", 25), ajRow("animejoy-allvideo", 20))}
	teams := []domain.AnimejoyTeam{{ID: "0", Name: "AnimeJoy", HasSibnet: true, HasAllVideo: true}}

	sib, ok := s.animejoyLegFamily(context.Background(), teams, "animejoy-sibnet", "Sibnet", "sibnet")
	if !ok || sib.Family != "animejoy-sibnet" || len(sib.Providers) != 1 {
		t.Fatalf("sibnet family wrong: ok=%v fam=%+v", ok, sib)
	}
	av, ok := s.animejoyLegFamily(context.Background(), teams, "animejoy-allvideo", "AllVideo", "allvideo")
	if !ok || av.Family != "animejoy-allvideo" || len(av.Providers) != 1 {
		t.Fatalf("allvideo family wrong: ok=%v fam=%+v", ok, av)
	}

	for _, tc := range []struct {
		fam   domain.SourceFamily
		label string
	}{{sib, "Sibnet"}, {av, "AllVideo"}} {
		p := tc.fam.Providers[0]
		if p.DisplayName != tc.label {
			t.Errorf("%s display name = %q", tc.label, p.DisplayName)
		}
		if len(p.Variants) != 1 {
			t.Fatalf("%s want 1 variant, got %d", tc.label, len(p.Variants))
		}
		v := p.Variants[0]
		if v.Category != "sub" || v.SubDelivery != "hard" {
			t.Errorf("%s variant wrong: %+v", tc.label, v)
		}
		if v.Source != "discovered" || v.QualitySource != "unknown" {
			t.Errorf("%s variant provenance wrong: %+v", tc.label, v)
		}
		if v.Team == nil || v.Team.Name != "AnimeJoy" || v.Team.ID != "0" {
			t.Errorf("%s team wrong: %+v", tc.label, v.Team)
		}
		// DEGRADED (policy=manual) → hacker-only, still selectable in hacker mode.
		if p.State != "degraded" || !p.HackerOnly || !p.Selectable {
			t.Errorf("%s feed view wrong: state=%q hackerOnly=%v selectable=%v", tc.label, p.State, p.HackerOnly, p.Selectable)
		}
	}
}

// ajRowActive builds an animejoy leg row with the real (post-soak) seed shape —
// Status=enabled, default policy=auto/health=up (mirrors seed.go, promoted out
// of soak 2026-06-30) — UNLIKE ajRow's policy=manual fixture. Used by the
// no_content tests below: with policy=manual, deriveProviderView short-circuits
// to "degraded" regardless of hasContent, which would mask the no_content state
// this task adds — the real DB rows are policy=auto, so that's what these tests
// must seed to observe "no_content".
func ajRowActive(name string, weight int) domain.ScraperProvider {
	return domain.ScraperProvider{
		Name: name, Status: domain.StatusEnabled,
		Group: "ru", PreferenceWeight: weight, SupportsSub: true,
	}
}

func TestAnimejoyLegFamily_SibnetOnlyAllVideoNoContent(t *testing.T) {
	s := &Service{db: newDB(t, ajRowActive("animejoy-sibnet", 25), ajRowActive("animejoy-allvideo", 20))}
	teams := []domain.AnimejoyTeam{{ID: "0", HasSibnet: true, HasAllVideo: false}}

	if _, ok := s.animejoyLegFamily(context.Background(), teams, "animejoy-sibnet", "Sibnet", "sibnet"); !ok {
		t.Error("sibnet family should be present when a team has the sibnet leg")
	}
	av, ok := s.animejoyLegFamily(context.Background(), teams, "animejoy-allvideo", "AllVideo", "allvideo")
	if !ok || av.Family != "animejoy-allvideo" || len(av.Providers) != 1 {
		t.Fatalf("allvideo family must be a PRESENT no_content family when no team has the allvideo leg: ok=%v fam=%+v", ok, av)
	}
	p := av.Providers[0]
	if p.State != "no_content" || p.Selectable {
		t.Errorf("allvideo no_content feed wrong: %+v", p)
	}
	if p.Reason != "No content for this title on AllVideo" {
		t.Errorf("allvideo no_content reason = %q", p.Reason)
	}
}

func TestAnimejoyLegFamily_NoNameOmitsTeam(t *testing.T) {
	s := &Service{db: newDB(t, ajRow("animejoy-sibnet", 25))}
	teams := []domain.AnimejoyTeam{{ID: "0", Name: "", HasSibnet: true}}
	fam, ok := s.animejoyLegFamily(context.Background(), teams, "animejoy-sibnet", "Sibnet", "sibnet")
	if !ok {
		t.Fatal("sibnet family should be present")
	}
	if v := fam.Providers[0].Variants[0]; v.Team != nil {
		t.Errorf("empty team name must yield nil Team, got %+v", v.Team)
	}
}

func TestAnimejoyLegFamily_EmptyTeamsNoContent(t *testing.T) {
	s := &Service{db: newDB(t, ajRowActive("animejoy-sibnet", 25))}
	fam, ok := s.animejoyLegFamily(context.Background(), nil, "animejoy-sibnet", "Sibnet", "sibnet")
	if !ok || fam.Family != "animejoy-sibnet" || len(fam.Providers) != 1 {
		t.Fatalf("no teams → family must be a PRESENT no_content family, not absent: ok=%v fam=%+v", ok, fam)
	}
	p := fam.Providers[0]
	if p.State != "no_content" || p.Selectable {
		t.Errorf("animejoy-sibnet no_content feed wrong: %+v", p)
	}
	if p.Reason != "No content for this title on Sibnet" {
		t.Errorf("animejoy-sibnet no_content reason = %q", p.Reason)
	}
}

func TestAnimejoyLegFamily_OmittedWhenRowDisabled(t *testing.T) {
	s := &Service{db: newDB(t, domain.ScraperProvider{Name: "animejoy-sibnet", Status: domain.StatusDisabled, Group: "ru"})}
	teams := []domain.AnimejoyTeam{{ID: "0", HasSibnet: true}}
	if _, ok := s.animejoyLegFamily(context.Background(), teams, "animejoy-sibnet", "Sibnet", "sibnet"); ok {
		t.Fatal("animejoy leg family must be omitted when its DB row is disabled")
	}
}

func TestBuildFamilies_AnimejoyBothLegsInOrder(t *testing.T) {
	db := newDB(t,
		domain.ScraperProvider{Name: "allanime", Status: domain.StatusEnabled, Group: "en", SupportsSub: true, PreferenceWeight: 90},
		ajRow("animejoy-sibnet", 25),
		ajRow("animejoy-allvideo", 20),
	)
	s := NewService(db, nil, fakeCatalog{
		ajTeams: []domain.AnimejoyTeam{{ID: "0", Name: "AnimeJoy", HasSibnet: true, HasAllVideo: true}},
	}, nil, nil, nil, nil)

	fams, err := s.buildFamilies(context.Background(), "uuid")
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(fams))
	for i, f := range fams {
		got[i] = f.Family
	}
	want := []string{"others"}
	if len(got) != len(want) {
		t.Fatalf("families = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("families = %v, want %v", got, want)
		}
	}

	// The merged "others" family must preserve the pre-regroup assembly order:
	// EN (allanime) first, then the two AnimeJoy legs in the order buildFamilies
	// appends them (sibnet, then allvideo) — NOT the arbitrary DB insertion order.
	gotProviders := make([]string, len(fams[0].Providers))
	for i, p := range fams[0].Providers {
		gotProviders[i] = p.Provider
	}
	wantProviders := []string{"allanime", "animejoy-sibnet", "animejoy-allvideo"}
	if len(gotProviders) != len(wantProviders) {
		t.Fatalf("others providers = %v, want %v", gotProviders, wantProviders)
	}
	for i := range wantProviders {
		if gotProviders[i] != wantProviders[i] {
			t.Fatalf("others providers = %v, want %v (order must match EN, sibnet, allvideo)", gotProviders, wantProviders)
		}
	}
}

func TestBuildFamilies_AnimejoyDiscoveryErrorBothAbsent(t *testing.T) {
	db := newDB(t,
		domain.ScraperProvider{Name: "allanime", Status: domain.StatusEnabled, Group: "en", SupportsSub: true, PreferenceWeight: 90},
		ajRow("animejoy-sibnet", 25),
		ajRow("animejoy-allvideo", 20),
	)
	// Discovery error → GetAnimejoyTeams returns nil teams → both leg families
	// absent; the feed still builds (EN-only).
	s := NewService(db, nil, fakeCatalog{ajTeamsErr: errors.New("not on animejoy")}, nil, nil, nil, nil)
	fams, err := s.buildFamilies(context.Background(), "uuid")
	if err != nil {
		t.Fatal(err)
	}
	if len(fams) != 1 || fams[0].Family != "others" {
		t.Fatalf("discovery error should yield EN-only, got %+v", fams)
	}
}

func TestBuildFamilies_NilCatalogENOnly(t *testing.T) {
	// Shared-cache DSN (via newDB): buildFamilies now also fans out the
	// DB-row-driven ae/raw/adult families concurrently, so a plain `:memory:`
	// DSN would hand a concurrent reader its own unmigrated connection.
	db := newDB(t)
	s := NewService(db, nil, nil, nil, nil, nil, nil)
	fams, err := s.buildFamilies(context.Background(), "uuid")
	if err != nil {
		t.Fatal(err)
	}
	if len(fams) != 1 || fams[0].Family != "others" {
		t.Fatalf("nil catalog should yield EN-only, got %+v", fams)
	}
}

// AUTO-608: a brand-new non-EN provider row with NO dedicated family builder
// must still surface in /capabilities via the generic rowFamily default —
// the whole point of the roster-driven registry (no code change needed to
// wire up a new DB row).
func TestBuildFamilies_UnknownRosterRowGetsGenericFamily(t *testing.T) {
	db := newDB(t, domain.ScraperProvider{
		Name: "newru", Status: domain.StatusEnabled, Group: "ru",
		DisplayName: "NewRU", SupportsDub: true, PreferenceWeight: 10,
	})
	s := NewService(db, nil, nil, nil, nil, nil, nil)
	report, err := s.Report(context.Background(), "some-anime-id")
	if err != nil {
		t.Fatal(err)
	}
	var found *domain.ProviderCap
	for _, fam := range report.Families {
		for i := range fam.Providers {
			if fam.Providers[i].Provider == "newru" {
				found = &fam.Providers[i]
			}
		}
	}
	if found == nil {
		t.Fatal("new roster row must surface in /capabilities via the generic rowFamily default")
	}
	if found.DisplayName != "NewRU" {
		t.Fatalf("display_name not read from row: %q", found.DisplayName)
	}
}

// AUTO-608: the roster row's player_key must reach the wire so the FE can
// persist watch combos without a hardcoded provider→player switch. Uses the
// real hanime row/builder (zero-value fakeCatalog → empty episodes →
// no_content branch) since applyFeedFields runs on that path too.
func TestFeed_ExposesPlayerKey(t *testing.T) {
	db := newDB(t, domain.ScraperProvider{
		Name: "hanime", Status: domain.StatusEnabled, Group: "adult",
		PlayerKey: "hanime", DisplayName: "Hanime",
	})
	s := NewService(db, nil, fakeCatalog{}, nil, nil, nil, nil)
	report, err := s.Report(context.Background(), "anime-x")
	if err != nil {
		t.Fatal(err)
	}
	for _, fam := range report.Families {
		for _, cap := range fam.Providers {
			if cap.Provider == "hanime" && cap.PlayerKey != "hanime" {
				t.Fatalf("player_key not on the wire: %+v", cap)
			}
		}
	}
}

// kodik-iframe is the legacy Classic-Kodik iframe fallback surface, not an
// aePlayer capability — buildFamilies must explicitly skip it (registry maps
// it to a nil builder) rather than let the generic default pick it up.
func TestBuildFamilies_KodikIframeSkipped(t *testing.T) {
	db := newDB(t, domain.ScraperProvider{Name: "kodik-iframe", Status: domain.StatusEnabled, Group: "ru"})
	s := NewService(db, nil, nil, nil, nil, nil, nil)
	report, err := s.Report(context.Background(), "some-anime-id-2")
	if err != nil {
		t.Fatal(err)
	}
	for _, fam := range report.Families {
		for _, cap := range fam.Providers {
			if cap.Provider == "kodik-iframe" {
				t.Fatal("kodik-iframe is the Classic-Kodik iframe surface, not an aePlayer capability — must be skipped")
			}
		}
	}
}
