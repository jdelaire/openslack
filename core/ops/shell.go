package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ShellOp is a generic shell command loaded from config.
type ShellOp struct {
	CmdName string `json:"name"`
	Desc    string `json:"description"`
	Command string `json:"command"`
	WorkDir string `json:"workdir"`
}

func (s *ShellOp) Name() string        { return s.CmdName }
func (s *ShellOp) Description() string  { return s.Desc }

func (s *ShellOp) Execute(ctx context.Context, args string) (string, error) {
	command := s.Command
	if args != "" {
		command = s.Command + " " + args
	}
	cmd := exec.CommandContext(ctx, "bash", "-l", "-c", command)
	if s.WorkDir != "" {
		cmd.Dir = s.WorkDir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w\n%s", s.CmdName, err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// LoadCommands reads a JSON config file and returns ShellOps.
// Returns nil, nil if the file does not exist.
func LoadCommands(path string) ([]ShellOp, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read commands config: %w", err)
	}

	var cmds []ShellOp
	if err := json.Unmarshal(data, &cmds); err != nil {
		return nil, fmt.Errorf("parse commands config: %w", err)
	}

	for i, c := range cmds {
		if c.CmdName == "" {
			return nil, fmt.Errorf("command at index %d missing name", i)
		}
		if c.Command == "" {
			return nil, fmt.Errorf("command %q missing command field", c.CmdName)
		}
	}

	return cmds, nil
}
