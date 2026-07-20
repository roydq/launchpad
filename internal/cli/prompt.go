package cli

import (
	"fmt"
	"strings"
)

// FormatPrompt returns a prompt fragment from resolved config.
// Empty project → empty string (callers print nothing).
func FormatPrompt(cfg Config, format string) string {
	project := strings.TrimSpace(cfg.Project)
	if project == "" {
		return ""
	}
	env := effectiveEnv(cfg)
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "short":
		return fmt.Sprintf("%s@%s", project, env)
	case "long":
		return fmt.Sprintf("project=%s env=%s", project, env)
	default:
		return fmt.Sprintf("%s@%s", project, env)
	}
}

// ShellInitScript returns an eval-able snippet for bash or zsh.
func ShellInitScript(shell string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(shell)) {
	case "bash", "zsh", "":
		// Same function body works for bash and zsh.
		return shellInitBashZsh(), nil
	default:
		return "", fmt.Errorf("unsupported shell %q (supported: bash, zsh)", shell)
	}
}

func shellInitBashZsh() string {
	return `# launchpad shell-init — project@env in PS1 when set
_launchpad_prompt() {
  local frag
  frag="$(command launchpad prompt 2>/dev/null)" || return 0
  if [ -n "$frag" ]; then
    printf ' (lp:%s)' "$frag"
  fi
}
# Prepend once if not already present
case "$PS1" in
  *'_launchpad_prompt'*) ;;
  *) PS1='$(_launchpad_prompt)'"$PS1" ;;
esac
`
}
