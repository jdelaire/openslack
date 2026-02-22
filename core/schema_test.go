package core

import (
	"strings"
	"testing"
)

func TestValidateRequest_ValidNotify(t *testing.T) {
	data := []byte(`{"version":1,"action":"notify","payload":{"text":"hello"}}`)
	req, err := ValidateRequest(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Action != "notify" {
		t.Errorf("expected action notify, got %s", req.Action)
	}
	if req.Version != 1 {
		t.Errorf("expected version 1, got %d", req.Version)
	}
}

func TestValidateRequest_WithSource(t *testing.T) {
	data := []byte(`{"version":1,"action":"notify","payload":{"text":"hello","source":"test-app"}}`)
	_, err := ValidateRequest(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRequest_PayloadTooLarge(t *testing.T) {
	big := `{"version":1,"action":"notify","payload":{"text":"` + strings.Repeat("x", MaxPayloadBytes) + `"}}`
	_, err := ValidateRequest([]byte(big))
	if err == nil {
		t.Fatal("expected error for oversized payload")
	}
	if !strings.Contains(err.Error(), "byte limit") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateRequest_BadVersion(t *testing.T) {
	data := []byte(`{"version":99,"action":"notify","payload":{"text":"hello"}}`)
	_, err := ValidateRequest(data)
	if err == nil {
		t.Fatal("expected error for bad version")
	}
	if !strings.Contains(err.Error(), "unsupported version") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateRequest_UnknownAction(t *testing.T) {
	data := []byte(`{"version":1,"action":"delete","payload":{}}`)
	_, err := ValidateRequest(data)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
	if !strings.Contains(err.Error(), "unknown action") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateRequest_EmptyText(t *testing.T) {
	data := []byte(`{"version":1,"action":"notify","payload":{"text":""}}`)
	_, err := ValidateRequest(data)
	if err == nil {
		t.Fatal("expected error for empty text")
	}
	if !strings.Contains(err.Error(), "text is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateRequest_TextTooLong(t *testing.T) {
	long := `{"version":1,"action":"notify","payload":{"text":"` + strings.Repeat("a", MaxTextLen+1) + `"}}`
	_, err := ValidateRequest([]byte(long))
	if err == nil {
		t.Fatal("expected error for text too long")
	}
	if !strings.Contains(err.Error(), "character limit") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateRequest_SourceTooLong(t *testing.T) {
	long := `{"version":1,"action":"notify","payload":{"text":"hi","source":"` + strings.Repeat("s", MaxSourceLen+1) + `"}}`
	_, err := ValidateRequest([]byte(long))
	if err == nil {
		t.Fatal("expected error for source too long")
	}
	if !strings.Contains(err.Error(), "source exceeds") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateRequest_UnknownField(t *testing.T) {
	data := []byte(`{"version":1,"action":"notify","payload":{"text":"hi"},"extra":"bad"}`)
	_, err := ValidateRequest(data)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestValidateRequest_UnknownPayloadField(t *testing.T) {
	data := []byte(`{"version":1,"action":"notify","payload":{"text":"hi","evil":"field"}}`)
	_, err := ValidateRequest(data)
	if err == nil {
		t.Fatal("expected error for unknown payload field")
	}
}

func TestValidateRequest_InvalidJSON(t *testing.T) {
	_, err := ValidateRequest([]byte(`{not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseNotifyPayload(t *testing.T) {
	data := []byte(`{"text":"hello","source":"cli"}`)
	p, err := ParseNotifyPayload(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Text != "hello" {
		t.Errorf("expected text hello, got %s", p.Text)
	}
	if p.Source != "cli" {
		t.Errorf("expected source cli, got %s", p.Source)
	}
}
