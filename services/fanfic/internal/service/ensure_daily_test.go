package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/catalog"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/groq"
)

type fakeRepo struct {
	eligible []domain.Fanfic
	created  *domain.Fanfic
}

func (f *fakeRepo) ListEligibleSince(context.Context, time.Time) ([]domain.Fanfic, error) {
	return f.eligible, nil
}
func (f *fakeRepo) Create(_ context.Context, ff *domain.Fanfic) error {
	ff.ID = "new"
	f.created = ff
	return nil
}

type fakeMeta struct{}

func (fakeMeta) FetchMeta(context.Context, string, string) (catalog.AnimeMeta, error) {
	return catalog.AnimeMeta{ID: "anime-uuid-1", Title: "Naruto", Poster: "http://p/x.jpg"}, nil
}

type fakeStream struct {
	err   error
	text  string
	calls int
}

func (f *fakeStream) Stream(_ context.Context, _, _ string, _ int, _ float64, on func(string)) (string, int, error) {
	f.calls++
	if f.err != nil {
		return "", 0, f.err
	}
	return f.text, 42, nil
}

type fakeAlerter struct{ sent []string }

func (a *fakeAlerter) Send(_ context.Context, s string) error { a.sent = append(a.sent, s); return nil }

// testNow = 2023-11-14 22:13:20 UTC — the fixed clock every EnsureDaily test
// runs under; CreatedAt values below are chosen relative to this instant.
var testNow = time.Unix(1700000000, 0).UTC()

func newDaily(repo dailyRepo, stream streamer, al *fakeAlerter) *DailyService {
	return NewDailyService(stream, repo, fakeMeta{}, al, "m", []string{"20"}, "ru", func() time.Time { return testNow }, nil)
}

func TestEnsureDaily_UserFanficExists_StillGeneratesDailyBot(t *testing.T) {
	// A user fanfic must NOT suppress bot generation: the bot is the guaranteed
	// fallback for the day the user fanfic ages out, and the Groq call is the
	// daily API-key health probe. PickDaily still prefers the user fanfic.
	repo := &fakeRepo{eligible: []domain.Fanfic{{ID: "u", AIGenerated: false, Status: domain.StatusComplete, CreatedAt: testNow.Add(-2 * time.Hour)}}}
	al := &fakeAlerter{}
	stream := &fakeStream{text: "# T\n\nBody"}
	res, err := newDaily(repo, stream, al).EnsureDaily(context.Background())
	if err != nil || !res.Generated || res.Reason != "generated" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if repo.created == nil || stream.calls != 1 {
		t.Fatalf("user fanfic must not suppress the daily bot; created=%v calls=%d", repo.created, stream.calls)
	}
}

func TestEnsureDaily_BotFanficExistsToday_NoOp(t *testing.T) {
	repo := &fakeRepo{eligible: []domain.Fanfic{{ID: "b", AIGenerated: true, Status: domain.StatusComplete, CreatedAt: testNow.Add(-2 * time.Hour)}}}
	al := &fakeAlerter{}
	stream := &fakeStream{text: "# T\n\nBody"}
	res, err := newDaily(repo, stream, al).EnsureDaily(context.Background())
	if err != nil || res.Generated || res.Reason != "bot_exists" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if repo.created != nil {
		t.Fatal("must not generate a second bot fanfic when one already exists today")
	}
	if stream.calls != 0 {
		t.Fatalf("must not call groq on the bot_exists no-op path; calls=%d", stream.calls)
	}
}

func TestEnsureDaily_YesterdaysBotInWindow_Generates(t *testing.T) {
	// Regression (2026-07-17 outage): a bot generated shortly AFTER yesterday's
	// cron tick is still inside the eligibility window when today's cron runs.
	// Counting it as "bot_exists" skips generation — then it ages out and the
	// site serves 404 for the rest of the day. Only a bot created on the
	// CURRENT UTC day may satisfy the idempotence check.
	repo := &fakeRepo{eligible: []domain.Fanfic{{ID: "b-yday", AIGenerated: true, Status: domain.StatusComplete, CreatedAt: testNow.Add(-23 * time.Hour)}}}
	al := &fakeAlerter{}
	stream := &fakeStream{text: "# T\n\nBody"}
	res, err := newDaily(repo, stream, al).EnsureDaily(context.Background())
	if err != nil || !res.Generated || res.Reason != "generated" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if repo.created == nil || stream.calls != 1 {
		t.Fatalf("yesterday's bot must not satisfy today's idempotence check; created=%v calls=%d", repo.created, stream.calls)
	}
}

func TestEnsureDaily_GeneratesBotFanfic(t *testing.T) {
	repo := &fakeRepo{}
	al := &fakeAlerter{}
	res, err := newDaily(repo, &fakeStream{text: "# Тайна\n\nОна вошла."}, al).EnsureDaily(context.Background())
	if err != nil || !res.Generated {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if repo.created == nil || !repo.created.AIGenerated || repo.created.Rating != "teen" ||
		repo.created.AuthorUsername != FanficBotUsername || !repo.created.SpotlightCredit ||
		repo.created.Status != domain.StatusComplete || repo.created.Title != "Тайна" {
		t.Fatalf("bad bot row: %+v", repo.created)
	}
	// AnimeID (uuid-typed column) must be populated from the catalog lookup —
	// regression test for the fanfic_daily job's Postgres 22P02 failure
	// ("invalid input syntax for type uuid: \"\"") when it was left unset.
	if repo.created.AnimeID != "anime-uuid-1" {
		t.Fatalf("AnimeID = %q; want catalog uuid to flow through from FetchMeta", repo.created.AnimeID)
	}
}

func TestEnsureDaily_401_Alerts(t *testing.T) {
	repo := &fakeRepo{}
	al := &fakeAlerter{}
	stream := &fakeStream{err: &groq.StatusError{Code: http.StatusUnauthorized, Body: "invalid_api_key"}}
	res, err := newDaily(repo, stream, al).EnsureDaily(context.Background())
	if err == nil || res.Generated {
		t.Fatalf("want error, res=%+v", res)
	}
	if len(al.sent) != 1 {
		t.Fatalf("want 1 alert, got %d", len(al.sent))
	}
}

func TestEnsureDaily_403_Alerts(t *testing.T) {
	repo := &fakeRepo{}
	al := &fakeAlerter{}
	stream := &fakeStream{err: &groq.StatusError{Code: http.StatusForbidden, Body: "forbidden"}}
	res, err := newDaily(repo, stream, al).EnsureDaily(context.Background())
	if err == nil || res.Generated {
		t.Fatalf("want error, res=%+v", res)
	}
	if len(al.sent) != 1 {
		t.Fatalf("want 1 alert on 403, got %d", len(al.sent))
	}
}

func TestEnsureDaily_TransientError_NoAlert(t *testing.T) {
	repo := &fakeRepo{}
	al := &fakeAlerter{}
	_, err := newDaily(repo, &fakeStream{err: errors.New("timeout")}, al).EnsureDaily(context.Background())
	if err == nil || len(al.sent) != 0 {
		t.Fatalf("transient must not alert; err=%v sent=%d", err, len(al.sent))
	}
}
