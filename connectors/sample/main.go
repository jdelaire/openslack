package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const connectorVersion = "1.0.0"

type request struct {
	Version string          `json:"version"`
	ID      string          `json:"id"`
	Tool    string          `json:"tool"`
	Args    json.RawMessage `json:"args"`
}

type response struct {
	Version string          `json:"version"`
	ID      string          `json:"id"`
	OK      bool            `json:"ok"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   *respError      `json:"error,omitempty"`
}

type respError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func main() {
	fmt.Fprintln(os.Stderr, "sample-connector started")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	for scanner.Scan() {
		line := scanner.Bytes()

		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			writeError("", "INVALID_REQUEST", fmt.Sprintf("invalid json: %s", err))
			continue
		}

		if req.Version != "v1" {
			writeError(req.ID, "INVALID_REQUEST", fmt.Sprintf("unsupported version: %s", req.Version))
			continue
		}

		resp := handle(req)
		out, _ := json.Marshal(resp)
		fmt.Fprintln(os.Stdout, string(out))
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "stdin error: %s\n", err)
		os.Exit(1)
	}
}

func handle(req request) response {
	switch req.Tool {
	case "__introspect":
		return handleIntrospect(req)
	case "echo":
		return handleEcho(req)
	case "time":
		return handleTime(req)
	case "sleep":
		return handleSleep(req)
	default:
		return response{
			Version: "v1",
			ID:      req.ID,
			OK:      false,
			Error:   &respError{Code: "NOT_SUPPORTED", Message: fmt.Sprintf("unknown tool: %s", req.Tool)},
		}
	}
}

func handleIntrospect(req request) response {
	data, _ := json.Marshal(map[string]interface{}{
		"name":    "sample",
		"version": connectorVersion,
		"tools": []map[string]string{
			{"name": "echo"},
			{"name": "time"},
			{"name": "sleep"},
		},
	})
	return response{Version: "v1", ID: req.ID, OK: true, Data: data}
}

func handleEcho(req request) response {
	var args struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(req.Args, &args); err != nil {
		return response{
			Version: "v1", ID: req.ID, OK: false,
			Error: &respError{Code: "INVALID_ARGS", Message: "invalid args"},
		}
	}
	if args.Text == "" {
		return response{
			Version: "v1", ID: req.ID, OK: false,
			Error: &respError{Code: "INVALID_ARGS", Message: "text is required"},
		}
	}
	data, _ := json.Marshal(map[string]string{"text": args.Text})
	return response{Version: "v1", ID: req.ID, OK: true, Data: data}
}

func handleTime(req request) response {
	data, _ := json.Marshal(map[string]string{"time": time.Now().Format(time.RFC3339)})
	return response{Version: "v1", ID: req.ID, OK: true, Data: data}
}

// handleSleep is a test tool that sleeps for a specified duration.
// Used to validate timeout enforcement.
func handleSleep(req request) response {
	var args struct {
		Ms int `json:"ms"`
	}
	if err := json.Unmarshal(req.Args, &args); err != nil || args.Ms <= 0 {
		return response{
			Version: "v1", ID: req.ID, OK: false,
			Error: &respError{Code: "INVALID_ARGS", Message: "ms must be a positive integer"},
		}
	}
	time.Sleep(time.Duration(args.Ms) * time.Millisecond)
	data, _ := json.Marshal(map[string]string{"slept": fmt.Sprintf("%dms", args.Ms)})
	return response{Version: "v1", ID: req.ID, OK: true, Data: data}
}

func writeError(id, code, message string) {
	resp := response{
		Version: "v1",
		ID:      id,
		OK:      false,
		Error:   &respError{Code: code, Message: message},
	}
	out, _ := json.Marshal(resp)
	fmt.Fprintln(os.Stdout, string(out))
}
