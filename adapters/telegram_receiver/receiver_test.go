package telegram_receiver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jdelaire/openslack/adapters/telegram_receiver"
	"github.com/jdelaire/openslack/core"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestPollSuccess(t *testing.T) {
	var mu sync.Mutex
	var received []core.InboundMessage

	handler := func(msg core.InboundMessage) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	}

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": []map[string]any{
					{
						"update_id": 100,
						"message": map[string]any{
							"message_id": 1,
							"from":       map[string]any{"id": 42},
							"chat":       map[string]any{"id": 123},
							"date":       time.Now().Unix(),
							"text":       "/status",
						},
					},
				},
			})
		} else {
			// Block until context is cancelled (simulates long poll).
			<-r.Context().Done()
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	recv := telegram_receiver.New("test-token", handler, testLogger()).WithBaseURL(srv.URL)
	recv.Start(ctx)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("received %d messages, want 1", len(received))
	}
	if received[0].Text != "/status" {
		t.Errorf("text = %q, want /status", received[0].Text)
	}
	if received[0].ChatID != 123 {
		t.Errorf("chatID = %d, want 123", received[0].ChatID)
	}
	if received[0].UserID != 42 {
		t.Errorf("userID = %d, want 42", received[0].UserID)
	}
	if received[0].UpdateID != 100 {
		t.Errorf("updateID = %d, want 100", received[0].UpdateID)
	}
}

func TestEmptyResult(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 2 {
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": []any{}})
		} else {
			<-r.Context().Done()
		}
	}))
	defer srv.Close()

	var received []core.InboundMessage
	handler := func(msg core.InboundMessage) {
		received = append(received, msg)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	recv := telegram_receiver.New("tok", handler, testLogger()).WithBaseURL(srv.URL)
	recv.Start(ctx)

	if len(received) != 0 {
		t.Errorf("received %d messages, want 0", len(received))
	}
}

func TestSkipsNoText(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": []map[string]any{
					{
						"update_id": 50,
						"message": map[string]any{
							"message_id": 1,
							"chat":       map[string]any{"id": 10},
							"date":       time.Now().Unix(),
							"text":       "",
						},
					},
				},
			})
		} else {
			<-r.Context().Done()
		}
	}))
	defer srv.Close()

	var received []core.InboundMessage
	handler := func(msg core.InboundMessage) {
		received = append(received, msg)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	recv := telegram_receiver.New("tok", handler, testLogger()).WithBaseURL(srv.URL)
	recv.Start(ctx)

	if len(received) != 0 {
		t.Errorf("received %d messages, want 0 (no text)", len(received))
	}
}

func TestOffsetIncrement(t *testing.T) {
	var offsets []string
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offsets = append(offsets, r.URL.Query().Get("offset"))
		callCount++
		if callCount == 1 {
			json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": []map[string]any{
					{
						"update_id": 200,
						"message": map[string]any{
							"message_id": 1,
							"from":       map[string]any{"id": 1},
							"chat":       map[string]any{"id": 1},
							"date":       time.Now().Unix(),
							"text":       "hello",
						},
					},
				},
			})
		} else if callCount == 2 {
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": []any{}})
		} else {
			<-r.Context().Done()
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	recv := telegram_receiver.New("tok", func(_ core.InboundMessage) {}, testLogger()).WithBaseURL(srv.URL)
	recv.Start(ctx)

	if len(offsets) < 2 {
		t.Fatalf("expected at least 2 polls, got %d", len(offsets))
	}
	if offsets[0] != "0" {
		t.Errorf("first offset = %q, want '0'", offsets[0])
	}
	if offsets[1] != "201" {
		t.Errorf("second offset = %q, want '201'", offsets[1])
	}
}

func TestContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	recv := telegram_receiver.New("tok", func(_ core.InboundMessage) {}, testLogger()).WithBaseURL(srv.URL)

	go func() {
		recv.Start(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("receiver did not stop after context cancellation")
	}
}

func TestAPIErrorBackoff(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, "internal error")
		} else {
			<-r.Context().Done()
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	recv := telegram_receiver.New("tok", func(_ core.InboundMessage) {}, testLogger()).WithBaseURL(srv.URL)
	recv.Start(ctx)

	// Should have retried after backoff.
	if callCount < 2 {
		t.Errorf("expected at least 2 calls (with backoff), got %d", callCount)
	}
}
