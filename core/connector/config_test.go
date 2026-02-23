package connector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "connectors.json")
	data := `{
		"connectors": {
			"sample": {
				"exec": "./bin/sample-connector",
				"tools": ["echo", "time"]
			}
		},
		"limits": {
			"req_max_bytes": 2048,
			"resp_max_bytes": 8192,
			"call_timeout_ms": 5000
		}
	}`
	os.WriteFile(path, []byte(data), 0644)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if len(cfg.Connectors) != 1 {
		t.Fatalf("expected 1 connector, got %d", len(cfg.Connectors))
	}
	cc := cfg.Connectors["sample"]
	if cc.Exec != "./bin/sample-connector" {
		t.Errorf("exec = %q", cc.Exec)
	}
	if len(cc.Tools) != 2 {
		t.Errorf("tools = %v", cc.Tools)
	}
	if cfg.Limits.ReqMaxBytes != 2048 {
		t.Errorf("req_max_bytes = %d", cfg.Limits.ReqMaxBytes)
	}
	if cfg.Limits.RespMaxBytes != 8192 {
		t.Errorf("resp_max_bytes = %d", cfg.Limits.RespMaxBytes)
	}
	if cfg.Limits.CallTimeoutMs != 5000 {
		t.Errorf("call_timeout_ms = %d", cfg.Limits.CallTimeoutMs)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "connectors.json")
	data := `{"connectors":{"sample":{"exec":"./bin/sample","tools":["echo"]}}}`
	os.WriteFile(path, []byte(data), 0644)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Limits.ReqMaxBytes != DefaultReqMaxBytes {
		t.Errorf("req_max_bytes = %d, want %d", cfg.Limits.ReqMaxBytes, DefaultReqMaxBytes)
	}
	if cfg.Limits.RespMaxBytes != DefaultRespMaxBytes {
		t.Errorf("resp_max_bytes = %d, want %d", cfg.Limits.RespMaxBytes, DefaultRespMaxBytes)
	}
	if cfg.Limits.CallTimeoutMs != DefaultCallTimeoutMs {
		t.Errorf("call_timeout_ms = %d, want %d", cfg.Limits.CallTimeoutMs, DefaultCallTimeoutMs)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/connectors.json")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if cfg != nil {
		t.Fatal("expected nil config")
	}
}

func TestLoadConfigMissingExec(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "connectors.json")
	os.WriteFile(path, []byte(`{"connectors":{"sample":{"tools":["echo"]}}}`), 0644)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for missing exec")
	}
	if !strings.Contains(err.Error(), "missing exec path") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadConfigNoTools(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "connectors.json")
	os.WriteFile(path, []byte(`{"connectors":{"sample":{"exec":"./bin/sample","tools":[]}}}`), 0644)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for empty tools")
	}
	if !strings.Contains(err.Error(), "no allowed tools") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadConfigDotInName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "connectors.json")
	os.WriteFile(path, []byte(`{"connectors":{"my.conn":{"exec":"./bin/x","tools":["a"]}}}`), 0644)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for dot in connector name")
	}
	if !strings.Contains(err.Error(), "must not contain dots") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestToolAllowed(t *testing.T) {
	cc := ConnectorConfig{Tools: []string{"echo", "time"}}

	if !cc.ToolAllowed("echo") {
		t.Error("expected echo to be allowed")
	}
	if !cc.ToolAllowed("time") {
		t.Error("expected time to be allowed")
	}
	if cc.ToolAllowed("delete") {
		t.Error("expected delete to be rejected")
	}
	if !cc.ToolAllowed(IntrospectToolName) {
		t.Error("expected __introspect to always be allowed")
	}
}
