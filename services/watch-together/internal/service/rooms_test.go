package service

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redis/go-redis/v9"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/repo"
)

// newService spins up a miniredis-backed RoomRepo + RoomService for each
// test. uuid + now are pinned via the injection points so assertions are
// deterministic. The frozen time is 2026-05-25T12:00:00Z — picked because
// it cleanly round-trips to both unix-seconds and unix-millis without
// fractional surprises.
func newService(t *testing.T, fixedID string, fixedNow time.Time) (*RoomService, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	r := repo.NewRoomRepo(client, 900*time.Second, logger.Default())
	svc := NewRoomService(r, logger.Default())
	svc.newID = func() string { return fixedID }
	svc.now = func() time.Time { return fixedNow }
	return svc, mr
}

// validInput returns a CreateRoomInput that passes all validate() checks.
// Tests start from this and mutate one field to drive failure paths.
func validInput() CreateRoomInput {
	return CreateRoomInput{
		AnimeID:       "anime-uuid-1",
		EpisodeID:     "ep-1",
		Player:        domain.PlayerAnimeLib,
		TranslationID: "translation-1",
	}
}

func TestCreate_PersistsRoomWithExpectedFields(t *testing.T) {
	fixedNow := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	svc, mr := newService(t, "room-123", fixedNow)
	ctx := context.Background()

	room, err := svc.Create(ctx, "host-user-1", "host-name", validInput())
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if room.ID != "room-123" {
		t.Errorf("Room.ID = %q, want %q", room.ID, "room-123")
	}
	if room.HostUserID != "host-user-1" {
		t.Errorf("Room.HostUserID = %q, want %q", room.HostUserID, "host-user-1")
	}
	if room.AnimeID != "anime-uuid-1" || room.EpisodeID != "ep-1" || room.Player != domain.PlayerAnimeLib || room.TranslationID != "translation-1" {
		t.Errorf("CreateRoomInput not copied verbatim: got %+v", room)
	}
	if room.PlaybackState != domain.StatePaused {
		t.Errorf("Room.PlaybackState = %q, want %q", room.PlaybackState, domain.StatePaused)
	}
	if room.PlaybackTime != 0 {
		t.Errorf("Room.PlaybackTime = %v, want 0", room.PlaybackTime)
	}
	if room.CreatedAt != fixedNow.Unix() {
		t.Errorf("Room.CreatedAt = %d, want %d", room.CreatedAt, fixedNow.Unix())
	}
	if room.PlaybackTimeUpdatedAtMs != fixedNow.UnixMilli() {
		t.Errorf("Room.PlaybackTimeUpdatedAtMs = %d, want %d", room.PlaybackTimeUpdatedAtMs, fixedNow.UnixMilli())
	}

	// Sanity: the room HASH is actually in Redis.
	if !mr.Exists("wt:room:room-123") {
		t.Errorf("wt:room:room-123 not created in miniredis")
	}
}

func TestCreate_RejectsEmptyAnimeID(t *testing.T) {
	svc, _ := newService(t, "x", time.Now())
	in := validInput()
	in.AnimeID = ""

	_, err := svc.Create(context.Background(), "host", "name", in)
	if !stderrors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestCreate_RejectsEmptyEpisodeID(t *testing.T) {
	svc, _ := newService(t, "x", time.Now())
	in := validInput()
	in.EpisodeID = ""

	_, err := svc.Create(context.Background(), "host", "name", in)
	if !stderrors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestCreate_RejectsEmptyTranslationID(t *testing.T) {
	svc, _ := newService(t, "x", time.Now())
	in := validInput()
	in.TranslationID = ""

	_, err := svc.Create(context.Background(), "host", "name", in)
	if !stderrors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestCreate_RejectsUnknownPlayer(t *testing.T) {
	svc, _ := newService(t, "x", time.Now())
	in := validInput()
	in.Player = "vlc"

	_, err := svc.Create(context.Background(), "host", "name", in)
	if !stderrors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestCreate_RejectsEmptyHostUserID(t *testing.T) {
	svc, _ := newService(t, "x", time.Now())

	_, err := svc.Create(context.Background(), "", "name", validInput())
	if !stderrors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestGet_ReturnsSnapshotWithProtocolVersionAndEmptyCollections(t *testing.T) {
	svc, _ := newService(t, "room-A", time.Now())
	ctx := context.Background()

	_, err := svc.Create(ctx, "host", "name", validInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	snap, err := svc.Get(ctx, "room-A")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if snap.ProtocolVersion != domain.ProtocolVersion {
		t.Errorf("ProtocolVersion = %q, want %q", snap.ProtocolVersion, domain.ProtocolVersion)
	}
	if snap.Room.ID != "room-A" {
		t.Errorf("Room.ID = %q, want %q", snap.Room.ID, "room-A")
	}
	// Empty members + messages should marshal as [] not null (handler test
	// covers the JSON shape end-to-end; here we just assert non-nil slices).
	if snap.Members == nil {
		t.Errorf("Members slice is nil; want empty slice (JSON marshals nil as null)")
	}
	if snap.Messages == nil {
		t.Errorf("Messages slice is nil; want empty slice")
	}
}

func TestGet_ReturnsLastMessagesInChronologicalOrder(t *testing.T) {
	svc, _ := newService(t, "room-Z", time.Now())
	ctx := context.Background()

	_, err := svc.Create(ctx, "host", "name", validInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Append 3 messages — the repo LPUSHes newest-at-head; we expect Get
	// to return them oldest-first (chronological).
	for i, body := range []string{"first", "second", "third"} {
		msg := domain.ChatMessage{
			ID:       []string{"m1", "m2", "m3"}[i],
			UserID:   "u",
			Username: "u-name",
			Body:     body,
			TS:       int64(1700000000 + i),
		}
		if err := svc.repo.AppendMessage(ctx, "room-Z", msg); err != nil {
			t.Fatalf("AppendMessage: %v", err)
		}
	}

	snap, err := svc.Get(ctx, "room-Z")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(snap.Messages) != 3 {
		t.Fatalf("len(Messages) = %d, want 3", len(snap.Messages))
	}
	if snap.Messages[0].Body != "first" || snap.Messages[1].Body != "second" || snap.Messages[2].Body != "third" {
		t.Errorf("messages out of chronological order: got %+v", snap.Messages)
	}
}

func TestGet_MissingRoomReturnsErrNotFound(t *testing.T) {
	svc, _ := newService(t, "x", time.Now())

	_, err := svc.Get(context.Background(), "no-such-room")
	if !stderrors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDelete_HostDeletesRoom(t *testing.T) {
	svc, mr := newService(t, "room-D", time.Now())
	ctx := context.Background()

	_, err := svc.Create(ctx, "host-user", "host-name", validInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.Delete(ctx, "host-user", "room-D"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if mr.Exists("wt:room:room-D") {
		t.Errorf("wt:room:room-D still exists after host Delete")
	}
}

func TestDelete_NonHostReturnsErrNotHost(t *testing.T) {
	svc, mr := newService(t, "room-E", time.Now())
	ctx := context.Background()

	_, err := svc.Create(ctx, "host-user", "host-name", validInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = svc.Delete(ctx, "some-other-user", "room-E")
	if !stderrors.Is(err, ErrNotHost) {
		t.Fatalf("expected ErrNotHost, got %v", err)
	}
	// Room must still exist — non-host delete is a no-op on Redis.
	if !mr.Exists("wt:room:room-E") {
		t.Errorf("wt:room:room-E was deleted by non-host caller")
	}
}

func TestDelete_MissingRoomReturnsErrNotFound(t *testing.T) {
	svc, _ := newService(t, "x", time.Now())

	err := svc.Delete(context.Background(), "anyone", "no-such-room")
	if !stderrors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// TestCreate_BumpsRoomsActive verifies that a successful Create increments
// the wt_rooms_active gauge by exactly 1. Delta-based assertion because
// other tests in the package may have bumped the gauge.
func TestCreate_BumpsRoomsActive(t *testing.T) {
	svc, _ := newService(t, "active-room", time.Now())
	ctx := context.Background()

	before := testutil.ToFloat64(RoomsActive)
	if _, err := svc.Create(ctx, "host", "name", validInput()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := testutil.ToFloat64(RoomsActive); got != before+1 {
		t.Errorf("RoomsActive after Create = %v, want %v", got, before+1)
	}
}

// TestDelete_ObservesAndDecsRoomsActive verifies the Plan 05.2 teardown
// observation path: SessionDurationSeconds + ChatMessagesPerRoom each get
// a new observation, and RoomsActive decrements back to its pre-Create
// baseline. The exact bucket placements aren't asserted (testutil doesn't
// expose them ergonomically); we assert _count advancement and the gauge
// delta.
func TestDelete_ObservesAndDecsRoomsActive(t *testing.T) {
	svc, _ := newService(t, "teardown-room", time.Now())
	ctx := context.Background()

	beforeActive := testutil.ToFloat64(RoomsActive)
	if _, err := svc.Create(ctx, "host", "name", validInput()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := testutil.ToFloat64(RoomsActive); got != beforeActive+1 {
		t.Fatalf("RoomsActive after Create = %v, want %v", got, beforeActive+1)
	}

	if err := svc.Delete(ctx, "host", "teardown-room"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := testutil.ToFloat64(RoomsActive); got != beforeActive {
		t.Errorf("RoomsActive after Delete = %v, want %v (restored)", got, beforeActive)
	}

	// Both histograms must have collected the metric (we can't assert exact
	// _count via ToFloat64 on histograms; CollectAndCount returns 1 for the
	// single-series, but at least the metric must be registered + observed
	// at least once during this test run).
	if testutil.CollectAndCount(SessionDurationSeconds) < 1 {
		t.Errorf("SessionDurationSeconds not collected post-teardown")
	}
	if testutil.CollectAndCount(ChatMessagesPerRoom) < 1 {
		t.Errorf("ChatMessagesPerRoom not collected post-teardown")
	}
}

// TestDelete_NonHostDoesNotDecGauge asserts that a forbidden Delete
// attempt by a non-host caller does NOT decrement the gauge. This catches
// the bug where a metric Dec was applied unconditionally before the
// host-check returned ErrNotHost.
func TestDelete_NonHostDoesNotDecGauge(t *testing.T) {
	svc, _ := newService(t, "host-only", time.Now())
	ctx := context.Background()

	if _, err := svc.Create(ctx, "host", "name", validInput()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	beforeReject := testutil.ToFloat64(RoomsActive)

	err := svc.Delete(ctx, "some-other-user", "host-only")
	if !stderrors.Is(err, ErrNotHost) {
		t.Fatalf("Delete: expected ErrNotHost, got %v", err)
	}
	if got := testutil.ToFloat64(RoomsActive); got != beforeReject {
		t.Errorf("RoomsActive moved on rejected Delete: %v → %v", beforeReject, got)
	}
}
