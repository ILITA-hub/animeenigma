package repo

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/domain"
)

const testTTL = 900 * time.Second

// newRepo spins up a fresh miniredis + RoomRepo for each test.
func newRepo(t *testing.T) (*RoomRepo, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	log := logger.Default()
	return NewRoomRepo(client, testTTL, log), mr
}

func sampleRoom(id string) *domain.Room {
	return &domain.Room{
		ID:                      id,
		CreatedAt:               1700000000,
		AnimeID:                 "anime-uuid-1",
		EpisodeID:               "ep-1",
		Player:                  "animelib",
		TranslationID:           "translation-1",
		PlaybackState:           "paused",
		PlaybackTime:            42.5,
		PlaybackTimeUpdatedAtMs: 1700000000000,
		HostUserID:              "user-host",
	}
}

func TestCreateAndGetRoom_RoundTrips(t *testing.T) {
	r, _ := newRepo(t)
	ctx := context.Background()

	want := sampleRoom("room-1")
	if err := r.CreateRoom(ctx, want); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	got, err := r.GetRoom(ctx, "room-1")
	if err != nil {
		t.Fatalf("GetRoom: %v", err)
	}

	if got.ID != want.ID ||
		got.CreatedAt != want.CreatedAt ||
		got.AnimeID != want.AnimeID ||
		got.EpisodeID != want.EpisodeID ||
		got.Player != want.Player ||
		got.TranslationID != want.TranslationID ||
		got.PlaybackState != want.PlaybackState ||
		got.PlaybackTime != want.PlaybackTime ||
		got.PlaybackTimeUpdatedAtMs != want.PlaybackTimeUpdatedAtMs ||
		got.HostUserID != want.HostUserID {
		t.Errorf("round-trip mismatch:\n got = %+v\nwant = %+v", got, want)
	}
}

func TestCreateRoom_SetsTTL(t *testing.T) {
	r, mr := newRepo(t)
	ctx := context.Background()
	room := sampleRoom("ttl-room")

	if err := r.CreateRoom(ctx, room); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	// All 3 persistent keys should carry the configured TTL.
	for _, key := range []string{
		KeyRoom("ttl-room"),
		KeyRoomMembers("ttl-room"),
		KeyRoomMessages("ttl-room"),
	} {
		ttl := mr.TTL(key)
		// Members/messages keys may not yet exist if Redis doesn't allow EXPIRE
		// on missing keys; we tolerate zero for those, but the room HASH MUST
		// have a TTL because it was just HSET.
		if key == KeyRoom("ttl-room") {
			if ttl != testTTL {
				t.Errorf("TTL on %s = %v; want %v", key, ttl, testTTL)
			}
		}
	}
}

func TestGetRoom_NotFound(t *testing.T) {
	r, _ := newRepo(t)
	ctx := context.Background()

	got, err := r.GetRoom(ctx, "does-not-exist")
	if got != nil {
		t.Errorf("GetRoom returned non-nil room: %+v", got)
	}
	if err == nil {
		t.Fatal("GetRoom on missing room: want error, got nil")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetRoom error = %v; want errors.Is(err, ErrNotFound)", err)
	}
}

func TestUpdateRoomState_PartialUpdateRefreshesTTL(t *testing.T) {
	r, mr := newRepo(t)
	ctx := context.Background()
	room := sampleRoom("update-room")

	if err := r.CreateRoom(ctx, room); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	// Advance miniredis clock and verify TTL was reset (well below TTL bound).
	mr.FastForward(100 * time.Second)
	ttlBefore := mr.TTL(KeyRoom("update-room"))
	if ttlBefore >= testTTL {
		t.Fatalf("precondition: TTL = %v after fast-forward; want < %v", ttlBefore, testTTL)
	}

	fields := map[string]interface{}{
		"playback_state":              "playing",
		"playback_time":               99.25,
		"playback_time_updated_at":    int64(1700000900000),
	}
	if err := r.UpdateRoomState(ctx, "update-room", fields); err != nil {
		t.Fatalf("UpdateRoomState: %v", err)
	}

	// Read back — only those 3 fields should have changed.
	got, err := r.GetRoom(ctx, "update-room")
	if err != nil {
		t.Fatalf("GetRoom: %v", err)
	}
	if got.PlaybackState != "playing" {
		t.Errorf("PlaybackState = %q; want playing", got.PlaybackState)
	}
	if got.PlaybackTime != 99.25 {
		t.Errorf("PlaybackTime = %v; want 99.25", got.PlaybackTime)
	}
	if got.PlaybackTimeUpdatedAtMs != 1700000900000 {
		t.Errorf("PlaybackTimeUpdatedAtMs = %d; want 1700000900000", got.PlaybackTimeUpdatedAtMs)
	}
	// AnimeID / EpisodeID / Player must remain unchanged.
	if got.AnimeID != room.AnimeID || got.EpisodeID != room.EpisodeID || got.Player != room.Player {
		t.Errorf("partial update bled into other fields: %+v", got)
	}

	// TTL must have been refreshed.
	ttlAfter := mr.TTL(KeyRoom("update-room"))
	if ttlAfter != testTTL {
		t.Errorf("TTL after UpdateRoomState = %v; want %v (sliding TTL not refreshed)", ttlAfter, testTTL)
	}
}

func TestUpdateRoomState_RejectsUnknownField(t *testing.T) {
	r, _ := newRepo(t)
	ctx := context.Background()
	room := sampleRoom("invalid-update")
	if err := r.CreateRoom(ctx, room); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	err := r.UpdateRoomState(ctx, "invalid-update", map[string]interface{}{
		"injected_field": "bad",
	})
	if err == nil {
		t.Fatal("UpdateRoomState with unknown field: want error, got nil")
	}
}

func TestAddListRemoveMember(t *testing.T) {
	r, mr := newRepo(t)
	ctx := context.Background()
	room := sampleRoom("members-room")
	if err := r.CreateRoom(ctx, room); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	meta1 := domain.MemberMeta{Username: "alice", AvatarURL: "/a.png", JoinedAt: 1700000100, LastSeenAt: 1700000200}
	meta2 := domain.MemberMeta{Username: "bob", AvatarURL: "/b.png", JoinedAt: 1700000110, LastSeenAt: 1700000210}

	if err := r.AddMember(ctx, "members-room", "user-1", meta1); err != nil {
		t.Fatalf("AddMember alice: %v", err)
	}
	if err := r.AddMember(ctx, "members-room", "user-2", meta2); err != nil {
		t.Fatalf("AddMember bob: %v", err)
	}

	members, err := r.ListMembers(ctx, "members-room")
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 2 {
		t.Errorf("ListMembers len = %d; want 2", len(members))
	}

	byID := map[string]domain.MemberMeta{}
	for _, m := range members {
		byID[m.UserID] = m.Meta
	}
	if byID["user-1"] != meta1 {
		t.Errorf("user-1 meta = %+v; want %+v", byID["user-1"], meta1)
	}
	if byID["user-2"] != meta2 {
		t.Errorf("user-2 meta = %+v; want %+v", byID["user-2"], meta2)
	}

	count, err := r.CountMembers(ctx, "members-room")
	if err != nil {
		t.Fatalf("CountMembers: %v", err)
	}
	if count != 2 {
		t.Errorf("CountMembers = %d; want 2", count)
	}

	if err := r.RemoveMember(ctx, "members-room", "user-1"); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}

	count, err = r.CountMembers(ctx, "members-room")
	if err != nil {
		t.Fatalf("CountMembers after remove: %v", err)
	}
	if count != 1 {
		t.Errorf("CountMembers after remove = %d; want 1", count)
	}

	// TTL on members key should be set after AddMember.
	if ttl := mr.TTL(KeyRoomMembers("members-room")); ttl != testTTL {
		t.Errorf("TTL(members) = %v; want %v", ttl, testTTL)
	}
}

func TestAppendMessage_CapsAt100(t *testing.T) {
	r, mr := newRepo(t)
	ctx := context.Background()
	room := sampleRoom("chat-room")
	if err := r.CreateRoom(ctx, room); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	// Push 150 messages — the LIST must be capped at 100 by LTRIM 0 99.
	for i := 0; i < 150; i++ {
		msg := domain.ChatMessage{
			ID:       fmt.Sprintf("msg-%03d", i),
			UserID:   "user-1",
			Username: "alice",
			Body:     fmt.Sprintf("body %d", i),
			TS:       int64(1700000000000 + i),
		}
		if err := r.AppendMessage(ctx, "chat-room", msg); err != nil {
			t.Fatalf("AppendMessage[%d]: %v", i, err)
		}
	}

	// Pull all 100 — they must be the most-recent 100, oldest-first.
	got, err := r.GetMessages(ctx, "chat-room", 100)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(got) != 100 {
		t.Fatalf("GetMessages len = %d; want 100", len(got))
	}

	// First (oldest) should be msg-050; last (newest) should be msg-149.
	if got[0].ID != "msg-050" {
		t.Errorf("oldest message ID = %q; want msg-050", got[0].ID)
	}
	if got[99].ID != "msg-149" {
		t.Errorf("newest message ID = %q; want msg-149", got[99].ID)
	}

	// Sanity-check chronological monotonicity.
	for i := 1; i < len(got); i++ {
		if got[i].TS < got[i-1].TS {
			t.Errorf("messages not oldest-first at index %d (TS=%d < prev=%d)", i, got[i].TS, got[i-1].TS)
			break
		}
	}

	// TTL on messages key should be the configured value.
	if ttl := mr.TTL(KeyRoomMessages("chat-room")); ttl != testTTL {
		t.Errorf("TTL(messages) = %v; want %v", ttl, testTTL)
	}
}

func TestGetMessages_LimitFifty(t *testing.T) {
	r, _ := newRepo(t)
	ctx := context.Background()
	room := sampleRoom("chat-50")
	if err := r.CreateRoom(ctx, room); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	for i := 0; i < 80; i++ {
		msg := domain.ChatMessage{
			ID: fmt.Sprintf("m-%03d", i),
			TS: int64(1700000000000 + i),
		}
		if err := r.AppendMessage(ctx, "chat-50", msg); err != nil {
			t.Fatalf("AppendMessage: %v", err)
		}
	}

	got, err := r.GetMessages(ctx, "chat-50", 50)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(got) != 50 {
		t.Fatalf("GetMessages len = %d; want 50", len(got))
	}
	// 50 newest = m-030..m-079, oldest-first.
	if got[0].ID != "m-030" {
		t.Errorf("oldest of 50 = %q; want m-030", got[0].ID)
	}
	if got[49].ID != "m-079" {
		t.Errorf("newest of 50 = %q; want m-079", got[49].ID)
	}
}

func TestDeleteRoom_RemovesAllKeys(t *testing.T) {
	r, mr := newRepo(t)
	ctx := context.Background()
	room := sampleRoom("delete-room")
	if err := r.CreateRoom(ctx, room); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := r.AddMember(ctx, "delete-room", "u1", domain.MemberMeta{Username: "alice"}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if err := r.AppendMessage(ctx, "delete-room", domain.ChatMessage{ID: "m1"}); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}

	// Sanity-check the keys exist.
	for _, k := range []string{
		KeyRoom("delete-room"),
		KeyRoomMembers("delete-room"),
		KeyRoomMessages("delete-room"),
	} {
		if !mr.Exists(k) {
			t.Fatalf("precondition: key %s should exist", k)
		}
	}

	if err := r.DeleteRoom(ctx, "delete-room"); err != nil {
		t.Fatalf("DeleteRoom: %v", err)
	}

	for _, k := range []string{
		KeyRoom("delete-room"),
		KeyRoomMembers("delete-room"),
		KeyRoomMessages("delete-room"),
	} {
		if mr.Exists(k) {
			t.Errorf("DeleteRoom: key %s should be gone", k)
		}
	}
}

func TestRefreshTTL_BumpsAllThreeKeys(t *testing.T) {
	r, mr := newRepo(t)
	ctx := context.Background()
	room := sampleRoom("refresh-room")
	if err := r.CreateRoom(ctx, room); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := r.AddMember(ctx, "refresh-room", "u1", domain.MemberMeta{Username: "alice"}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if err := r.AppendMessage(ctx, "refresh-room", domain.ChatMessage{ID: "m1"}); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}

	mr.FastForward(300 * time.Second)
	// Each key should now have TTL < testTTL.
	for _, k := range []string{
		KeyRoom("refresh-room"),
		KeyRoomMembers("refresh-room"),
		KeyRoomMessages("refresh-room"),
	} {
		if ttl := mr.TTL(k); ttl == 0 || ttl >= testTTL {
			t.Fatalf("precondition: TTL(%s)=%v; want 0<ttl<%v", k, ttl, testTTL)
		}
	}

	if err := r.RefreshTTL(ctx, "refresh-room"); err != nil {
		t.Fatalf("RefreshTTL: %v", err)
	}

	for _, k := range []string{
		KeyRoom("refresh-room"),
		KeyRoomMembers("refresh-room"),
		KeyRoomMessages("refresh-room"),
	} {
		if ttl := mr.TTL(k); ttl != testTTL {
			t.Errorf("TTL(%s) = %v; want %v", k, ttl, testTTL)
		}
	}
}

func TestExists(t *testing.T) {
	r, _ := newRepo(t)
	ctx := context.Background()

	ok, err := r.Exists(ctx, "missing")
	if err != nil {
		t.Fatalf("Exists missing: %v", err)
	}
	if ok {
		t.Errorf("Exists(missing) = true; want false")
	}

	if err := r.CreateRoom(ctx, sampleRoom("present")); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	ok, err = r.Exists(ctx, "present")
	if err != nil {
		t.Fatalf("Exists present: %v", err)
	}
	if !ok {
		t.Errorf("Exists(present) = false; want true")
	}
}

// TestMessageCount_ReturnsLLEN verifies the Plan 05.2 helper used by the
// rooms.go / grace.go teardown observation path. Empty room returns 0
// (not an error). After AppendMessage runs N times, MessageCount == N (up
// to the LTRIM cap of 100).
func TestMessageCount_ReturnsLLEN(t *testing.T) {
	r, _ := newRepo(t)
	ctx := context.Background()

	// Empty room → 0, not an error.
	n, err := r.MessageCount(ctx, "msgcount-room")
	if err != nil {
		t.Fatalf("MessageCount empty: %v", err)
	}
	if n != 0 {
		t.Errorf("empty MessageCount = %d, want 0", n)
	}

	// Create the room + append 3 messages.
	if err := r.CreateRoom(ctx, sampleRoom("msgcount-room")); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for i := 0; i < 3; i++ {
		msg := domain.ChatMessage{
			ID:   fmt.Sprintf("msg-%02d", i),
			Body: fmt.Sprintf("body %d", i),
			TS:   int64(1700000000000 + i),
		}
		if err := r.AppendMessage(ctx, "msgcount-room", msg); err != nil {
			t.Fatalf("AppendMessage[%d]: %v", i, err)
		}
	}

	n, err = r.MessageCount(ctx, "msgcount-room")
	if err != nil {
		t.Fatalf("MessageCount: %v", err)
	}
	if n != 3 {
		t.Errorf("MessageCount = %d, want 3", n)
	}
}

func TestPublishSubscribe_RoundTrip(t *testing.T) {
	// miniredis supports PubSub since v2.30 — verify the roundtrip end-to-end.
	r, _ := newRepo(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sub := r.Subscribe(ctx, "pubsub-room")
	defer func() { _ = sub.Close() }()

	// Wait for the subscription to register (Receive returns *Subscription first).
	if _, err := sub.Receive(ctx); err != nil {
		t.Fatalf("subscribe handshake: %v", err)
	}

	ch := sub.Channel()

	payload := []byte(`{"type":"playback:play"}`)
	go func() {
		// Tiny delay so the subscriber Channel goroutine is fully wired before
		// the publish. miniredis is synchronous so this is belt-and-suspenders.
		time.Sleep(50 * time.Millisecond)
		if err := r.Publish(ctx, "pubsub-room", payload); err != nil {
			t.Errorf("Publish: %v", err)
		}
	}()

	select {
	case msg := <-ch:
		if msg.Payload != string(payload) {
			t.Errorf("pubsub payload = %q; want %q", msg.Payload, payload)
		}
		if msg.Channel != KeyRoomEvents("pubsub-room") {
			t.Errorf("pubsub channel = %q; want %q", msg.Channel, KeyRoomEvents("pubsub-room"))
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for pubsub message")
	}
}
