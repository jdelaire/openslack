package connector

import (
	"encoding/json"
	"testing"
)

func TestValidateRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     Request
		wantErr bool
	}{
		{
			name:    "valid",
			req:     Request{Version: "v1", ID: "req_1", Tool: "echo", Args: json.RawMessage(`{}`)},
			wantErr: false,
		},
		{
			name:    "bad version",
			req:     Request{Version: "v2", ID: "req_1", Tool: "echo", Args: json.RawMessage(`{}`)},
			wantErr: true,
		},
		{
			name:    "missing id",
			req:     Request{Version: "v1", Tool: "echo", Args: json.RawMessage(`{}`)},
			wantErr: true,
		},
		{
			name:    "missing tool",
			req:     Request{Version: "v1", ID: "req_1", Args: json.RawMessage(`{}`)},
			wantErr: true,
		},
		{
			name:    "missing args",
			req:     Request{Version: "v1", ID: "req_1", Tool: "echo"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRequest(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateResponse(t *testing.T) {
	tests := []struct {
		name    string
		resp    Response
		wantErr bool
	}{
		{
			name:    "valid ok",
			resp:    Response{Version: "v1", ID: "req_1", OK: true},
			wantErr: false,
		},
		{
			name:    "valid error",
			resp:    Response{Version: "v1", ID: "req_1", OK: false, Error: &ResponseError{Code: "INTERNAL", Message: "fail"}},
			wantErr: false,
		},
		{
			name:    "error without error object",
			resp:    Response{Version: "v1", ID: "req_1", OK: false},
			wantErr: true,
		},
		{
			name:    "bad version",
			resp:    Response{Version: "v2", ID: "req_1", OK: true},
			wantErr: true,
		},
		{
			name:    "missing id",
			resp:    Response{Version: "v1", OK: true},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateResponse(&tt.resp)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateResponse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewErrorResponse(t *testing.T) {
	resp := NewErrorResponse("req_42", ErrInvalidArgs, "bad input")
	if resp.OK {
		t.Error("expected OK=false")
	}
	if resp.ID != "req_42" {
		t.Errorf("ID = %q, want %q", resp.ID, "req_42")
	}
	if resp.Error.Code != ErrInvalidArgs {
		t.Errorf("Code = %q, want %q", resp.Error.Code, ErrInvalidArgs)
	}
	if resp.Error.Message != "bad input" {
		t.Errorf("Message = %q, want %q", resp.Error.Message, "bad input")
	}
}
