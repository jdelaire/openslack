package telegram_notifier

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jdelaire/openslack/core"
)

func newTestNotification() core.Notification {
	return core.Notification{
		ID:        "test-id",
		Text:      "hello from test",
		Source:    "test",
		CreatedAt: time.Now(),
	}
}

func TestNotifier_SendSuccess(t *testing.T) {
	var receivedChatID, receivedText string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		r.ParseForm()
		receivedChatID = r.FormValue("chat_id")
		receivedText = r.FormValue("text")

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	n := New("test-token", "12345").WithBaseURL(server.URL)
	err := n.Send(context.Background(), newTestNotification())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedChatID != "12345" {
		t.Errorf("expected chat_id 12345, got %s", receivedChatID)
	}
	if receivedText != "hello from test" {
		t.Errorf("expected text 'hello from test', got %s", receivedText)
	}
}

func TestNotifier_SendAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"ok":false,"description":"Bad Request: chat not found"}`))
	}))
	defer server.Close()

	n := New("test-token", "bad-chat").WithBaseURL(server.URL)
	err := n.Send(context.Background(), newTestNotification())
	if err == nil {
		t.Fatal("expected error for API error response")
	}
	if !strings.Contains(err.Error(), "chat not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNotifier_SendNetworkError(t *testing.T) {
	n := New("test-token", "12345").WithBaseURL("http://127.0.0.1:1")
	err := n.Send(context.Background(), newTestNotification())
	if err == nil {
		t.Fatal("expected error for network failure")
	}
}

func TestNotifier_Name(t *testing.T) {
	n := New("token", "chat")
	if n.Name() != "telegram" {
		t.Errorf("expected name 'telegram', got %s", n.Name())
	}
}

func TestNotifier_BotTokenInURL(t *testing.T) {
	var requestedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	n := New("my-secret-token", "12345").WithBaseURL(server.URL)
	n.Send(context.Background(), newTestNotification())

	if requestedPath != "/botmy-secret-token/sendMessage" {
		t.Errorf("unexpected path: %s", requestedPath)
	}
}
