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
	botDaily bool
	created  *domain.Fanfic
}

func (f *fakeRepo) ListEligibleSince(context.Context, time.Time) ([]domain.Fanfic, error) {
	return f.eligible, nil
}
func (f *fakeRepo) DailyBotExists(context.Context, time.Time) (bool, error) { return f.botDaily, nil }
func (f *fakeRepo) Create(_ context.Context, ff *domain.Fanfic) error {
	ff.ID = "new"
	f.created = ff
	return nil
}

type fakeMeta struct{}

func (fakeMeta) FetchMeta(context.Context, string, string) (catalog.AnimeMeta, error) {
	return catalog.AnimeMeta{Title: "Naruto", Poster: "http://p/x.jpg"}, nil
}

type fakeStream struct {
	err  error
	text string
}

func (f fakeStream) Stream(_ context.Context, _, _ string, _ int, _ float64, on func(string)) (string, int, error) {
	if f.err != nil {
		return "", 0, f.err
	}
	return f.text, 42, nil
}

type fakeAlerter struct{ sent []string }

func (a *fakeAlerter) Send(_ context.Context, s string) error { a.sent = append(a.sent, s); return nil }

func newDaily(repo dailyRepo, stream streamer, al *fakeAlerter) *DailyService {
	return NewDailyService(stream, repo, fakeMeta{}, al, "m", []string{"20"}, "ru", func() time.Time { return time.Unix(1700000000, 0) }, nil)
}

func TestEnsureDaily_UserFanficExists_NoOp(t *testing.T) {
	repo := &fakeRepo{eligible: []domain.Fanfic{{ID: "u", AIGenerated: false, Status: domain.StatusComplete}}}
	al := &fakeAlerter{}
	res, err := newDaily(repo, fakeStream{text: "# T\n\nBody"}, al).EnsureDaily(context.Background())
	if err != nil || res.Generated || res.Reason != "user_exists" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if repo.created != nil {
		t.Fatal("must not generate when a user fanfic exists")
	}
}

func TestEnsureDaily_GeneratesBotFanfic(t *testing.T) {
	repo := &fakeRepo{}
	al := &fakeAlerter{}
	res, err := newDaily(repo, fakeStream{text: "# Тайна\n\nОна вошла."}, al).EnsureDaily(context.Background())
	if err != nil || !res.Generated {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if repo.created == nil || !repo.created.AIGenerated || repo.created.Rating != "teen" ||
		repo.created.AuthorUsername != FanficBotUsername || !repo.created.SpotlightCredit ||
		repo.created.Status != domain.StatusComplete || repo.created.Title != "Тайна" {
		t.Fatalf("bad bot row: %+v", repo.created)
	}
}

func TestEnsureDaily_401_Alerts(t *testing.T) {
	repo := &fakeRepo{}
	al := &fakeAlerter{}
	stream := fakeStream{err: &groq.StatusError{Code: http.StatusUnauthorized, Body: "invalid_api_key"}}
	res, err := newDaily(repo, stream, al).EnsureDaily(context.Background())
	if err == nil || res.Generated {
		t.Fatalf("want error, res=%+v", res)
	}
	if len(al.sent) != 1 {
		t.Fatalf("want 1 alert, got %d", len(al.sent))
	}
}

func TestEnsureDaily_TransientError_NoAlert(t *testing.T) {
	repo := &fakeRepo{}
	al := &fakeAlerter{}
	_, err := newDaily(repo, fakeStream{err: errors.New("timeout")}, al).EnsureDaily(context.Background())
	if err == nil || len(al.sent) != 0 {
		t.Fatalf("transient must not alert; err=%v sent=%d", err, len(al.sent))
	}
}
