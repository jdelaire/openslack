package core

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const (
	MaxPayloadBytes = 8192
	MaxTextLen      = 4096
	MaxSourceLen    = 128
	CurrentVersion  = 1
)

// Request is the JSON envelope sent over the socket.
type Request struct {
	Version int             `json:"version"`
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload"`
}

// NotifyPayload is the payload for the "notify" action.
type NotifyPayload struct {
	Text   string `json:"text"`
	Source string `json:"source,omitempty"`
}

// Response is the JSON envelope sent back to the client.
type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	ID    string `json:"id,omitempty"`
}

// ValidateRequest checks the request envelope and returns a typed payload for known actions.
func ValidateRequest(data []byte) (*Request, error) {
	if len(data) > MaxPayloadBytes {
		return nil, fmt.Errorf("payload exceeds %d byte limit", MaxPayloadBytes)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var req Request
	if err := dec.Decode(&req); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	if req.Version != CurrentVersion {
		return nil, fmt.Errorf("unsupported version %d, expected %d", req.Version, CurrentVersion)
	}

	switch req.Action {
	case "notify":
		if err := validateNotifyPayload(req.Payload); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown action %q", req.Action)
	}

	return &req, nil
}

func validateNotifyPayload(raw json.RawMessage) error {
	if raw == nil {
		return fmt.Errorf("missing payload")
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	var p NotifyPayload
	if err := dec.Decode(&p); err != nil {
		return fmt.Errorf("invalid notify payload: %w", err)
	}

	if p.Text == "" {
		return fmt.Errorf("text is required")
	}
	if len(p.Text) > MaxTextLen {
		return fmt.Errorf("text exceeds %d character limit", MaxTextLen)
	}
	if len(p.Source) > MaxSourceLen {
		return fmt.Errorf("source exceeds %d character limit", MaxSourceLen)
	}

	return nil
}

// ParseNotifyPayload extracts the NotifyPayload from a validated request.
func ParseNotifyPayload(raw json.RawMessage) (NotifyPayload, error) {
	var p NotifyPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return NotifyPayload{}, err
	}
	return p, nil
}

