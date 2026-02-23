package connector

import (
	"testing"
)

func TestSplitTool(t *testing.T) {
	tests := []struct {
		input   string
		conn    string
		tool    string
		wantErr bool
	}{
		{"sample.echo", "sample", "echo", false},
		{"reminders.add", "reminders", "add", false},
		{"sample.__introspect", "sample", "__introspect", false},
		{"nodot", "", "", true},
		{".notool", "", "", true},
		{"noprefix.", "", "", true},
		{"", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			conn, tool, err := splitTool(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("splitTool(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if conn != tt.conn {
				t.Errorf("connector = %q, want %q", conn, tt.conn)
			}
			if tool != tt.tool {
				t.Errorf("tool = %q, want %q", tool, tt.tool)
			}
		})
	}
}
