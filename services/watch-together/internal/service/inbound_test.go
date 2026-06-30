package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redis/go-redis/v9"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/repo"
)

// ----------------------------------------------------------------------------
// Test scaffolding
//
// We use a real miniredis-backed RoomRepo (so the Redis-write side effects
// actually exercise production code) plus a fakeHub that records every
// Broadcast / SendTo call for assertion. The DriftEngine and RateLimiter are
// real (we want their actual behavior exercised), constructed fresh per test.
// ----------------------------------------------------------------------------

// fakeHubCall captures one Broadcast or SendTo invocation.
type fakeHubCall struct {
	method        string // "Broadcast" or "SendTo"
	roomID        string
	userID        string // SendTo only
	excludeUserID string // Broadcast only
	env           domain.Envelope
}

// fakeHub satisfies HubFanout. Thread-safe so concurrent handler tests
// can assert without races.
type fakeHub struct {
	mu    sync.Mutex
	calls []fakeHubCall
}

func newFakeHub() *fakeHub { return &fakeHub{} }

func (h *fakeHub) Broadcast(_ context.Context, roomID string, env domain.Envelope, excludeUserID string) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, fakeHubCall{
		method:        "Broadcast",
		roomID:        roomID,
		excludeUserID: excludeUserID,
		env:           env,
	})
	return 1, nil
}

func (h *fakeHub) SendTo(_ context.Context, roomID, userID string, env domain.Envelope) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, fakeHubCall{
		method: "SendTo",
		roomID: roomID,
		userID: userID,
		env:    env,
	})
	return 1, nil
}

func (h *fakeHub) snapshot() []fakeHubCall {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]fakeHubCall, len(h.calls))
	copy(out, h.calls)
	return out
}

func (h *fakeHub) findFirst(method, msgType string) (fakeHubCall, bool) {
	for _, c := range h.snapshot() {
		if c.method == method && c.env.Type == msgType {
			return c, true
		}
	}
	return fakeHubCall{}, false
}

// fakeCatalog satisfies CatalogValidator. The validateFn lets each test
// install a per-case stub (always-valid / always-invalid / per-call decision)
// without needing an httptest server. callCount is bumped on every call so
// tests can assert that the catalog was (or wasn't) hit at all.
type fakeCatalog struct {
	mu         sync.Mutex
	calls      []fakeCatalogCall
	validateFn func(shikimoriID, player, episodeID, translationID, watchType string) (ValidateResult, error)
}

// fakeCatalogCall captures one ValidateEpisode invocation. Tests use it to
// assert that the router passed through the right Room state (anime_id /
// player / episode_id / translation_id) to the catalog.
type fakeCatalogCall struct {
	ShikimoriID, Player, EpisodeID, TranslationID, WatchType string
}

func (f *fakeCatalog) ValidateEpisode(
	_ context.Context,
	shikimoriID, player, episodeID, translationID, watchType string,
) (ValidateResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, fakeCatalogCall{
		ShikimoriID:   shikimoriID,
		Player:        player,
		EpisodeID:     episodeID,
		TranslationID: translationID,
		WatchType:     watchType,
	})
	fn := f.validateFn
	f.mu.Unlock()
	if fn == nil {
		return ValidateResult{Valid: true}, nil
	}
	return fn(shikimoriID, player, episodeID, translationID, watchType)
}

func (f *fakeCatalog) snapshot() []fakeCatalogCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]fakeCatalogCall, len(f.calls))
	copy(out, f.calls)
	return out
}

// alwaysValid configures fakeCatalog to always return Valid=true.
func (f *fakeCatalog) alwaysValid() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.validateFn = func(string, string, string, string, string) (ValidateResult, error) {
		return ValidateResult{Valid: true}, nil
	}
}

// alwaysInvalid configures fakeCatalog to always return Valid=false with the
// given reason (one of the ErrCode*Unavailable constants).
func (f *fakeCatalog) alwaysInvalid(reason string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.validateFn = func(string, string, string, string, string) (ValidateResult, error) {
		return ValidateResult{Valid: false, Reason: reason}, nil
	}
}

// alwaysError configures fakeCatalog to always return a transport error.
func (f *fakeCatalog) alwaysError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.validateFn = func(string, string, string, string, string) (ValidateResult, error) {
		return ValidateResult{}, err
	}
}

// routerFixture bundles the moving parts a router test needs.
type routerFixture struct {
	repo    *repo.RoomRepo
	hub     *fakeHub
	drift   *DriftEngine
	rl      *RateLimiter
	catalog *fakeCatalog
	router  *InboundRouter
	mr      *miniredis.Miniredis
	now     time.Time
}

// fixedID returns a stable chat-message id so test asserts are deterministic.
const fixedChatID = "msg-fixed-id"

// newRouterFixture creates a real-Redis-backed router with a pinned clock
// and pinned newID. Caller seeds the room state explicitly.
func newRouterFixture(t *testing.T) *routerFixture {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	log := logger.Default()
	r := repo.NewRoomRepo(client, 900*time.Second, log)
	hub := newFakeHub()
	drift := NewDriftEngine(log)
	rl := NewRateLimiter()
	// Default fake catalog returns Valid=true on every call. Tests that need
	// a different behavior override via fx.catalog.alwaysInvalid / alwaysError.
	catalog := &fakeCatalog{}
	catalog.alwaysValid()
	router := NewInboundRouter(r, hub, drift, rl, catalog, log)

	// Pinned wall-clock for deterministic server_ts / chat TS assertions.
	fixedNow := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	router.SetClockForTest(func() time.Time { return fixedNow })
	router.SetIDProviderForTest(func() string { return fixedChatID })

	return &routerFixture{
		repo:    r,
		hub:     hub,
		drift:   drift,
		rl:      rl,
		catalog: catalog,
		router:  router,
		mr:      mr,
		now:     fixedNow,
	}
}

// seedRoom inserts a Room HASH for the test.
func (fx *routerFixture) seedRoom(t *testing.T, room domain.Room) {
	t.Helper()
	if err := fx.repo.CreateRoom(context.Background(), &room); err != nil {
		t.Fatalf("seed room: %v", err)
	}
}

// seedMember seeds a member entry so heartbeat tests find an existing meta.
func (fx *routerFixture) seedMember(t *testing.T, roomID, userID string, meta domain.MemberMeta) {
	t.Helper()
	if err := fx.repo.AddMember(context.Background(), roomID, userID, meta); err != nil {
		t.Fatalf("seed member: %v", err)
	}
}

func (fx *routerFixture) defaultRoom(roomID string) domain.Room {
	return domain.Room{
		ID:                      roomID,
		CreatedAt:               fx.now.Unix(),
		AnimeID:                 "anime-1",
		EpisodeID:               "ep-1",
		Player:                  domain.PlayerAnimeLib,
		TranslationID:           "trans-1",
		PlaybackState:           domain.StatePaused,
		PlaybackTime:            0,
		PlaybackTimeUpdatedAtMs: fx.now.UnixMilli(),
		HostUserID:              "host",
	}
}

// dispatchJSON marshals body into Envelope.Data and runs Dispatch.
func (fx *routerFixture) dispatchJSON(t *testing.T, conn ConnectionCtx, msgType string, body interface{}) {
	t.Helper()
	var data json.RawMessage
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		data = raw
	}
	fx.router.Dispatch(conn, domain.Envelope{Type: msgType, Data: data})
}

func aliceConn(roomID string) ConnectionCtx {
	return ConnectionCtx{RoomID: roomID, UserID: "alice", Username: "Alice"}
}

// ----------------------------------------------------------------------------
// Test 1 — playback:play updates room HASH + broadcasts playback:event.
// ----------------------------------------------------------------------------

func TestRouter_PlaybackPlay_UpdatesAndBroadcasts(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "room-1"
	fx.seedRoom(t, fx.defaultRoom(roomID))

	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgPlaybackPlay, map[string]interface{}{"time": 42.5})

	// Redis state.
	room, err := fx.repo.GetRoom(context.Background(), roomID)
	if err != nil {
		t.Fatalf("GetRoom: %v", err)
	}
	if room.PlaybackState != domain.StatePlaying {
		t.Errorf("PlaybackState = %q, want %q", room.PlaybackState, domain.StatePlaying)
	}
	if room.PlaybackTime != 42.5 {
		t.Errorf("PlaybackTime = %v, want 42.5", room.PlaybackTime)
	}
	if room.PlaybackTimeUpdatedAtMs != fx.now.UnixMilli() {
		t.Errorf("PlaybackTimeUpdatedAtMs = %d, want %d", room.PlaybackTimeUpdatedAtMs, fx.now.UnixMilli())
	}

	// Hub fanout.
	call, ok := fx.hub.findFirst("Broadcast", domain.MsgPlaybackEvent)
	if !ok {
		t.Fatalf("no Broadcast(playback:event) recorded; calls=%v", fx.hub.snapshot())
	}
	if call.excludeUserID != "alice" {
		t.Errorf("excludeUserID = %q, want alice (sender excluded)", call.excludeUserID)
	}
	var evt domain.PlaybackEventData
	if err := json.Unmarshal(call.env.Data, &evt); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if evt.Kind != "play" || evt.Time != 42.5 || evt.ByUserID != "alice" {
		t.Errorf("event = %+v, want kind=play time=42.5 by=alice", evt)
	}
}

// ----------------------------------------------------------------------------
// Test 2 — playback:pause sets state=paused.
// ----------------------------------------------------------------------------

func TestRouter_PlaybackPause_SetsPaused(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "room-2"
	r := fx.defaultRoom(roomID)
	r.PlaybackState = domain.StatePlaying
	fx.seedRoom(t, r)

	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgPlaybackPause, map[string]interface{}{"time": 100.0})

	room, _ := fx.repo.GetRoom(context.Background(), roomID)
	if room.PlaybackState != domain.StatePaused {
		t.Fatalf("PlaybackState = %q, want paused", room.PlaybackState)
	}

	call, ok := fx.hub.findFirst("Broadcast", domain.MsgPlaybackEvent)
	if !ok {
		t.Fatal("no playback:event broadcast")
	}
	var evt domain.PlaybackEventData
	_ = json.Unmarshal(call.env.Data, &evt)
	if evt.Kind != "pause" {
		t.Errorf("Kind = %q, want pause", evt.Kind)
	}
}

// ----------------------------------------------------------------------------
// Test 3 — playback:seek within rate limit updates state + broadcasts.
// ----------------------------------------------------------------------------

func TestRouter_PlaybackSeek_WithinRate_BroadcastsEvent(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "room-3"
	fx.seedRoom(t, fx.defaultRoom(roomID))

	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgPlaybackSeek, map[string]interface{}{"time": 75.0})

	room, _ := fx.repo.GetRoom(context.Background(), roomID)
	if room.PlaybackTime != 75.0 {
		t.Errorf("PlaybackTime = %v, want 75", room.PlaybackTime)
	}

	call, ok := fx.hub.findFirst("Broadcast", domain.MsgPlaybackEvent)
	if !ok {
		t.Fatal("no playback:event broadcast")
	}
	if call.excludeUserID != "alice" {
		t.Errorf("excludeUserID = %q, want alice", call.excludeUserID)
	}
	var evt domain.PlaybackEventData
	_ = json.Unmarshal(call.env.Data, &evt)
	if evt.Kind != "seek" || evt.Time != 75.0 {
		t.Errorf("event = %+v, want kind=seek time=75", evt)
	}
}

// ----------------------------------------------------------------------------
// Test 4 — playback:seek over 1/sec rate limit yields RATE_LIMITED error
// to sender ONLY; no state change.
// ----------------------------------------------------------------------------

func TestRouter_PlaybackSeek_OverRate_RateLimited(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "room-4"
	fx.seedRoom(t, fx.defaultRoom(roomID))

	// First seek succeeds.
	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgPlaybackSeek, map[string]interface{}{"time": 10.0})
	// Second seek (no time passed) → rate-limited.
	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgPlaybackSeek, map[string]interface{}{"time": 20.0})

	// Redis still reflects first seek only.
	room, _ := fx.repo.GetRoom(context.Background(), roomID)
	if room.PlaybackTime != 10.0 {
		t.Errorf("PlaybackTime = %v, want 10 (second seek should have been rejected)", room.PlaybackTime)
	}

	// Expect a SendTo(error: RATE_LIMITED) for the second seek.
	var found bool
	for _, c := range fx.hub.snapshot() {
		if c.method != "SendTo" || c.env.Type != domain.MsgError {
			continue
		}
		var e domain.ErrorData
		_ = json.Unmarshal(c.env.Data, &e)
		if e.Code == domain.ErrCodeRateLimited && c.userID == "alice" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no RATE_LIMITED SendTo recorded; calls=%v", fx.hub.snapshot())
	}
}

// ----------------------------------------------------------------------------
// Test 5 — playback:time_tick with drift in soft band → SendTo(correction)
// with time ≈ expected, ServerTS == fixed now.
// ----------------------------------------------------------------------------

func TestRouter_TimeTick_SoftDrift_SendsCorrection(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "room-5"

	// Room: playing, time=100, updated 2s ago in wall-clock terms.
	// To make wall-clock advance "real", set updatedAtMs to fx.now - 2000ms.
	r := fx.defaultRoom(roomID)
	r.PlaybackState = domain.StatePlaying
	r.PlaybackTime = 100.0
	r.PlaybackTimeUpdatedAtMs = fx.now.UnixMilli() - 2000
	fx.seedRoom(t, r)

	// Reported time = 99.5 → drift = |99.5 - 102| = 2.5 (soft band).
	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgPlaybackTimeTick, map[string]interface{}{"time": 99.5})

	call, ok := fx.hub.findFirst("SendTo", domain.MsgPlaybackCorrection)
	if !ok {
		t.Fatalf("no playback:correction SendTo; calls=%v", fx.hub.snapshot())
	}
	if call.userID != "alice" {
		t.Errorf("SendTo userID = %q, want alice", call.userID)
	}
	var c domain.PlaybackCorrectionData
	_ = json.Unmarshal(call.env.Data, &c)
	if c.Time < 101.9 || c.Time > 102.1 {
		t.Errorf("correction.Time = %v, want ~102", c.Time)
	}
	if c.ServerTS != fx.now.UnixMilli() {
		t.Errorf("correction.ServerTS = %d, want %d", c.ServerTS, fx.now.UnixMilli())
	}
}

// ----------------------------------------------------------------------------
// Test 6 — chat:message under cap → AppendMessage + Broadcast to ALL.
// ----------------------------------------------------------------------------

func TestRouter_ChatMessage_PersistsAndBroadcasts(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "room-6"
	fx.seedRoom(t, fx.defaultRoom(roomID))

	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgChatMessage, map[string]interface{}{"body": "hi"})

	// Redis should have the message.
	msgs, err := fx.repo.GetMessages(context.Background(), roomID, 10)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("len(msgs) = %d, want 1; got=%+v", len(msgs), msgs)
	}
	if msgs[0].Body != "hi" || msgs[0].UserID != "alice" || msgs[0].ID != fixedChatID {
		t.Errorf("persisted message = %+v", msgs[0])
	}
	if msgs[0].TS != fx.now.UnixMilli() {
		t.Errorf("TS = %d, want %d", msgs[0].TS, fx.now.UnixMilli())
	}

	call, ok := fx.hub.findFirst("Broadcast", domain.MsgChatMessageOut)
	if !ok {
		t.Fatalf("no chat:message broadcast")
	}
	// Sender INCLUDED — excludeUserID must be "".
	if call.excludeUserID != "" {
		t.Errorf("chat:message excludeUserID = %q, want \"\" (sender included)", call.excludeUserID)
	}
}

// ----------------------------------------------------------------------------
// Test 7 — chat:message body >500 chars → CHAT_TOO_LONG to sender; NOT
// persisted, NOT broadcast.
// ----------------------------------------------------------------------------

func TestRouter_ChatMessage_OverCap_RejectsSenderOnly(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "room-7"
	fx.seedRoom(t, fx.defaultRoom(roomID))

	longBody := strings.Repeat("a", 501)
	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgChatMessage, map[string]interface{}{"body": longBody})

	// No persistence.
	msgs, _ := fx.repo.GetMessages(context.Background(), roomID, 10)
	if len(msgs) != 0 {
		t.Fatalf("len(msgs) = %d, want 0 (over-cap should not persist)", len(msgs))
	}

	// No broadcast.
	if _, ok := fx.hub.findFirst("Broadcast", domain.MsgChatMessageOut); ok {
		t.Fatal("chat:message should NOT have been broadcast")
	}

	// CHAT_TOO_LONG SendTo to sender.
	var found bool
	for _, c := range fx.hub.snapshot() {
		if c.method == "SendTo" && c.env.Type == domain.MsgError {
			var e domain.ErrorData
			_ = json.Unmarshal(c.env.Data, &e)
			if e.Code == domain.ErrCodeChatTooLong && c.userID == "alice" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("no CHAT_TOO_LONG SendTo; calls=%v", fx.hub.snapshot())
	}
}

// ----------------------------------------------------------------------------
// Test 8 — 6 chat messages within burst window → first 5 succeed, 6th
// gets RATE_LIMITED.
// ----------------------------------------------------------------------------

func TestRouter_ChatMessage_OverRate_RateLimited(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "room-8"
	fx.seedRoom(t, fx.defaultRoom(roomID))

	for i := 0; i < 6; i++ {
		fx.dispatchJSON(t, aliceConn(roomID), domain.MsgChatMessage, map[string]interface{}{"body": "x"})
	}

	// 5 broadcasts, 1 RATE_LIMITED.
	broadcasts := 0
	rateLimited := 0
	for _, c := range fx.hub.snapshot() {
		if c.method == "Broadcast" && c.env.Type == domain.MsgChatMessageOut {
			broadcasts++
		}
		if c.method == "SendTo" && c.env.Type == domain.MsgError {
			var e domain.ErrorData
			_ = json.Unmarshal(c.env.Data, &e)
			if e.Code == domain.ErrCodeRateLimited {
				rateLimited++
			}
		}
	}
	if broadcasts != 5 {
		t.Errorf("broadcasts = %d, want 5 (burst=5)", broadcasts)
	}
	if rateLimited != 1 {
		t.Errorf("RATE_LIMITED count = %d, want 1 (6th message)", rateLimited)
	}
}

// ----------------------------------------------------------------------------
// Test 9 — chat:reaction (whitelist emoji) → Broadcast, no persistence.
// ----------------------------------------------------------------------------

func TestRouter_ChatReaction_BroadcastsNoPersist(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "room-9"
	fx.seedRoom(t, fx.defaultRoom(roomID))

	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgChatReaction, map[string]interface{}{"emoji": "🔥"})

	// No persistence — message LIST stays empty.
	msgs, _ := fx.repo.GetMessages(context.Background(), roomID, 10)
	if len(msgs) != 0 {
		t.Fatalf("reactions must NOT persist; len(msgs) = %d", len(msgs))
	}

	call, ok := fx.hub.findFirst("Broadcast", domain.MsgChatReactionOut)
	if !ok {
		t.Fatalf("no chat:reaction broadcast; calls=%v", fx.hub.snapshot())
	}
	if call.excludeUserID != "" {
		t.Errorf("excludeUserID = %q, want \"\" (sender included for own-feedback)", call.excludeUserID)
	}
	var rxn domain.ChatReactionOutData
	_ = json.Unmarshal(call.env.Data, &rxn)
	if rxn.UserID != "alice" || rxn.Emoji != "🔥" {
		t.Errorf("reaction payload = %+v", rxn)
	}
}

// ----------------------------------------------------------------------------
// Test 9b — out-of-whitelist emoji silently dropped (no broadcast, no error).
// ----------------------------------------------------------------------------

func TestRouter_ChatReaction_NonWhitelist_Dropped(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "room-9b"
	fx.seedRoom(t, fx.defaultRoom(roomID))

	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgChatReaction, map[string]interface{}{"emoji": "💩"})

	if _, ok := fx.hub.findFirst("Broadcast", domain.MsgChatReactionOut); ok {
		t.Fatal("non-whitelist emoji should NOT broadcast")
	}
	if _, ok := fx.hub.findFirst("SendTo", domain.MsgError); ok {
		t.Fatal("non-whitelist emoji should be silently dropped, not error")
	}
}

// ----------------------------------------------------------------------------
// Test 10 — presence:heartbeat updates LastSeenAt; no broadcast.
// ----------------------------------------------------------------------------

func TestRouter_PresenceHeartbeat_UpdatesLastSeenNoBroadcast(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "room-10"
	fx.seedRoom(t, fx.defaultRoom(roomID))
	earlyJoinedAt := fx.now.Unix() - 600
	fx.seedMember(t, roomID, "alice", domain.MemberMeta{
		Username:   "Alice",
		AvatarURL:  "",
		JoinedAt:   earlyJoinedAt,
		LastSeenAt: earlyJoinedAt,
	})

	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgPresenceHeartbeat, struct{}{})

	// No broadcast / no SendTo expected.
	if len(fx.hub.snapshot()) != 0 {
		t.Fatalf("heartbeat must not generate hub calls; got=%v", fx.hub.snapshot())
	}

	// LastSeenAt updated; JoinedAt preserved.
	members, _ := fx.repo.ListMembers(context.Background(), roomID)
	var found bool
	for _, m := range members {
		if m.UserID == "alice" {
			found = true
			if m.Meta.LastSeenAt != fx.now.Unix() {
				t.Errorf("LastSeenAt = %d, want %d", m.Meta.LastSeenAt, fx.now.Unix())
			}
			if m.Meta.JoinedAt != earlyJoinedAt {
				t.Errorf("JoinedAt = %d, want preserved %d", m.Meta.JoinedAt, earlyJoinedAt)
			}
		}
	}
	if !found {
		t.Fatal("alice not found in members after heartbeat")
	}
}

// ----------------------------------------------------------------------------
// Test 11 — state:change_episode updates the HASH (episode_id, time=0,
// state=paused) and broadcasts room:state_changed to ALL.
// ----------------------------------------------------------------------------

func TestRouter_StateChangeEpisode_UpdatesAndBroadcastsAll(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "room-11"
	r := fx.defaultRoom(roomID)
	r.PlaybackState = domain.StatePlaying
	r.PlaybackTime = 123.0
	fx.seedRoom(t, r)

	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgStateChangeEpisode, map[string]interface{}{"episode_id": "5"})

	room, _ := fx.repo.GetRoom(context.Background(), roomID)
	if room.EpisodeID != "5" {
		t.Errorf("EpisodeID = %q, want 5", room.EpisodeID)
	}
	if room.PlaybackTime != 0 {
		t.Errorf("PlaybackTime = %v, want 0 (reset on episode change)", room.PlaybackTime)
	}
	if room.PlaybackState != domain.StatePaused {
		t.Errorf("PlaybackState = %q, want paused", room.PlaybackState)
	}

	call, ok := fx.hub.findFirst("Broadcast", domain.MsgRoomStateChanged)
	if !ok {
		t.Fatalf("no room:state_changed broadcast")
	}
	if call.excludeUserID != "" {
		t.Errorf("state_changed excludeUserID = %q, want \"\" (sender included)", call.excludeUserID)
	}
	var sc domain.RoomStateChangedData
	_ = json.Unmarshal(call.env.Data, &sc)
	if sc.Field != "episode_id" || sc.Value != "5" || sc.ByUserID != "alice" {
		t.Errorf("state_changed = %+v", sc)
	}

	// Verify catalog was invoked with the room's existing player + translation.
	cc := fx.catalog.snapshot()
	if len(cc) != 1 {
		t.Fatalf("catalog calls = %d, want 1", len(cc))
	}
	if cc[0].ShikimoriID != "anime-1" || cc[0].Player != domain.PlayerAnimeLib ||
		cc[0].EpisodeID != "5" || cc[0].TranslationID != "trans-1" {
		t.Errorf("catalog call = %+v, want {anime-1, animelib, 5, trans-1}", cc[0])
	}
}

// ----------------------------------------------------------------------------
// Phase 4 / Plan 04.3 — validated state-change handlers.
//
// Test matrix per CONTEXT §Inbound message handlers + 04.3-PLAN.md:
//   - change_episode: Valid=true / Valid=false / transport error / room TTL'd /
//     bad payload
//   - change_player:  Valid=true / Valid=false / bogus player value
//   - change_translation: Valid=true / Valid=false / transport error
// ----------------------------------------------------------------------------

// helper: did the test capture an error envelope with the given code?
func findErrorCode(t *testing.T, calls []fakeHubCall, code string) bool {
	t.Helper()
	for _, c := range calls {
		if c.method != "SendTo" || c.env.Type != domain.MsgError {
			continue
		}
		var e domain.ErrorData
		_ = json.Unmarshal(c.env.Data, &e)
		if e.Code == code {
			return true
		}
	}
	return false
}

// findBroadcast returns true iff any Broadcast call for the given msgType was
// recorded. Used to assert "NO broadcast happened" via negation.
func findBroadcast(calls []fakeHubCall, msgType string) bool {
	for _, c := range calls {
		if c.method == "Broadcast" && c.env.Type == msgType {
			return true
		}
	}
	return false
}

// ----------------------------------------------------------------------------
// Test StateChange.1 — change_episode Valid=true → broadcast + HSET, no error.
// (Covered partially by Test 11 above; this test focuses on the catalog tuple.)
// ----------------------------------------------------------------------------

func TestStateChange_Episode_Valid_BroadcastsAndUpdates(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "sc-ep-valid"
	fx.seedRoom(t, fx.defaultRoom(roomID))
	// Default fixture is alwaysValid.

	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgStateChangeEpisode,
		map[string]interface{}{"episode_id": "7"})

	calls := fx.hub.snapshot()
	if !findBroadcast(calls, domain.MsgRoomStateChanged) {
		t.Fatalf("expected room:state_changed broadcast; calls=%v", calls)
	}
	if findErrorCode(t, calls, domain.ErrCodeEpisodeUnavailable) {
		t.Fatal("unexpected EPISODE_UNAVAILABLE error sent on Valid=true path")
	}
	room, _ := fx.repo.GetRoom(context.Background(), roomID)
	if room.EpisodeID != "7" {
		t.Errorf("EpisodeID = %q, want 7", room.EpisodeID)
	}
}

// ----------------------------------------------------------------------------
// Test StateChange.2 — change_episode Valid=false → SendTo EPISODE_UNAVAILABLE
// sender-only, NO broadcast, NO HSET.
// ----------------------------------------------------------------------------

func TestStateChange_Episode_Invalid_SendsErrorNoMutation(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "sc-ep-invalid"
	fx.seedRoom(t, fx.defaultRoom(roomID))
	fx.catalog.alwaysInvalid(domain.ErrCodeEpisodeUnavailable)

	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgStateChangeEpisode,
		map[string]interface{}{"episode_id": "999"})

	calls := fx.hub.snapshot()
	if findBroadcast(calls, domain.MsgRoomStateChanged) {
		t.Fatal("expected NO broadcast on Valid=false")
	}
	if !findErrorCode(t, calls, domain.ErrCodeEpisodeUnavailable) {
		t.Fatalf("expected EPISODE_UNAVAILABLE error to sender; calls=%v", calls)
	}
	// Sender-only: the SendTo must be addressed to alice, not broadcast.
	var senderOnly bool
	for _, c := range calls {
		if c.method == "SendTo" && c.env.Type == domain.MsgError && c.userID == "alice" {
			senderOnly = true
		}
	}
	if !senderOnly {
		t.Fatal("EPISODE_UNAVAILABLE was not addressed to sender alice")
	}
	// Redis unchanged.
	room, _ := fx.repo.GetRoom(context.Background(), roomID)
	if room.EpisodeID != "ep-1" {
		t.Errorf("EpisodeID = %q, want unchanged ep-1", room.EpisodeID)
	}
}

// ----------------------------------------------------------------------------
// Test StateChange.3 — change_episode transport error → SendTo
// EPISODE_UNAVAILABLE, NO broadcast, NO HSET.
// ----------------------------------------------------------------------------

func TestStateChange_Episode_TransportError_SendsErrorNoMutation(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "sc-ep-transport"
	fx.seedRoom(t, fx.defaultRoom(roomID))
	fx.catalog.alwaysError(errors.New("catalog timeout"))

	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgStateChangeEpisode,
		map[string]interface{}{"episode_id": "5"})

	calls := fx.hub.snapshot()
	if findBroadcast(calls, domain.MsgRoomStateChanged) {
		t.Fatal("transport error must NOT broadcast (Redis-desync risk)")
	}
	if !findErrorCode(t, calls, domain.ErrCodeEpisodeUnavailable) {
		t.Fatalf("expected EPISODE_UNAVAILABLE on transport error; calls=%v", calls)
	}
	room, _ := fx.repo.GetRoom(context.Background(), roomID)
	if room.EpisodeID != "ep-1" {
		t.Errorf("EpisodeID = %q, want unchanged ep-1", room.EpisodeID)
	}
}

// ----------------------------------------------------------------------------
// Test StateChange.4 — change_episode against a TTL'd-out room (GetRoom
// returns ErrNotFound) → silent drop, no envelope, no broadcast.
// ----------------------------------------------------------------------------

func TestStateChange_Episode_RoomTTLOut_SilentDrop(t *testing.T) {
	fx := newRouterFixture(t)
	// Do NOT seed a room — GetRoom will return ErrNotFound.

	fx.dispatchJSON(t, aliceConn("ghost-room"), domain.MsgStateChangeEpisode,
		map[string]interface{}{"episode_id": "5"})

	calls := fx.hub.snapshot()
	if len(calls) != 0 {
		t.Fatalf("expected silent drop (no hub calls); got=%v", calls)
	}
	// Catalog should NOT have been called either (we never got to validation).
	if cc := fx.catalog.snapshot(); len(cc) != 0 {
		t.Fatalf("catalog should NOT be invoked when room is missing; got=%d calls", len(cc))
	}
}

// ----------------------------------------------------------------------------
// Test StateChange.5 — change_episode missing episode_id → BAD_PAYLOAD
// sender-only, no catalog call.
// ----------------------------------------------------------------------------

func TestStateChange_Episode_BadPayload_RejectsBeforeCatalog(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "sc-ep-bad"
	fx.seedRoom(t, fx.defaultRoom(roomID))

	// Empty payload — episode_id will decode to "".
	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgStateChangeEpisode,
		map[string]interface{}{})

	calls := fx.hub.snapshot()
	if !findErrorCode(t, calls, errCodeBadPayload) {
		t.Fatalf("expected BAD_PAYLOAD; calls=%v", calls)
	}
	if findBroadcast(calls, domain.MsgRoomStateChanged) {
		t.Fatal("bad payload must NOT broadcast")
	}
	if cc := fx.catalog.snapshot(); len(cc) != 0 {
		t.Fatalf("bad payload must short-circuit before catalog; got %d calls", len(cc))
	}
}

// ----------------------------------------------------------------------------
// Test StateChange.6 — change_player with invalid player value (not in the
// 5-member set) → BAD_PAYLOAD sender-only, no catalog call, no broadcast.
// ----------------------------------------------------------------------------

func TestStateChange_Player_BogusValue_BadPayload(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "sc-pl-bogus"
	fx.seedRoom(t, fx.defaultRoom(roomID))

	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgStateChangePlayer,
		map[string]interface{}{"player": "youtube"})

	calls := fx.hub.snapshot()
	if !findErrorCode(t, calls, errCodeBadPayload) {
		t.Fatalf("expected BAD_PAYLOAD for bogus player; calls=%v", calls)
	}
	if findBroadcast(calls, domain.MsgRoomStateChanged) {
		t.Fatal("bogus player must NOT broadcast")
	}
	if cc := fx.catalog.snapshot(); len(cc) != 0 {
		t.Fatalf("bogus player must short-circuit before catalog; got %d calls", len(cc))
	}
}

// ----------------------------------------------------------------------------
// Test StateChange.6b — aeplayer follows the permissive path: an aeplayer
// room with an unknown catalog episode id is NOT short-circuited — it
// round-trips to the catalog (whose permissive contract decides), exactly
// like ourenglish/hanime. Mechanism: aeplayer ∈ validPlayers so a
// change_player to it reaches catalog rather than BAD_PAYLOAD; and an
// aeplayer-room change_episode reaches catalog with the unknown id (no
// per-player short-circuit in this service).
// ----------------------------------------------------------------------------

func TestStateChange_AePlayer_PermissivePath(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "sc-ae-permissive"
	r := fx.defaultRoom(roomID)
	r.Player = domain.PlayerAePlayer // room is already on aeplayer
	fx.seedRoom(t, r)

	// Unknown catalog episode id: with the permissive contract (fake catalog
	// returns Valid=true) this must be accepted — NOT rejected pre-catalog.
	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgStateChangeEpisode,
		map[string]interface{}{"episode_id": "unknown-ep-999"})

	calls := fx.hub.snapshot()
	if findErrorCode(t, calls, errCodeBadPayload) {
		t.Fatal("aeplayer episode change must NOT be rejected as BAD_PAYLOAD (permissive path)")
	}
	if !findBroadcast(calls, domain.MsgRoomStateChanged) {
		t.Fatalf("expected room:state_changed broadcast on permissive accept; calls=%v", calls)
	}

	// The episode change must have round-tripped to the catalog with the
	// aeplayer + the unknown id (proves it took the permissive validate path).
	cc := fx.catalog.snapshot()
	if len(cc) != 1 {
		t.Fatalf("catalog calls = %d, want 1 (episode change must reach catalog)", len(cc))
	}
	if cc[0].Player != domain.PlayerAePlayer || cc[0].EpisodeID != "unknown-ep-999" {
		t.Errorf("catalog call = %+v, want {player=aeplayer, episode=unknown-ep-999}", cc[0])
	}

	room, _ := fx.repo.GetRoom(context.Background(), roomID)
	if room.EpisodeID != "unknown-ep-999" {
		t.Errorf("EpisodeID = %q, want unknown-ep-999 (permissive accept persisted)", room.EpisodeID)
	}
}

// ----------------------------------------------------------------------------
// Test StateChange.6c — changing TO aeplayer reaches the catalog round-trip
// (is in validPlayers), not short-circuited as BAD_PAYLOAD.
// ----------------------------------------------------------------------------

func TestStateChange_Player_AePlayer_ReachesCatalog(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "sc-pl-ae"
	fx.seedRoom(t, fx.defaultRoom(roomID))

	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgStateChangePlayer,
		map[string]interface{}{"player": domain.PlayerAePlayer})

	calls := fx.hub.snapshot()
	if findErrorCode(t, calls, errCodeBadPayload) {
		t.Fatal("aeplayer must be accepted by validPlayers, not BAD_PAYLOAD")
	}
	if !findBroadcast(calls, domain.MsgRoomStateChanged) {
		t.Fatalf("expected room:state_changed broadcast; calls=%v", calls)
	}
	cc := fx.catalog.snapshot()
	if len(cc) != 1 || cc[0].Player != domain.PlayerAePlayer {
		t.Fatalf("expected 1 catalog call with player=aeplayer; got %+v", cc)
	}
}

// ----------------------------------------------------------------------------
// Test StateChange.7 — change_player Valid=true → HSET player + reset
// episode_id="1" + translation_id="" + playback resets; broadcast to ALL.
// ----------------------------------------------------------------------------

func TestStateChange_Player_Valid_ResetsEpisodeAndTranslation(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "sc-pl-valid"
	r := fx.defaultRoom(roomID)
	r.PlaybackState = domain.StatePlaying
	r.PlaybackTime = 50.0
	fx.seedRoom(t, r)

	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgStateChangePlayer,
		map[string]interface{}{"player": domain.PlayerKodik})

	calls := fx.hub.snapshot()
	if !findBroadcast(calls, domain.MsgRoomStateChanged) {
		t.Fatalf("expected room:state_changed broadcast; calls=%v", calls)
	}

	// Find the broadcast envelope and inspect Field/Value.
	call, _ := fx.hub.findFirst("Broadcast", domain.MsgRoomStateChanged)
	var sc domain.RoomStateChangedData
	_ = json.Unmarshal(call.env.Data, &sc)
	if sc.Field != "player" || sc.Value != domain.PlayerKodik {
		t.Errorf("state_changed = %+v, want field=player value=kodik", sc)
	}
	if call.excludeUserID != "" {
		t.Errorf("player change excludeUserID = %q, want \"\" (sender included)", call.excludeUserID)
	}

	// Redis state reset.
	room, _ := fx.repo.GetRoom(context.Background(), roomID)
	if room.Player != domain.PlayerKodik {
		t.Errorf("Player = %q, want kodik", room.Player)
	}
	if room.EpisodeID != "1" {
		t.Errorf("EpisodeID = %q, want \"1\" (reset to first episode)", room.EpisodeID)
	}
	if room.TranslationID != "" {
		t.Errorf("TranslationID = %q, want empty (reset on player change)", room.TranslationID)
	}
	if room.PlaybackTime != 0 || room.PlaybackState != domain.StatePaused {
		t.Errorf("playback not reset: time=%v state=%q", room.PlaybackTime, room.PlaybackState)
	}

	// Catalog called with new player + empty episode/translation (per
	// player-change validation mode).
	cc := fx.catalog.snapshot()
	if len(cc) != 1 {
		t.Fatalf("catalog calls = %d, want 1", len(cc))
	}
	if cc[0].Player != domain.PlayerKodik || cc[0].EpisodeID != "" || cc[0].TranslationID != "" {
		t.Errorf("catalog call = %+v, want {player=kodik, empty ep/trans}", cc[0])
	}
}

// ----------------------------------------------------------------------------
// Test StateChange.8 — change_player Valid=false{PLAYER_UNAVAILABLE} →
// SendTo PLAYER_UNAVAILABLE sender-only, NO broadcast, NO HSET.
// ----------------------------------------------------------------------------

func TestStateChange_Player_Invalid_SendsErrorNoMutation(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "sc-pl-invalid"
	fx.seedRoom(t, fx.defaultRoom(roomID))
	fx.catalog.alwaysInvalid(domain.ErrCodePlayerUnavailable)

	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgStateChangePlayer,
		map[string]interface{}{"player": domain.PlayerHanime})

	calls := fx.hub.snapshot()
	if findBroadcast(calls, domain.MsgRoomStateChanged) {
		t.Fatal("Valid=false must NOT broadcast")
	}
	if !findErrorCode(t, calls, domain.ErrCodePlayerUnavailable) {
		t.Fatalf("expected PLAYER_UNAVAILABLE sender-only; calls=%v", calls)
	}
	room, _ := fx.repo.GetRoom(context.Background(), roomID)
	if room.Player != domain.PlayerAnimeLib {
		t.Errorf("Player = %q, want unchanged animelib", room.Player)
	}
}

// ----------------------------------------------------------------------------
// Test StateChange.9 — change_translation Valid=true → HSET translation_id +
// playback resets; broadcast to ALL.
// ----------------------------------------------------------------------------

func TestStateChange_Translation_Valid_UpdatesAndBroadcasts(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "sc-tr-valid"
	r := fx.defaultRoom(roomID)
	r.PlaybackState = domain.StatePlaying
	r.PlaybackTime = 200.0
	fx.seedRoom(t, r)

	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgStateChangeTrans,
		map[string]interface{}{"translation_id": "trans-2"})

	calls := fx.hub.snapshot()
	if !findBroadcast(calls, domain.MsgRoomStateChanged) {
		t.Fatalf("expected room:state_changed broadcast; calls=%v", calls)
	}
	call, _ := fx.hub.findFirst("Broadcast", domain.MsgRoomStateChanged)
	var sc domain.RoomStateChangedData
	_ = json.Unmarshal(call.env.Data, &sc)
	if sc.Field != "translation_id" || sc.Value != "trans-2" || sc.ByUserID != "alice" {
		t.Errorf("state_changed = %+v, want {translation_id, trans-2, alice}", sc)
	}

	room, _ := fx.repo.GetRoom(context.Background(), roomID)
	if room.TranslationID != "trans-2" {
		t.Errorf("TranslationID = %q, want trans-2", room.TranslationID)
	}
	if room.PlaybackTime != 0 || room.PlaybackState != domain.StatePaused {
		t.Errorf("playback not reset: time=%v state=%q", room.PlaybackTime, room.PlaybackState)
	}

	// Catalog called with current player + episode + new translation.
	cc := fx.catalog.snapshot()
	if len(cc) != 1 {
		t.Fatalf("catalog calls = %d, want 1", len(cc))
	}
	if cc[0].EpisodeID != "ep-1" || cc[0].TranslationID != "trans-2" || cc[0].Player != domain.PlayerAnimeLib {
		t.Errorf("catalog call = %+v, want {animelib, ep-1, trans-2}", cc[0])
	}
}

// ----------------------------------------------------------------------------
// Test StateChange.10 — change_translation Valid=false{TRANSLATION_UNAVAILABLE}
// → SendTo sender-only, NO broadcast, NO HSET.
// ----------------------------------------------------------------------------

func TestStateChange_Translation_Invalid_SendsErrorNoMutation(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "sc-tr-invalid"
	fx.seedRoom(t, fx.defaultRoom(roomID))
	fx.catalog.alwaysInvalid(domain.ErrCodeTranslationUnavailable)

	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgStateChangeTrans,
		map[string]interface{}{"translation_id": "missing-trans"})

	calls := fx.hub.snapshot()
	if findBroadcast(calls, domain.MsgRoomStateChanged) {
		t.Fatal("Valid=false must NOT broadcast")
	}
	if !findErrorCode(t, calls, domain.ErrCodeTranslationUnavailable) {
		t.Fatalf("expected TRANSLATION_UNAVAILABLE sender-only; calls=%v", calls)
	}
	room, _ := fx.repo.GetRoom(context.Background(), roomID)
	if room.TranslationID != "trans-1" {
		t.Errorf("TranslationID = %q, want unchanged trans-1", room.TranslationID)
	}
}

// ----------------------------------------------------------------------------
// Test StateChange.11 — change_translation transport error → SendTo
// TRANSLATION_UNAVAILABLE, NO broadcast, NO HSET.
// ----------------------------------------------------------------------------

func TestStateChange_Translation_TransportError_SendsErrorNoMutation(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "sc-tr-transport"
	fx.seedRoom(t, fx.defaultRoom(roomID))
	fx.catalog.alwaysError(errors.New("catalog 500"))

	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgStateChangeTrans,
		map[string]interface{}{"translation_id": "trans-2"})

	calls := fx.hub.snapshot()
	if findBroadcast(calls, domain.MsgRoomStateChanged) {
		t.Fatal("transport error must NOT broadcast")
	}
	if !findErrorCode(t, calls, domain.ErrCodeTranslationUnavailable) {
		t.Fatalf("expected TRANSLATION_UNAVAILABLE on transport error; calls=%v", calls)
	}
	room, _ := fx.repo.GetRoom(context.Background(), roomID)
	if room.TranslationID != "trans-1" {
		t.Errorf("TranslationID = %q, want unchanged trans-1", room.TranslationID)
	}
}

// ----------------------------------------------------------------------------
// Test StateChange.12 — Verify all Valid=true broadcasts carry the right
// envelope shape (Field/Value/ByUserID populated AND excludeUserID="" meaning
// sender included).
// ----------------------------------------------------------------------------

func TestStateChange_AllValid_BroadcastEnvelopeShape(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "sc-shape"
	fx.seedRoom(t, fx.defaultRoom(roomID))

	tests := []struct {
		name      string
		msgType   string
		payload   map[string]interface{}
		wantField string
		wantValue string
	}{
		{"episode", domain.MsgStateChangeEpisode,
			map[string]interface{}{"episode_id": "3"}, "episode_id", "3"},
		{"player", domain.MsgStateChangePlayer,
			map[string]interface{}{"player": domain.PlayerOurEnglish}, "player", domain.PlayerOurEnglish},
		{"translation", domain.MsgStateChangeTrans,
			map[string]interface{}{"translation_id": "t-9"}, "translation_id", "t-9"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Fresh hub per sub-case so findFirst returns the right call.
			fx.hub = newFakeHub()
			fx.router.hub = fx.hub
			// Re-seed the room each sub-case since player-change mutates it.
			fx.mr.FlushAll()
			fx.seedRoom(t, fx.defaultRoom(roomID))

			fx.dispatchJSON(t, aliceConn(roomID), tc.msgType, tc.payload)

			call, ok := fx.hub.findFirst("Broadcast", domain.MsgRoomStateChanged)
			if !ok {
				t.Fatalf("[%s] no room:state_changed broadcast", tc.name)
			}
			if call.excludeUserID != "" {
				t.Errorf("[%s] excludeUserID = %q, want \"\"", tc.name, call.excludeUserID)
			}
			var sc domain.RoomStateChangedData
			_ = json.Unmarshal(call.env.Data, &sc)
			if sc.Field != tc.wantField || sc.Value != tc.wantValue || sc.ByUserID != "alice" {
				t.Errorf("[%s] envelope = %+v, want field=%s value=%s by=alice",
					tc.name, sc, tc.wantField, tc.wantValue)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Test 12 — unknown message type → UNKNOWN_TYPE error to sender, connection
// not crashed (the router just returns).
// ----------------------------------------------------------------------------

func TestRouter_UnknownType_SendsUnknownError(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "room-12"
	fx.seedRoom(t, fx.defaultRoom(roomID))

	fx.router.Dispatch(aliceConn(roomID), domain.Envelope{Type: "bogus:not_a_real_type", Data: json.RawMessage("{}")})

	var found bool
	for _, c := range fx.hub.snapshot() {
		if c.method == "SendTo" && c.env.Type == domain.MsgError {
			var e domain.ErrorData
			_ = json.Unmarshal(c.env.Data, &e)
			if e.Code == errCodeUnknownType {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("no UNKNOWN_TYPE SendTo; calls=%v", fx.hub.snapshot())
	}
}

// ----------------------------------------------------------------------------
// Test 13 — malformed payload (invalid JSON for the expected shape) →
// BAD_PAYLOAD to sender, no state change.
// ----------------------------------------------------------------------------

func TestRouter_BadPayload_SendsBadPayloadError(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "room-13"
	fx.seedRoom(t, fx.defaultRoom(roomID))

	// "time" is a string instead of a number → decode fails.
	fx.router.Dispatch(aliceConn(roomID), domain.Envelope{
		Type: domain.MsgPlaybackPlay,
		Data: json.RawMessage(`{"time": "not a number"}`),
	})

	room, _ := fx.repo.GetRoom(context.Background(), roomID)
	if room.PlaybackState == domain.StatePlaying {
		t.Fatal("PlaybackState should NOT have advanced on bad payload")
	}

	var found bool
	for _, c := range fx.hub.snapshot() {
		if c.method == "SendTo" && c.env.Type == domain.MsgError {
			var e domain.ErrorData
			_ = json.Unmarshal(c.env.Data, &e)
			if e.Code == errCodeBadPayload {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("no BAD_PAYLOAD SendTo; calls=%v", fx.hub.snapshot())
	}
}

// ----------------------------------------------------------------------------
// Test 14 — OnDisconnect resets the drift state + rate-limit buckets.
// After persistent drift, OnDisconnect lets a "reconnected" sender start fresh.
// ----------------------------------------------------------------------------

func TestRouter_OnDisconnect_ClearsDriftAndRateLimit(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "room-14"
	r := fx.defaultRoom(roomID)
	r.PlaybackState = domain.StatePlaying
	r.PlaybackTime = 100.0
	r.PlaybackTimeUpdatedAtMs = fx.now.UnixMilli()
	fx.seedRoom(t, r)

	// Push alice into persistent drift via 5 hard ticks.
	for i := 0; i < 5; i++ {
		fx.dispatchJSON(t, aliceConn(roomID), domain.MsgPlaybackTimeTick, map[string]interface{}{"time": 106.0})
	}
	// Consume alice's seek burst.
	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgPlaybackSeek, map[string]interface{}{"time": 5.0})

	// Verify pre-disconnect: another seek is rate-limited.
	prevCallCount := len(fx.hub.snapshot())
	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgPlaybackSeek, map[string]interface{}{"time": 6.0})
	// Expect a RATE_LIMITED error.
	if newCount := len(fx.hub.snapshot()); newCount <= prevCallCount {
		t.Fatal("expected second seek to generate hub call (RATE_LIMITED)")
	}

	// Disconnect alice.
	fx.router.OnDisconnect(roomID, "alice")

	// Post-disconnect: seek succeeds (fresh bucket).
	preSeekCalls := len(fx.hub.snapshot())
	fx.dispatchJSON(t, aliceConn(roomID), domain.MsgPlaybackSeek, map[string]interface{}{"time": 99.0})
	// Find the most recent Broadcast(playback:event, kind=seek).
	var seekBroadcast bool
	for _, c := range fx.hub.snapshot()[preSeekCalls:] {
		if c.method == "Broadcast" && c.env.Type == domain.MsgPlaybackEvent {
			var evt domain.PlaybackEventData
			_ = json.Unmarshal(c.env.Data, &evt)
			if evt.Kind == "seek" {
				seekBroadcast = true
			}
		}
	}
	if !seekBroadcast {
		t.Fatal("expected fresh seek broadcast after OnDisconnect")
	}
}

// ----------------------------------------------------------------------------
// Plan 05.2 — PersistentDriftTotal label tests (WT-NF-06).
//
// The drift engine declares "persistent drift" after 5 consecutive hard
// ticks; the router labels the wt_persistent_drift_total counter by
// whether the affected user is the room's host or a regular member. We
// drive 5 hard ticks for each case and assert the labeled counter advances.
// ----------------------------------------------------------------------------

// pushPersistentDrift drives the supplied conn into persistent-drift state
// by dispatching 5 hard time_tick envelopes in a row. The fixture's room
// MUST be set to StatePlaying with PlaybackTime + PlaybackTimeUpdatedAtMs
// such that drift = |106 - 100| = 6s (hard band).
func (fx *routerFixture) pushPersistentDrift(t *testing.T, conn ConnectionCtx) {
	t.Helper()
	for i := 0; i < 5; i++ {
		fx.dispatchJSON(t, conn, domain.MsgPlaybackTimeTick, map[string]interface{}{"time": 106.0})
	}
}

func TestRouter_TimeTick_PersistentDrift_LabelsHost(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "room-pdrift-host"
	r := fx.defaultRoom(roomID)
	r.PlaybackState = domain.StatePlaying
	r.PlaybackTime = 100.0
	r.PlaybackTimeUpdatedAtMs = fx.now.UnixMilli()
	r.HostUserID = "alice" // alice IS the host
	fx.seedRoom(t, r)

	beforeHost := testutil.ToFloat64(PersistentDriftTotal.WithLabelValues("host"))
	fx.pushPersistentDrift(t, aliceConn(roomID))

	if got := testutil.ToFloat64(PersistentDriftTotal.WithLabelValues("host")); got != beforeHost+1 {
		t.Errorf("PersistentDriftTotal{user_role=host} = %v, want %v", got, beforeHost+1)
	}
}

func TestRouter_TimeTick_PersistentDrift_LabelsMember(t *testing.T) {
	fx := newRouterFixture(t)
	roomID := "room-pdrift-member"
	r := fx.defaultRoom(roomID)
	r.PlaybackState = domain.StatePlaying
	r.PlaybackTime = 100.0
	r.PlaybackTimeUpdatedAtMs = fx.now.UnixMilli()
	r.HostUserID = "bob" // host is bob, alice is just a member
	fx.seedRoom(t, r)

	beforeMember := testutil.ToFloat64(PersistentDriftTotal.WithLabelValues("member"))
	fx.pushPersistentDrift(t, aliceConn(roomID))

	if got := testutil.ToFloat64(PersistentDriftTotal.WithLabelValues("member")); got != beforeMember+1 {
		t.Errorf("PersistentDriftTotal{user_role=member} = %v, want %v", got, beforeMember+1)
	}
}
