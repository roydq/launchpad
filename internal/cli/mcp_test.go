package cli

import (
	"strings"
	"testing"
)

func TestMCPCommandPresent(t *testing.T) {
	root := NewRoot(Config{})
	var mcp *cobraCommand
	for _, c := range root.Commands() {
		if c.Name() == "mcp" {
			mcp = &cobraCommand{use: c.Use, short: c.Short}
			if !strings.Contains(strings.ToLower(c.Short), "stdio") && !strings.Contains(c.Short, "MCP") {
				t.Fatalf("short %q should mention stdio or MCP", c.Short)
			}
			if !strings.Contains(strings.ToLower(c.Short), "stdio") {
				t.Fatalf("short %q should mention stdio", c.Short)
			}
			break
		}
	}
	if mcp == nil {
		t.Fatal("mcp command missing")
	}
	// Help must not start the server.
	root.SetArgs([]string{"mcp", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("mcp --help: %v", err)
	}
}

type cobraCommand struct {
	use, short string
}
