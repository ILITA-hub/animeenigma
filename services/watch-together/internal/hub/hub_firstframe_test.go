package hub

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/domain"
)

// registerFakeWithFirstFrame attaches a fakeConn via the package-private
// snapshot-aware registration seam (registerInternalWithFirstFrame). Mirrors
// registerFake but primes the first frame the way the WS handler does in
// production.
func registerFakeWithFirstFrame(t *testing.T, h *Hub, roomID, userID, username string, firstFrame []byte) (*Connection, *fakeConn) {
	t.Helper()
	fc := newFakeConn()
	c := h.registerInternalWithFirstFrame(roomID, userID, username, fc, firstFrame)
	return c, fc
}

// ----------------------------------------------------------------------------
// L809 — room:snapshot must be the FIRST frame the client sees, even when a
// concurrent Broadcast (another member's chat, a near-simultaneous
// member:joined) lands during the join. The snapshot is primed onto the
// connection's sendCh BEFORE the connection becomes broadcast-eligible, so the
// FIFO writePump always drains it first.
//
// RED before the fix: registerInternalWithFirstFrame does not exist. After the
// fix: the snapshot is enqueued under the same critical section that makes the
// conn eligible, so a Broadcast fired immediately after registration returns is
// always ordered AFTER the snapshot in the FIFO.
// ----------------------------------------------------------------------------

func TestRegisterWithFirstFrame_SnapshotDrainsFirst(t *testing.T) {
	h, _, _ := newTestHub(t)

	snapshotEnv := sampleEnvelope(t, domain.MsgRoomSnapshot, map[string]any{"snapshot": true})
	snapshotPayload, err := json.Marshal(snapshotEnv)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}

	// Register with the snapshot primed as the first frame. The moment this
	// returns the connection is broadcast-eligible.
	_, fc := registerFakeWithFirstFrame(t, h, "room-A", "alice", "Alice", snapshotPayload)

	// Immediately fire a Broadcast into the same room — this is the race
	// window the finding describes. With the snapshot already on sendCh, the
	// chat frame is enqueued AFTER it.
	chatEnv := sampleEnvelope(t, domain.MsgChatMessageOut, map[string]any{"body": "first!"})
	if _, err := h.Broadcast(context.Background(), "room-A", chatEnv, ""); err != nil {
		t.Fatalf("Broadcast: %v", err)
	}

	// Wait for both frames to drain, then assert the snapshot is frame 0.
	got := waitForPayload(t, fc, 2, time.Second)

	var first domain.Envelope
	if err := json.Unmarshal(got[0], &first); err != nil {
		t.Fatalf("decode first frame: %v", err)
	}
	if first.Type != domain.MsgRoomSnapshot {
		t.Fatalf("first drained frame Type = %q, want %q (snapshot must be first)", first.Type, domain.MsgRoomSnapshot)
	}
}

// ----------------------------------------------------------------------------
// The first frame must land on a brand-new connection's empty buffer (the
// 64-deep sendCh starts empty, so the non-blocking Send can never drop it).
// ----------------------------------------------------------------------------

func TestRegisterWithFirstFrame_NeverDropsOnEmptyBuffer(t *testing.T) {
	h, _, _ := newTestHub(t)

	snapshotEnv := sampleEnvelope(t, domain.MsgRoomSnapshot, map[string]any{"v": 1})
	snapshotPayload, err := json.Marshal(snapshotEnv)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	_, fc := registerFakeWithFirstFrame(t, h, "room-A", "alice", "Alice", snapshotPayload)

	got := waitForPayload(t, fc, 1, time.Second)
	var first domain.Envelope
	if err := json.Unmarshal(got[0], &first); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if first.Type != domain.MsgRoomSnapshot {
		t.Fatalf("first frame Type = %q, want snapshot", first.Type)
	}
}

// ----------------------------------------------------------------------------
// Backward-compat: registerInternalWithFirstFrame(nil) behaves exactly like
// the legacy registerInternal — no first frame is enqueued, the connection is
// eligible, and a subsequent Broadcast is delivered.
// ----------------------------------------------------------------------------

func TestRegisterWithFirstFrame_NilFrame_NoPrime(t *testing.T) {
	h, _, _ := newTestHub(t)

	_, fc := registerFakeWithFirstFrame(t, h, "room-A", "alice", "Alice", nil)

	chatEnv := sampleEnvelope(t, domain.MsgChatMessageOut, map[string]any{"body": "hi"})
	if _, err := h.Broadcast(context.Background(), "room-A", chatEnv, ""); err != nil {
		t.Fatalf("Broadcast: %v", err)
	}

	got := waitForPayload(t, fc, 1, time.Second)
	if len(got) != 1 {
		t.Fatalf("got %d frames, want 1 (no primed snapshot)", len(got))
	}
	var first domain.Envelope
	if err := json.Unmarshal(got[0], &first); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if first.Type != domain.MsgChatMessageOut {
		t.Fatalf("first frame Type = %q, want chat (no snapshot was primed)", first.Type)
	}
}
