package alert

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTelegram_Send_PostsSendMessage(t *testing.T) {
	var gotPath, gotChat string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotChat = r.Form.Get("chat_id")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	tg := NewTelegram("SECRET", "-100123", srv.URL, srv.Client())
	if err := tg.Send(context.Background(), "boom"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotPath, "/botSECRET/sendMessage") || gotChat != "-100123" {
		t.Fatalf("path=%s chat=%s", gotPath, gotChat)
	}
}

func TestNoop_Send_NoError(t *testing.T) {
	if err := NewNoop().Send(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
}
