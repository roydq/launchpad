package main

import (
	"os"

	"github.com/launchpad/launchpad/internal/cli"
)

func main() {
	cli.MustRun(cli.Config{
		APIURL: envOr("LAUNCHPAD_API_URL", "http://localhost:8080"),
		Token:  os.Getenv("LAUNCHPAD_TOKEN"),
		Team:   envOr("LAUNCHPAD_TEAM", "default"),
		App:    os.Getenv("LAUNCHPAD_APP"),
	})
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}