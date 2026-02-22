package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type echoNotifier struct {
	sent []Notification
}

func (e *echoNotifier) Name() string { return "echo" }
func (e *echoNotifier) Send(_ context.Context, n Notification) error {
	e.sent = append(e.sent, n)
	return nil
}

type failNotifier struct{}

func (f *failNotifier) Name() string { return "fail" }
func (f *failNotifier) Send(_ context.Context, _ Notification) error {
	return fmt.Errorf("delivery failed")
}

func setupTestServer(t *testing.T, notifiers ...Notifier) (*Server, string, context.CancelFunc) {
	t.Helper()
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	reg := NewRegistry()
	for _, n := range notifiers {
		if err := reg.Register(n); err != nil {
			t.Fatalf("register notifier: %v", err)
		}
	}

	srv := NewServer(sockPath, reg, logger)
	ctx, cancel := context.WithCancel(context.Background())

	if err := srv.Start(ctx); err != nil {
		cancel()
		t.Fatalf("start server: %v", err)
	}

	return srv, sockPath, cancel
}

func sendRequest(t *testing.T, sockPath string, data []byte) Response {
	t.Helper()
	conn, err := net.DialTimeout("unix", sockPath, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(2 * time.Second))

	if _, err := conn.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Signal we're done writing so server's ReadAll returns.
	conn.(*net.UnixConn).CloseWrite()

	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

func TestServer_NotifySuccess(t *testing.T) {
	echo := &echoNotifier{}
	srv, sockPath, cancel := setupTestServer(t, echo)
	defer func() { cancel(); srv.Shutdown() }()

	data := []byte(`{"version":1,"action":"notify","payload":{"text":"hello","source":"test"}}`)
	resp := sendRequest(t, sockPath, data)

	if !resp.OK {
		t.Fatalf("expected ok, got error: %s", resp.Error)
	}
	if resp.ID == "" {
		t.Error("expected non-empty ID")
	}
	if len(echo.sent) != 1 {
		t.Fatalf("expected 1 sent notification, got %d", len(echo.sent))
	}
	if echo.sent[0].Text != "hello" {
		t.Errorf("expected text hello, got %s", echo.sent[0].Text)
	}
	if echo.sent[0].Source != "test" {
		t.Errorf("expected source test, got %s", echo.sent[0].Source)
	}
}

func TestServer_InvalidJSON(t *testing.T) {
	srv, sockPath, cancel := setupTestServer(t, &echoNotifier{})
	defer func() { cancel(); srv.Shutdown() }()

	resp := sendRequest(t, sockPath, []byte(`{bad`))
	if resp.OK {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestServer_UnknownAction(t *testing.T) {
	srv, sockPath, cancel := setupTestServer(t, &echoNotifier{})
	defer func() { cancel(); srv.Shutdown() }()

	data := []byte(`{"version":1,"action":"delete","payload":{}}`)
	resp := sendRequest(t, sockPath, data)
	if resp.OK {
		t.Fatal("expected error for unknown action")
	}
	if !strings.Contains(resp.Error, "unknown action") {
		t.Errorf("unexpected error: %s", resp.Error)
	}
}

func TestServer_DeliveryFailure(t *testing.T) {
	srv, sockPath, cancel := setupTestServer(t, &failNotifier{})
	defer func() { cancel(); srv.Shutdown() }()

	data := []byte(`{"version":1,"action":"notify","payload":{"text":"hello"}}`)
	resp := sendRequest(t, sockPath, data)
	if resp.OK {
		t.Fatal("expected error for delivery failure")
	}
	if !strings.Contains(resp.Error, "delivery failed") {
		t.Errorf("unexpected error: %s", resp.Error)
	}
}

func TestServer_PayloadTooLarge(t *testing.T) {
	srv, sockPath, cancel := setupTestServer(t, &echoNotifier{})
	defer func() { cancel(); srv.Shutdown() }()

	big := []byte(`{"version":1,"action":"notify","payload":{"text":"` + strings.Repeat("x", MaxPayloadBytes) + `"}}`)
	resp := sendRequest(t, sockPath, big)
	if resp.OK {
		t.Fatal("expected error for oversized payload")
	}
	if !strings.Contains(resp.Error, "byte limit") {
		t.Errorf("unexpected error: %s", resp.Error)
	}
}

func TestServer_SocketPermissions(t *testing.T) {
	srv, sockPath, cancel := setupTestServer(t, &echoNotifier{})
	defer func() { cancel(); srv.Shutdown() }()

	info, err := os.Stat(sockPath)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("expected socket permissions 0600, got %o", perm)
	}
}

func TestServer_DirectoryPermissions(t *testing.T) {
	// Use /tmp for shorter path — macOS Unix socket paths max 104 chars.
	dir, err := os.MkdirTemp("/tmp", "osd")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(dir)

	subdir := filepath.Join(dir, "sub")
	sockPath := filepath.Join(subdir, "t.sock")
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	reg := NewRegistry()
	reg.Register(&echoNotifier{})
	srv := NewServer(sockPath, reg, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer srv.Shutdown()

	info, err := os.Stat(subdir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("expected directory permissions 0700, got %o", perm)
	}
}

func TestServer_StaleSocketCleanup(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	// Create a stale socket file.
	os.WriteFile(sockPath, []byte("stale"), 0600)

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	reg := NewRegistry()
	reg.Register(&echoNotifier{})
	srv := NewServer(sockPath, reg, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start failed with stale socket: %v", err)
	}
	defer srv.Shutdown()

	// Verify server works.
	data := []byte(`{"version":1,"action":"notify","payload":{"text":"after cleanup"}}`)
	resp := sendRequest(t, sockPath, data)
	if !resp.OK {
		t.Fatalf("expected ok after stale cleanup, got: %s", resp.Error)
	}
}

func TestServer_GracefulShutdown(t *testing.T) {
	srv, sockPath, cancel := setupTestServer(t, &echoNotifier{})

	cancel()
	srv.Shutdown()

	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Error("expected socket file to be removed after shutdown")
	}
}

func TestServer_MultipleConnections(t *testing.T) {
	echo := &echoNotifier{}
	srv, sockPath, cancel := setupTestServer(t, echo)
	defer func() { cancel(); srv.Shutdown() }()

	for i := 0; i < 5; i++ {
		data := []byte(fmt.Sprintf(`{"version":1,"action":"notify","payload":{"text":"msg %d"}}`, i))
		resp := sendRequest(t, sockPath, data)
		if !resp.OK {
			t.Fatalf("request %d failed: %s", i, resp.Error)
		}
	}

	if len(echo.sent) != 5 {
		t.Errorf("expected 5 sent, got %d", len(echo.sent))
	}
}
