package telegram

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAllowedUpdatesIncludesReactions(t *testing.T) {
	if !strings.Contains(allowedUpdates, "message_reaction") {
		t.Fatalf("allowedUpdates must request message_reaction, got %q", allowedUpdates)
	}
}

func TestUnmarshalMessageReactionUpdate(t *testing.T) {
	raw := `[{
		"update_id": 100,
		"message_reaction": {
			"chat": {"id": -1003753190340, "type": "supergroup"},
			"message_id": 4242,
			"user": {"id": 898912046, "is_bot": false, "username": "tNeymik"},
			"old_reaction": [],
			"new_reaction": [{"type": "emoji", "emoji": "💔"}]
		}
	}]`

	var updates []Update
	if err := json.Unmarshal([]byte(raw), &updates); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("want 1 update, got %d", len(updates))
	}
	r := updates[0].MessageReaction
	if r == nil {
		t.Fatal("MessageReaction is nil — field not parsed")
	}
	if r.MessageID != 4242 {
		t.Errorf("MessageID = %d, want 4242", r.MessageID)
	}
	if r.User == nil || r.User.ID != 898912046 {
		t.Errorf("User.ID not parsed: %+v", r.User)
	}
	if len(r.NewReaction) != 1 || r.NewReaction[0].Type != "emoji" || r.NewReaction[0].Emoji != "\U0001F494" {
		t.Errorf("NewReaction not parsed: %+v", r.NewReaction)
	}
}
