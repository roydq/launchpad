package domain

import (
	"fmt"
	"strings"
)

// ParsedProcfileEntry is one process line from a Procfile.
type ParsedProcfileEntry struct {
	Name     string
	Command  string
	Quantity int
	Expose   string
}

// ParseProcfile parses classic Heroku-style "name: command" lines.
// Blank lines and # comments are ignored. Defaults:
//   - web → expose=http, quantity=1
//   - release → expose=none, quantity=0 (stored, not deployed)
//   - other → expose=none, quantity=1
func ParseProcfile(text string) ([]ParsedProcfileEntry, error) {
	var out []ParsedProcfileEntry
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, cmd, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("procfile line %d: expected name: command", i+1)
		}
		name = strings.TrimSpace(name)
		cmd = strings.TrimSpace(cmd)
		if name == "" {
			return nil, fmt.Errorf("procfile line %d: empty process name", i+1)
		}
		if cmd == "" {
			return nil, fmt.Errorf("procfile line %d: empty command for %q", i+1, name)
		}
		e := ParsedProcfileEntry{Name: name, Command: cmd, Quantity: 1, Expose: "none"}
		switch name {
		case "web":
			e.Expose = "http"
		case "release":
			e.Quantity = 0
		}
		out = append(out, e)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("procfile has no process entries")
	}
	return out, nil
}
