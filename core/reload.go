package core

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/jdelaire/openslack/core/connector"
	"github.com/jdelaire/openslack/core/ops"
)

// Reloader handles hot-reloading of dynamic ops (shell commands and connectors).
type Reloader struct {
	registry *ops.Registry
	connMgr  *connector.Manager
	logger   *slog.Logger

	mu           sync.Mutex
	shellOpNames []string
	connOpNames  []string
}

// NewReloader creates a reloader that tracks dynamic ops.
// connMgr may be nil if connectors are not yet configured.
func NewReloader(registry *ops.Registry, connMgr *connector.Manager, logger *slog.Logger) *Reloader {
	return &Reloader{
		registry: registry,
		connMgr:  connMgr,
		logger:   logger,
	}
}

// SetConnectorManager updates the connector manager reference.
// Used when connectors are first loaded during a reload.
func (r *Reloader) SetConnectorManager(mgr *connector.Manager) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connMgr = mgr
}

// TrackShellOps records names of shell ops loaded at startup so we know what to unregister.
func (r *Reloader) TrackShellOps(names []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shellOpNames = make([]string, len(names))
	copy(r.shellOpNames, names)
}

// TrackConnectorOps records names of connector ops loaded at startup.
func (r *Reloader) TrackConnectorOps(names []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connOpNames = make([]string, len(names))
	copy(r.connOpNames, names)
}

// ReloadCommands unregisters old shell ops, loads new ones from the config file,
// and registers them.
func (r *Reloader) ReloadCommands(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Unregister old shell ops.
	for _, name := range r.shellOpNames {
		r.registry.Unregister(name)
	}
	r.shellOpNames = nil

	// Load new commands.
	cmds, err := ops.LoadCommands(path)
	if err != nil {
		r.logger.Error("reload commands failed", "path", path, "error", err)
		return
	}

	var names []string
	for i := range cmds {
		if err := r.registry.Register(&cmds[i]); err != nil {
			r.logger.Warn("skip reloaded command", "name", cmds[i].Name(), "error", err)
			continue
		}
		names = append(names, cmds[i].Name())
		r.logger.Info("reloaded command", "name", cmds[i].Name())
	}
	r.shellOpNames = names
	r.logger.Info("commands reloaded", "count", len(names))
}

// ReloadConnectors stops old connectors, unregisters their ops, loads new config,
// starts new connectors, and registers new ops.
func (r *Reloader) ReloadConnectors(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Unregister old connector ops.
	for _, name := range r.connOpNames {
		r.registry.Unregister(name)
	}
	r.connOpNames = nil

	// Stop old connectors.
	if r.connMgr != nil {
		r.connMgr.Shutdown()
	}

	// Load new config.
	cfg, err := connector.LoadConfig(path)
	if err != nil {
		r.logger.Error("reload connectors failed", "path", path, "error", err)
		return
	}

	if cfg == nil || len(cfg.Connectors) == 0 {
		r.connMgr = nil
		r.logger.Info("connectors reloaded", "count", 0)
		return
	}

	// Start new connectors.
	mgr := connector.NewManager(cfg, r.logger)
	if err := mgr.Start(); err != nil {
		r.logger.Error("reload connectors: start failed", "error", err)
		return
	}
	r.connMgr = mgr

	// Register new connector ops.
	router := connector.NewRouter(cfg, mgr, r.logger)
	var names []string
	for connName, cc := range cfg.Connectors {
		for _, tool := range cc.Tools {
			qualified := connName + "." + tool
			op := &connector.ConnectorOp{
				QualifiedName: qualified,
				Desc:          fmt.Sprintf("Connector: %s", qualified),
				Router:        router,
			}
			if err := r.registry.Register(op); err != nil {
				r.logger.Warn("skip reloaded connector op", "name", qualified, "error", err)
				continue
			}
			names = append(names, qualified)
		}
	}
	r.connOpNames = names
	r.logger.Info("connectors reloaded", "count", len(cfg.Connectors))
}
