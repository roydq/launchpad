package cli

import (
	"strings"
	"testing"
)

func TestFormatPrompt(t *testing.T) {
	tests := []struct {
		name   string
		cfg    Config
		format string
		want   string
	}{
		{name: "empty project", cfg: Config{}, format: "short", want: ""},
		{name: "short default env", cfg: Config{Project: "my-api"}, format: "short", want: "my-api@dev"},
		{name: "short with env", cfg: Config{Project: "my-api", Environment: "staging"}, format: "short", want: "my-api@staging"},
		{name: "long", cfg: Config{Project: "p", Environment: "prod"}, format: "long", want: "project=p env=prod"},
		{name: "empty format is short", cfg: Config{Project: "x", Environment: "dev"}, format: "", want: "x@dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatPrompt(tt.cfg, tt.format)
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestShellInitScript(t *testing.T) {
	for _, sh := range []string{"bash", "zsh", ""} {
		s, err := ShellInitScript(sh)
		if err != nil {
			t.Fatalf("%s: %v", sh, err)
		}
		for _, part := range []string{"_launchpad_prompt", "launchpad prompt", "PS1"} {
			if !strings.Contains(s, part) {
				t.Fatalf("%s: missing %q in script", sh, part)
			}
		}
	}
	if _, err := ShellInitScript("fish"); err == nil {
		t.Fatal("expected fish error")
	}
}
