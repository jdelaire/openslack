package connector

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Default limits.
const (
	DefaultReqMaxBytes  = 4096
	DefaultRespMaxBytes = 16384
	DefaultCallTimeoutMs = 10000
)

// Config is the top-level connector configuration.
type Config struct {
	Connectors map[string]ConnectorConfig `json:"connectors"`
	Limits     LimitsConfig              `json:"limits"`
}

// ConnectorConfig defines a single connector's executable and allowed tools.
type ConnectorConfig struct {
	Exec  string   `json:"exec"`
	Tools []string `json:"tools"`
}

// LimitsConfig holds global resource limits.
type LimitsConfig struct {
	ReqMaxBytes   int `json:"req_max_bytes"`
	RespMaxBytes  int `json:"resp_max_bytes"`
	CallTimeoutMs int `json:"call_timeout_ms"`
}

// LoadConfig reads and validates a connector config file.
// Returns nil, nil if the file does not exist.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read connector config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse connector config: %w", err)
	}

	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}

	applyDefaults(&cfg)
	return &cfg, nil
}

func validateConfig(cfg *Config) error {
	for name, cc := range cfg.Connectors {
		if name == "" {
			return fmt.Errorf("connector name cannot be empty")
		}
		if strings.Contains(name, ".") {
			return fmt.Errorf("connector name %q must not contain dots", name)
		}
		if cc.Exec == "" {
			return fmt.Errorf("connector %q missing exec path", name)
		}
		if len(cc.Tools) == 0 {
			return fmt.Errorf("connector %q has no allowed tools", name)
		}
		for _, t := range cc.Tools {
			if t == "" {
				return fmt.Errorf("connector %q has empty tool name", name)
			}
			if strings.HasPrefix(t, "__") && t != IntrospectToolName {
				return fmt.Errorf("connector %q: tool %q uses reserved prefix __", name, t)
			}
		}
	}
	return nil
}

func applyDefaults(cfg *Config) {
	if cfg.Limits.ReqMaxBytes <= 0 {
		cfg.Limits.ReqMaxBytes = DefaultReqMaxBytes
	}
	if cfg.Limits.RespMaxBytes <= 0 {
		cfg.Limits.RespMaxBytes = DefaultRespMaxBytes
	}
	if cfg.Limits.CallTimeoutMs <= 0 {
		cfg.Limits.CallTimeoutMs = DefaultCallTimeoutMs
	}
}

// ToolAllowed returns true if the given tool is in the connector's allowlist.
// The __introspect tool is always allowed.
func (cc *ConnectorConfig) ToolAllowed(tool string) bool {
	if tool == IntrospectToolName {
		return true
	}
	for _, t := range cc.Tools {
		if t == tool {
			return true
		}
	}
	return false
}
