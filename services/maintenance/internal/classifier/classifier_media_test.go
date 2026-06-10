package classifier

import (
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/telegram"
)

var admins = []string{"adminuser"}

func humanMsg(id int, username, text string) *telegram.Message {
	return &telegram.Message{
		MessageID: id,
		From:      &telegram.UserInfo{ID: 100, Username: username},
		Chat:      &telegram.Chat{ID: -1},
		Text:      text,
	}
}

func TestClassify_CaptionFallbackAndAttachment(t *testing.T) {
	m := humanMsg(10, "someuser", "")
	m.Caption = "плеер сломался, вот скрин"
	m.Photo = []telegram.PhotoSize{
		{FileID: "small", Width: 90, Height: 90},
		{FileID: "big", Width: 1280, Height: 720, FileSize: 12345},
	}

	got := Classify(telegram.Update{UpdateID: 1, Message: m}, admins)
	if got.Type != domain.MessageUserIssue {
		t.Fatalf("expected UserIssue, got %v", got.Type)
	}
	if got.Text != "плеер сломался, вот скрин" {
		t.Errorf("caption must become text, got %q", got.Text)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].FileID != "big" || got.Attachments[0].Kind != "photo" {
		t.Errorf("expected largest photo attachment, got %+v", got.Attachments)
	}
}

func TestClassify_AttachmentWithoutKeywordsIsRelevant(t *testing.T) {
	m := humanMsg(11, "someuser", "")
	m.Document = &telegram.Document{FileID: "doc1", FileName: "logs.txt", MimeType: "text/plain"}

	got := Classify(telegram.Update{UpdateID: 2, Message: m}, admins)
	if got.Type != domain.MessageUserIssue {
		t.Fatalf("document message must be relevant, got %v", got.Type)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].FileName != "logs.txt" {
		t.Errorf("document attachment missing: %+v", got.Attachments)
	}
}

func TestClassify_ForwardAndReplyContext(t *testing.T) {
	m := humanMsg(12, "someuser", "вот что пишут")
	m.ForwardOrigin = &telegram.ForwardOrigin{Type: "channel", Chat: &telegram.Chat{ID: -5, Title: "News RU"}}
	m.ReplyTo = &telegram.Message{
		MessageID: 5,
		From:      &telegram.UserInfo{ID: 999, Username: "maintbot", IsBot: true},
		Text:      "Issue AUTO-1 created",
	}

	got := Classify(telegram.Update{UpdateID: 3, Message: m}, admins)
	if got.Type != domain.MessageUserIssue {
		t.Fatalf("forwarded message must be relevant, got %v", got.Type)
	}
	if got.ForwardedFrom != "News RU" {
		t.Errorf("forward origin label: %q", got.ForwardedFrom)
	}
	if got.ReplyToText == "" {
		t.Errorf("reply context missing")
	}
	for _, want := range []string{"[Forwarded from News RU]", "вот что пишут", "Issue AUTO-1 created", "(bot)"} {
		if !strings.Contains(got.Text, want) {
			t.Errorf("composed text missing %q: %q", want, got.Text)
		}
	}
}

func TestClassifyBatch_MergesAlbum(t *testing.T) {
	head := humanMsg(20, "someuser", "")
	head.Caption = "не работает плеер" // keyword → relevant
	head.MediaGroupID = "g1"
	head.Photo = []telegram.PhotoSize{{FileID: "p1"}}

	tail := humanMsg(21, "someuser", "")
	tail.MediaGroupID = "g1"
	tail.Photo = []telegram.PhotoSize{{FileID: "p2"}}

	batch := ClassifyBatch([]telegram.Update{
		{UpdateID: 10, Message: head},
		{UpdateID: 11, Message: tail},
	}, admins)

	if len(batch.Relevant) != 1 {
		t.Fatalf("album must merge into 1 relevant message, got %d", len(batch.Relevant))
	}
	if len(batch.Relevant[0].Attachments) != 2 {
		t.Errorf("merged attachments: %+v", batch.Relevant[0].Attachments)
	}
	if batch.Relevant[0].Text != "не работает плеер" {
		t.Errorf("caption lost on merge: %q", batch.Relevant[0].Text)
	}
}

func TestClassify_PlainChatterStillIgnored(t *testing.T) {
	got := Classify(telegram.Update{UpdateID: 4, Message: humanMsg(30, "someuser", "привет всем")}, admins)
	if got.Type != domain.MessageIgnore {
		t.Errorf("plain chatter must stay ignored, got %v", got.Type)
	}
}
