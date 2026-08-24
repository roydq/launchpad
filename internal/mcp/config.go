package mcp

import (
	"fmt"
	"strings"

	"github.com/launchpad/launchpad/pkg/apiclient"
)

// Config is the MCP process configuration (same sources as the CLI).
type Config struct {
	APIURL      string
	Token       string
	Project     string
	Environment string
}

func (c Config) apiURL() string {
	if c.APIURL != "" {
		return c.APIURL
	}
	return "http://localhost:8080"
}

// ResolveProject returns the explicit tool arg or configured default.
func (c Config) ResolveProject(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if c.Project != "" {
		return c.Project, nil
	}
	return "", fmt.Errorf(`{"detail":"project is required; pass project or set LAUNCHPAD_PROJECT"}`)
}

// ResolveEnv returns the explicit tool arg, configured env, or "dev".
func (c Config) ResolveEnv(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if c.Environment != "" {
		return c.Environment
	}
	return "dev"
}

// RequireToken fails when LAUNCHPAD_TOKEN is empty.
func (c Config) RequireToken() error {
	if c.Token == "" {
		return errMissingToken()
	}
	return nil
}

// IsSensitiveEnv reports whether env requires confirm on deploy/rollback/promote.
func IsSensitiveEnv(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "prod", "production":
		return true
	default:
		return false
	}
}

// Client returns a new HTTP client with the given ambient environment header.
func (c Config) Client(env string) *apiclient.Client {
	cl := apiclient.New(c.apiURL(), c.Token)
	cl.Environment = env
	return cl
}
