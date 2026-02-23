package connector

import (
	"encoding/json"
	"fmt"
)

const ProtocolVersion = "v1"

// Request is the JSON envelope sent to a connector over stdin.
type Request struct {
	Version string          `json:"version"`
	ID      string          `json:"id"`
	Tool    string          `json:"tool"`
	Args    json.RawMessage `json:"args"`
	Meta    *RequestMeta    `json:"meta,omitempty"`
}

// RequestMeta carries optional tracing metadata.
type RequestMeta struct {
	TraceID   string `json:"trace_id,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
}

// Response is the JSON envelope read from a connector's stdout.
type Response struct {
	Version string          `json:"version"`
	ID      string          `json:"id"`
	OK      bool            `json:"ok"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
}

// ResponseError describes a structured error from a connector.
type ResponseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ResponseError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Error codes.
const (
	ErrInvalidArgs    = "INVALID_ARGS"
	ErrNotSupported   = "NOT_SUPPORTED"
	ErrInternal       = "INTERNAL"
	ErrTimeout        = "TIMEOUT"
	ErrUnauthorized   = "UNAUTHORIZED"
	ErrInvalidRequest = "INVALID_REQUEST"
)

// IntrospectData is returned by the __introspect tool.
type IntrospectData struct {
	Name    string          `json:"name"`
	Version string          `json:"version"`
	Tools   []IntrospectTool `json:"tools"`
}

// IntrospectTool describes a single tool a connector exposes.
type IntrospectTool struct {
	Name string `json:"name"`
}

// IntrospectToolName is the reserved tool name for introspection.
const IntrospectToolName = "__introspect"

// ValidateRequest checks a request for protocol correctness.
func ValidateRequest(req *Request) error {
	if req.Version != ProtocolVersion {
		return fmt.Errorf("unsupported protocol version %q, expected %q", req.Version, ProtocolVersion)
	}
	if req.ID == "" {
		return fmt.Errorf("request id is required")
	}
	if req.Tool == "" {
		return fmt.Errorf("tool name is required")
	}
	if req.Args == nil {
		return fmt.Errorf("args is required")
	}
	return nil
}

// ValidateResponse checks a response for protocol correctness.
func ValidateResponse(resp *Response) error {
	if resp.Version != ProtocolVersion {
		return fmt.Errorf("unsupported protocol version %q", resp.Version)
	}
	if resp.ID == "" {
		return fmt.Errorf("response id is required")
	}
	if !resp.OK && resp.Error == nil {
		return fmt.Errorf("error response must include error object")
	}
	return nil
}

// NewErrorResponse creates an error response for a given request ID.
func NewErrorResponse(id, code, message string) *Response {
	return &Response{
		Version: ProtocolVersion,
		ID:      id,
		OK:      false,
		Error:   &ResponseError{Code: code, Message: message},
	}
}
