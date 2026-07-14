package service

import (
	"context"
	"errors"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
)

type fakeStreamer struct {
	out   string
	err   error
	calls int
}

func (f *fakeStreamer) Stream(_ context.Context, _, _ string, _ int, _ float64, onDelta func(string)) (string, int, error) {
	f.calls++
	if f.err != nil {
		// A real streamer may still have emitted partial deltas before failing;
		// exercise that by emitting once before returning the error.
		if f.out != "" {
			onDelta(f.out)
		}
		return f.out, 0, f.err
	}
	onDelta(f.out)
	return f.out, 55, nil
}

type fakeStore struct {
	created   *domain.Fanfic
	title     string
	body      string
	usage     int
	updateErr error
	failedID  string
	failedMsg string
}

func (s *fakeStore) Create(_ context.Context, f *domain.Fanfic) error {
	f.ID = "fixed-id"
	s.created = f
	return nil
}
func (s *fakeStore) UpdateResult(_ context.Context, _, title, content string, usage int) error {
	s.title, s.body, s.usage = title, content, usage
	return s.updateErr
}
func (s *fakeStore) MarkFailed(_ context.Context, id, msg string) error {
	s.failedID, s.failedMsg = id, msg
	return nil
}
func (s *fakeStore) Get(_ context.Context, _, _ string) (*domain.Fanfic, error) { return nil, nil }
func (s *fakeStore) AppendPart(_ context.Context, _, _, _ string, _, _ int) error {
	return nil
}

type noopQuota struct{}

func (noopQuota) Acquire(_ context.Context, _ string) (func(), error) { return func() {}, nil }

type stubQuota struct {
	release  func()
	err      error
	acquired bool
}

func (q *stubQuota) Acquire(_ context.Context, _ string) (func(), error) {
	q.acquired = true
	if q.release == nil {
		q.release = func() {}
	}
	return q.release, q.err
}

func TestGenerate_StreamsPersistsAndSplitsTitle(t *testing.T) {
	store := &fakeStore{}
	g := NewGenerator(&fakeStreamer{out: "# Тяжесть столетий\n\nКогда солнце..."}, store, noopQuota{}, nil, "llama-3.1-8b-instant", 24000, nil)

	var events []string
	emit := func(event string, _ any) error { events = append(events, event); return nil }

	req := domain.GenerateRequest{Anime: domain.AnimeRef{Title: "Frieren"}, Length: "oneshot", POV: "third", Rating: "mature", Language: "ru", SpotlightCredit: true}
	if err := g.Generate(context.Background(), "user-1", "arisu42", req, emit); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// Persisted with parsed title + body + usage.
	if store.title != "Тяжесть столетий" {
		t.Errorf("title = %q", store.title)
	}
	if store.usage != 55 {
		t.Errorf("usage = %d", store.usage)
	}
	if store.created.UserID != "user-1" || store.created.AnimeTitle != "Frieren" {
		t.Errorf("snapshot wrong: %+v", store.created)
	}
	// The author username (from JWT claims, threaded through by the handler)
	// and the user's spotlight opt-in must land on the persisted row.
	if store.created.AuthorUsername != "arisu42" {
		t.Errorf("AuthorUsername = %q, want arisu42", store.created.AuthorUsername)
	}
	if !store.created.SpotlightCredit {
		t.Error("expected SpotlightCredit = true")
	}
	// Explicitly false for user-generated fanfics, mirroring bot rows setting
	// it true (Task 6) — this is the user-path counterpart.
	if store.created.AIGenerated {
		t.Error("expected AIGenerated = false for a user-generated fanfic")
	}
	// Event order: meta, delta..., done.
	if events[0] != "meta" || events[len(events)-1] != "done" {
		t.Errorf("events = %v", events)
	}
}

// TestGenerate_SnapshotsCharactersAndTags asserts the JSON snapshot of the
// request's characters/tags actually reaches the persisted row, not just the
// scalar anime fields already covered above.
func TestGenerate_SnapshotsCharactersAndTags(t *testing.T) {
	store := &fakeStore{}
	g := NewGenerator(&fakeStreamer{out: "# T\n\nbody"}, store, noopQuota{}, nil, "llama-3.1-8b-instant", 24000, nil)

	req := domain.GenerateRequest{
		Anime:      domain.AnimeRef{Title: "Frieren", ShikimoriID: "52991"},
		Characters: []domain.CharacterRef{{ID: "1", Name: "Frieren"}, {Name: "Fern"}},
		Tags:       []string{"angst", "slow-burn"},
		Length:     "drabble", POV: "first", Rating: "teen", Language: "en",
	}
	if err := g.Generate(context.Background(), "user-2", "somebody", req, nil); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if store.created == nil {
		t.Fatal("expected a persisted snapshot")
	}
	gotChars := string(store.created.Characters)
	if !(len(gotChars) > 0 && contains(gotChars, "Frieren") && contains(gotChars, "Fern")) {
		t.Errorf("characters snapshot = %s", gotChars)
	}
	gotTags := string(store.created.Tags)
	if !(contains(gotTags, "angst") && contains(gotTags, "slow-burn")) {
		t.Errorf("tags snapshot = %s", gotTags)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

// TestGenerate_StreamerErrorMarksFailedAndEmitsError asserts that a streamer
// failure (network drop, Groq error, etc.) persists MarkFailed against the
// row created for THIS generation, emits an "error" event, and propagates
// the error to the caller (no silent swallow).
func TestGenerate_StreamerErrorMarksFailedAndEmitsError(t *testing.T) {
	store := &fakeStore{}
	streamErr := errors.New("groq status 503: upstream overloaded")
	g := NewGenerator(&fakeStreamer{err: streamErr}, store, noopQuota{}, nil, "llama-3.1-8b-instant", 24000, nil)

	var events []string
	emit := func(event string, _ any) error { events = append(events, event); return nil }

	req := domain.GenerateRequest{Anime: domain.AnimeRef{Title: "Frieren"}, Length: "oneshot", POV: "third", Rating: "mature", Language: "ru"}
	err := g.Generate(context.Background(), "user-1", "arisu42", req, emit)
	if !errors.Is(err, streamErr) {
		t.Fatalf("expected streamer error to propagate, got %v", err)
	}
	if store.failedID != "fixed-id" {
		t.Errorf("MarkFailed id = %q, want fixed-id", store.failedID)
	}
	if store.failedMsg != streamErr.Error() {
		t.Errorf("MarkFailed msg = %q", store.failedMsg)
	}
	// UpdateResult must NOT have been called on failure.
	if store.title != "" || store.usage != 0 {
		t.Errorf("expected no UpdateResult on failure, got title=%q usage=%d", store.title, store.usage)
	}
	if events[0] != "meta" || events[len(events)-1] != "error" {
		t.Errorf("events = %v", events)
	}
}

// TestGenerate_EmitErrorDoesNotAbortPersistence asserts that a disconnected
// client (every emit call returns an error, simulating a dropped SSE
// connection) does not abort server-side accumulation/persistence: the
// generation still completes, UpdateResult still runs, and Generate still
// returns nil.
func TestGenerate_EmitErrorDoesNotAbortPersistence(t *testing.T) {
	store := &fakeStore{}
	g := NewGenerator(&fakeStreamer{out: "# Title\n\nBody text"}, store, noopQuota{}, nil, "llama-3.1-8b-instant", 24000, nil)

	emitCalls := 0
	emit := func(event string, _ any) error {
		emitCalls++
		return errors.New("client disconnected")
	}

	req := domain.GenerateRequest{Anime: domain.AnimeRef{Title: "Frieren"}, Length: "oneshot", POV: "third", Rating: "mature", Language: "ru"}
	if err := g.Generate(context.Background(), "user-1", "arisu42", req, emit); err != nil {
		t.Fatalf("Generate should not fail on emit error, got %v", err)
	}
	if store.title != "Title" || store.body != "Body text" {
		t.Errorf("persistence did not complete: title=%q body=%q", store.title, store.body)
	}
	if store.usage != 55 {
		t.Errorf("usage = %d", store.usage)
	}
	// meta, delta, done all attempted despite each erroring.
	if emitCalls < 3 {
		t.Errorf("expected at least 3 emit attempts (meta/delta/done), got %d", emitCalls)
	}
}

// TestGenerate_QuotaErrorAbortsBeforePersistence asserts that a quota
// rejection (busy or exceeded) short-circuits before any row is created —
// it must not leave a dangling "generating" snapshot behind.
func TestGenerate_QuotaErrorAbortsBeforePersistence(t *testing.T) {
	store := &fakeStore{}
	q := &stubQuota{err: ErrBusy}
	g := NewGenerator(&fakeStreamer{out: "# T\n\nbody"}, store, q, nil, "llama-3.1-8b-instant", 24000, nil)

	req := domain.GenerateRequest{Anime: domain.AnimeRef{Title: "Frieren"}, Length: "oneshot", POV: "third", Rating: "mature", Language: "ru"}
	if err := g.Generate(context.Background(), "user-1", "arisu42", req, nil); !errors.Is(err, ErrBusy) {
		t.Fatalf("expected ErrBusy, got %v", err)
	}
	if !q.acquired {
		t.Error("expected Acquire to have been called")
	}
	if store.created != nil {
		t.Errorf("expected no row created on quota rejection, got %+v", store.created)
	}
}

// TestGenerate_QuotaErrorEmitsErrorEvent asserts that a quota rejection
// (ErrQuotaExceeded — daily cap, or ErrBusy — cross-tab/stale-lock) emits a
// single SSE "error" event before returning, so the handler's already-open
// 200 SSE stream doesn't just go silent on the client. It must also not
// create a fanfic row or reach the streamer.
func TestGenerate_QuotaErrorEmitsErrorEvent(t *testing.T) {
	store := &fakeStore{}
	streamed := &fakeStreamer{out: "# T\n\nbody"}
	q := &stubQuota{err: ErrQuotaExceeded}
	g := NewGenerator(streamed, store, q, nil, "llama-3.1-8b-instant", 24000, nil)

	var events []string
	var payloads []any
	emit := func(event string, data any) error {
		events = append(events, event)
		payloads = append(payloads, data)
		return nil
	}

	req := domain.GenerateRequest{Anime: domain.AnimeRef{Title: "Frieren"}, Length: "oneshot", POV: "third", Rating: "mature", Language: "ru"}
	err := g.Generate(context.Background(), "user-1", "arisu42", req, emit)
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("expected ErrQuotaExceeded, got %v", err)
	}
	if len(events) != 1 || events[0] != "error" {
		t.Fatalf("expected exactly one 'error' event, got %v", events)
	}
	msg, _ := payloads[0].(map[string]any)["message"].(string)
	if msg != ErrQuotaExceeded.Error() {
		t.Errorf("error payload message = %q, want %q", msg, ErrQuotaExceeded.Error())
	}
	if store.created != nil {
		t.Errorf("expected no row created on quota rejection, got %+v", store.created)
	}
	if store.title != "" || store.usage != 0 {
		t.Errorf("expected no persistence on quota rejection, got title=%q usage=%d", store.title, store.usage)
	}
	if streamed.calls != 0 {
		t.Errorf("expected streamer not to be called on quota rejection, got %d calls", streamed.calls)
	}
}
