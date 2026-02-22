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
	"sync"
	"time"

	"github.com/google/uuid"
)

// Server listens on a Unix domain socket and dispatches requests.
type Server struct {
	socketPath string
	registry   *Registry
	listener   net.Listener
	wg         sync.WaitGroup
	logger     *slog.Logger
}

// NewServer creates a new socket server.
func NewServer(socketPath string, registry *Registry, logger *slog.Logger) *Server {
	return &Server{
		socketPath: socketPath,
		registry:   registry,
		logger:     logger,
	}
}

// Start begins listening. It cleans up stale sockets, creates the directory
// with 0700 permissions, and sets the socket to 0600.
func (s *Server) Start(ctx context.Context) error {
	dir := filepath.Dir(s.socketPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create socket directory: %w", err)
	}

	// Clean up stale socket.
	if _, err := os.Stat(s.socketPath); err == nil {
		// Check if something is listening.
		conn, err := net.DialTimeout("unix", s.socketPath, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return fmt.Errorf("another instance is already listening on %s", s.socketPath)
		}
		s.logger.Info("removing stale socket", "path", s.socketPath)
		if err := os.Remove(s.socketPath); err != nil {
			return fmt.Errorf("remove stale socket: %w", err)
		}
	}

	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	if err := os.Chmod(s.socketPath, 0600); err != nil {
		ln.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}

	s.listener = ln
	s.logger.Info("listening", "path", s.socketPath)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.acceptLoop(ctx)
	}()

	return nil
}

// Shutdown gracefully stops the server and waits for in-flight connections.
func (s *Server) Shutdown() {
	if s.listener != nil {
		s.listener.Close()
	}
	s.wg.Wait()
	os.Remove(s.socketPath)
}

func (s *Server) acceptLoop(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				if opErr, ok := err.(*net.OpError); ok && opErr.Err.Error() == "use of closed network connection" {
					return
				}
				s.logger.Error("accept error", "error", err)
				return
			}
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConnection(ctx, conn)
		}()
	}
}

func (s *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	data, err := io.ReadAll(io.LimitReader(conn, MaxPayloadBytes+1))
	if err != nil {
		s.writeResponse(conn, Response{OK: false, Error: "read error"})
		return
	}

	if len(data) > MaxPayloadBytes {
		s.writeResponse(conn, Response{OK: false, Error: fmt.Sprintf("payload exceeds %d byte limit", MaxPayloadBytes)})
		return
	}

	req, err := ValidateRequest(data)
	if err != nil {
		s.logger.Warn("invalid request", "error", err)
		s.writeResponse(conn, Response{OK: false, Error: err.Error()})
		return
	}

	switch req.Action {
	case "notify":
		s.handleNotify(ctx, conn, req)
	default:
		s.writeResponse(conn, Response{OK: false, Error: fmt.Sprintf("unknown action %q", req.Action)})
	}
}

func (s *Server) handleNotify(ctx context.Context, conn net.Conn, req *Request) {
	payload, err := ParseNotifyPayload(req.Payload)
	if err != nil {
		s.writeResponse(conn, Response{OK: false, Error: err.Error()})
		return
	}

	notifier, err := s.registry.Default()
	if err != nil {
		s.logger.Error("no default notifier", "error", err)
		s.writeResponse(conn, Response{OK: false, Error: "no notifier configured"})
		return
	}

	id := uuid.New().String()
	n := Notification{
		ID:        id,
		Text:      payload.Text,
		Source:    payload.Source,
		CreatedAt: time.Now(),
	}

	if err := notifier.Send(ctx, n); err != nil {
		s.logger.Error("send failed", "notifier", notifier.Name(), "error", err)
		s.writeResponse(conn, Response{OK: false, Error: "delivery failed"})
		return
	}

	s.logger.Info("notification sent", "id", id, "notifier", notifier.Name(), "source", payload.Source)
	s.writeResponse(conn, Response{OK: true, ID: id})
}

func (s *Server) writeResponse(conn net.Conn, resp Response) {
	json.NewEncoder(conn).Encode(resp)
}
