package main

import (
	"github.com/launchpad/launchpad/internal/cli"
)

func main() {
	cli.MustRun(cli.LoadConfig())
}