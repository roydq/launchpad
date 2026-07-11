package cli

import (
	"context"
	"fmt"

	"github.com/launchpad/launchpad/pkg/apiclient"
)

func runDoctor(ctx context.Context, client *apiclient.Client, cfg Config) error {
	ok := true
	check := func(name string, err error) {
		if err != nil {
			fmt.Printf("FAIL  %s: %v\n", name, err)
			ok = false
			return
		}
		fmt.Printf("ok    %s\n", name)
	}

	check("api healthz", client.Healthz(ctx))
	if cfg.Token == "" {
		check("token", fmt.Errorf("LAUNCHPAD_TOKEN not set"))
	} else {
		check("token", nil)
	}

	if cfg.Token != "" {
		_, err := client.ListProjects(ctx)
		check("list projects", err)
	}

	project := cfg.Project
	if project == "" {
		fmt.Println("skip  project context (not set — run launchpad use or set .launchpad/config)")
	} else {
		fmt.Printf("ok    project context: %s\n", project)
		env := effectiveEnv(cfg)
		fmt.Printf("ok    environment context: %s\n", env)
		if cfg.Token != "" {
			_, err := client.GetProject(ctx, project)
			check("get project", err)
			_, err = client.GetEnvironment(ctx, project, env)
			check("get environment", err)
		}
	}

	if !ok {
		return fmt.Errorf("doctor found problems")
	}
	fmt.Println("doctor: all checks passed")
	return nil
}
