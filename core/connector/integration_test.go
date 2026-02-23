package connector_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jdelaire/openslack/core/connector"
)

// buildSampleConnector compiles the sample connector to a temp dir and returns the path.
func buildSampleConnector(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "sample-connector")

	// Find the project root by walking up from the test file.
	// The connector source is at connectors/sample/main.go relative to repo root.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// We're in core/connector, go up two levels.
	root := filepath.Join(wd, "..", "..")
	src := filepath.Join(root, "connectors", "sample")

	cmd := exec.Command("go", "build", "-o", bin, src)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build sample-connector: %v\n%s", err, out)
	}
	return bin
}

func testConfig(bin string) *connector.Config {
	return &connector.Config{
		Connectors: map[string]connector.ConnectorConfig{
			"sample": {
				Exec:  bin,
				Tools: []string{"echo", "time", "sleep"},
			},
		},
		Limits: connector.LimitsConfig{
			ReqMaxBytes:   4096,
			RespMaxBytes:  16384,
			CallTimeoutMs: 5000,
		},
	}
}

func TestIntegrationEcho(t *testing.T) {
	bin := buildSampleConnector(t)
	cfg := testConfig(bin)
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mgr := connector.NewManager(cfg, logger)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer mgr.Shutdown()

	router := connector.NewRouter(cfg, mgr, logger)

	args := json.RawMessage(`{"text":"hello world"}`)
	resp, err := router.Call(context.Background(), "sample.echo", args)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok, got error: %v", resp.Error)
	}

	var data struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if data.Text != "hello world" {
		t.Errorf("text = %q, want %q", data.Text, "hello world")
	}
}

func TestIntegrationTime(t *testing.T) {
	bin := buildSampleConnector(t)
	cfg := testConfig(bin)
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mgr := connector.NewManager(cfg, logger)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer mgr.Shutdown()

	router := connector.NewRouter(cfg, mgr, logger)

	resp, err := router.Call(context.Background(), "sample.time", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok, got error: %v", resp.Error)
	}

	var data struct {
		Time string `json:"time"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if _, err := time.Parse(time.RFC3339, data.Time); err != nil {
		t.Errorf("invalid time format: %q", data.Time)
	}
}

func TestIntegrationIntrospect(t *testing.T) {
	bin := buildSampleConnector(t)
	cfg := testConfig(bin)
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mgr := connector.NewManager(cfg, logger)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer mgr.Shutdown()

	router := connector.NewRouter(cfg, mgr, logger)

	resp, err := router.Call(context.Background(), "sample.__introspect", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok, got error: %v", resp.Error)
	}

	var data connector.IntrospectData
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("unmarshal introspect: %v", err)
	}
	if data.Name != "sample" {
		t.Errorf("name = %q, want %q", data.Name, "sample")
	}
	if len(data.Tools) < 2 {
		t.Errorf("expected at least 2 tools, got %d", len(data.Tools))
	}
}

func TestIntegrationUnknownConnector(t *testing.T) {
	bin := buildSampleConnector(t)
	cfg := testConfig(bin)
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mgr := connector.NewManager(cfg, logger)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer mgr.Shutdown()

	router := connector.NewRouter(cfg, mgr, logger)

	_, err := router.Call(context.Background(), "unknown.echo", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for unknown connector")
	}
	if !strings.Contains(err.Error(), "unknown connector") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestIntegrationToolNotAllowed(t *testing.T) {
	bin := buildSampleConnector(t)
	cfg := &connector.Config{
		Connectors: map[string]connector.ConnectorConfig{
			"sample": {
				Exec:  bin,
				Tools: []string{"echo"}, // only echo is allowed
			},
		},
		Limits: connector.LimitsConfig{
			ReqMaxBytes:   4096,
			RespMaxBytes:  16384,
			CallTimeoutMs: 5000,
		},
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mgr := connector.NewManager(cfg, logger)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer mgr.Shutdown()

	router := connector.NewRouter(cfg, mgr, logger)

	_, err := router.Call(context.Background(), "sample.time", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for disallowed tool")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestIntegrationInvalidToolFormat(t *testing.T) {
	bin := buildSampleConnector(t)
	cfg := testConfig(bin)
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mgr := connector.NewManager(cfg, logger)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer mgr.Shutdown()

	router := connector.NewRouter(cfg, mgr, logger)

	_, err := router.Call(context.Background(), "nodot", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for invalid tool format")
	}
	if !strings.Contains(err.Error(), "must be connector.tool") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestIntegrationTimeout(t *testing.T) {
	bin := buildSampleConnector(t)
	cfg := &connector.Config{
		Connectors: map[string]connector.ConnectorConfig{
			"sample": {
				Exec:  bin,
				Tools: []string{"sleep"},
			},
		},
		Limits: connector.LimitsConfig{
			ReqMaxBytes:   4096,
			RespMaxBytes:  16384,
			CallTimeoutMs: 200, // 200ms timeout
		},
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mgr := connector.NewManager(cfg, logger)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer mgr.Shutdown()

	router := connector.NewRouter(cfg, mgr, logger)

	// Ask connector to sleep 2s but timeout is 200ms.
	_, err := router.Call(context.Background(), "sample.sleep", json.RawMessage(`{"ms":2000}`))
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestIntegrationEchoMissingText(t *testing.T) {
	bin := buildSampleConnector(t)
	cfg := testConfig(bin)
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mgr := connector.NewManager(cfg, logger)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer mgr.Shutdown()

	router := connector.NewRouter(cfg, mgr, logger)

	resp, err := router.Call(context.Background(), "sample.echo", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if resp.OK {
		t.Fatal("expected error response for missing text")
	}
	if resp.Error.Code != "INVALID_ARGS" {
		t.Errorf("code = %q, want INVALID_ARGS", resp.Error.Code)
	}
}

func TestIntegrationOversizedRequest(t *testing.T) {
	bin := buildSampleConnector(t)
	cfg := &connector.Config{
		Connectors: map[string]connector.ConnectorConfig{
			"sample": {
				Exec:  bin,
				Tools: []string{"echo"},
			},
		},
		Limits: connector.LimitsConfig{
			ReqMaxBytes:   100, // tiny limit
			RespMaxBytes:  16384,
			CallTimeoutMs: 5000,
		},
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mgr := connector.NewManager(cfg, logger)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer mgr.Shutdown()

	// Build a large payload that exceeds the 100 byte limit.
	bigText := strings.Repeat("x", 200)
	args := json.RawMessage(`{"text":"` + bigText + `"}`)

	req := &connector.Request{
		Version: connector.ProtocolVersion,
		ID:      "req_big",
		Tool:    "echo",
		Args:    args,
	}
	_, err := mgr.Call(context.Background(), "sample", req)
	if err == nil {
		t.Fatal("expected error for oversized request")
	}
	if !strings.Contains(err.Error(), "byte limit") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestIntegrationMultipleCalls(t *testing.T) {
	bin := buildSampleConnector(t)
	cfg := testConfig(bin)
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mgr := connector.NewManager(cfg, logger)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer mgr.Shutdown()

	router := connector.NewRouter(cfg, mgr, logger)

	// Multiple sequential calls should work.
	for i := 0; i < 5; i++ {
		resp, err := router.Call(context.Background(), "sample.echo", json.RawMessage(`{"text":"ping"}`))
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if !resp.OK {
			t.Fatalf("call %d: not ok", i)
		}
	}
}
