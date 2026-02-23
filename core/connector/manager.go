package connector

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"time"
)

// Manager owns the lifecycle of connector processes and routes calls.
type Manager struct {
	cfg    *Config
	logger *slog.Logger

	mu    sync.RWMutex
	procs map[string]*connectorProc
}

// connectorProc tracks a running connector child process.
type connectorProc struct {
	name   string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	mu     sync.Mutex // serializes requests to this connector
}

// NewManager creates a connector manager from config.
func NewManager(cfg *Config, logger *slog.Logger) *Manager {
	return &Manager{
		cfg:    cfg,
		logger: logger,
		procs:  make(map[string]*connectorProc),
	}
}

// Start launches all configured connectors.
func (m *Manager) Start() error {
	for name, cc := range m.cfg.Connectors {
		if err := m.startConnector(name, cc.Exec); err != nil {
			m.Shutdown()
			return fmt.Errorf("start connector %q: %w", name, err)
		}
		m.logger.Info("connector started", "name", name, "exec", cc.Exec)
	}
	return nil
}

func (m *Manager) startConnector(name, execPath string) error {
	cmd := exec.Command(execPath)
	cmd.Stderr = &logWriter{logger: m.logger, connector: name}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	scanner := bufio.NewScanner(stdoutPipe)
	scanner.Buffer(make([]byte, m.cfg.Limits.RespMaxBytes), m.cfg.Limits.RespMaxBytes)

	proc := &connectorProc{
		name:   name,
		cmd:    cmd,
		stdin:  stdin,
		stdout: scanner,
	}

	m.mu.Lock()
	m.procs[name] = proc
	m.mu.Unlock()

	return nil
}

// Call sends a request to a connector and returns the response.
func (m *Manager) Call(ctx context.Context, connectorName string, req *Request) (*Response, error) {
	m.mu.RLock()
	proc, ok := m.procs[connectorName]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("connector %q not running", connectorName)
	}

	// Enforce request size limit.
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	if len(reqData) > m.cfg.Limits.ReqMaxBytes {
		return nil, fmt.Errorf("request exceeds %d byte limit (%d bytes)", m.cfg.Limits.ReqMaxBytes, len(reqData))
	}

	timeout := time.Duration(m.cfg.Limits.CallTimeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Serialize access to this connector's stdin/stdout.
	proc.mu.Lock()
	defer proc.mu.Unlock()

	// Write request.
	reqData = append(reqData, '\n')
	if _, err := proc.stdin.Write(reqData); err != nil {
		return nil, fmt.Errorf("write to connector %q: %w", connectorName, err)
	}

	// Read response with timeout.
	type scanResult struct {
		line []byte
		err  error
	}
	ch := make(chan scanResult, 1)
	go func() {
		if proc.stdout.Scan() {
			// Copy the bytes since scanner reuses the buffer.
			line := make([]byte, len(proc.stdout.Bytes()))
			copy(line, proc.stdout.Bytes())
			ch <- scanResult{line: line}
		} else {
			ch <- scanResult{err: proc.stdout.Err()}
		}
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("connector %q call timed out", connectorName)
	case result := <-ch:
		if result.err != nil {
			return nil, fmt.Errorf("read from connector %q: %w", connectorName, result.err)
		}
		if result.line == nil {
			return nil, fmt.Errorf("connector %q closed stdout", connectorName)
		}

		// Enforce response size limit.
		if len(result.line) > m.cfg.Limits.RespMaxBytes {
			return nil, fmt.Errorf("response from %q exceeds %d byte limit", connectorName, m.cfg.Limits.RespMaxBytes)
		}

		var resp Response
		if err := json.Unmarshal(result.line, &resp); err != nil {
			return nil, fmt.Errorf("invalid response from %q: %w", connectorName, err)
		}

		if err := ValidateResponse(&resp); err != nil {
			return nil, fmt.Errorf("invalid response from %q: %w", connectorName, err)
		}

		if resp.ID != req.ID {
			return nil, fmt.Errorf("response id mismatch from %q: got %q, want %q", connectorName, resp.ID, req.ID)
		}

		return &resp, nil
	}
}

// StopConnector stops a single connector by name.
func (m *Manager) StopConnector(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	proc, ok := m.procs[name]
	if !ok {
		return fmt.Errorf("connector %q not running", name)
	}

	proc.stdin.Close()
	if err := proc.cmd.Process.Kill(); err != nil {
		m.logger.Warn("failed to kill connector", "name", name, "error", err)
	}
	proc.cmd.Wait()
	delete(m.procs, name)
	m.logger.Info("connector stopped", "name", name)
	return nil
}

// StartConnector launches a single connector by name using the given exec path.
func (m *Manager) StartConnector(name, execPath string) error {
	return m.startConnector(name, execPath)
}

// Shutdown stops all connector processes.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, proc := range m.procs {
		proc.stdin.Close()
		if err := proc.cmd.Process.Kill(); err != nil {
			m.logger.Warn("failed to kill connector", "name", name, "error", err)
		}
		proc.cmd.Wait()
		m.logger.Info("connector stopped", "name", name)
	}
	m.procs = make(map[string]*connectorProc)
}

// logWriter adapts connector stderr to slog.
type logWriter struct {
	logger    *slog.Logger
	connector string
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.logger.Debug("connector stderr", "connector", w.connector, "output", string(p))
	return len(p), nil
}
