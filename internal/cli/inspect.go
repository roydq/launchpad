package cli

import (
	"context"
	"fmt"

	"github.com/launchpad/launchpad/pkg/apiclient"
)

func runInspect(ctx context.Context, client *apiclient.Client, cfg Config) error {
	project, err := requireProject(cfg)
	if err != nil {
		return err
	}
	envName := effectiveEnv(cfg)
	fmt.Printf("project: %s\nenvironment: %s\n", project, envName)

	if p, err := client.GetProject(ctx, project); err == nil {
		fmt.Printf("status: %s\nprimary_service: %s\n", p.Status, p.PrimaryService)
	}

	cs, err := loadPending(ctx, client, project)
	if err != nil {
		return err
	}
	n := pendingCount(cs)
	if n == 0 {
		fmt.Println("pending: none")
	} else {
		pin := cs.Environment
		if pin == "" {
			pin = envName
		}
		fmt.Printf("pending: %d change(s) (pinned=%s)\n", n, pin)
	}

	if rel, err := latestReleaseForEnv(ctx, client, project, envName); err == nil && rel != nil {
		fmt.Printf("last deploy: v%d %s (%s)\n", rel.Version, rel.ArtifactRef, rel.Status)
	} else {
		fmt.Println("last deploy: none")
	}

	procs, err := client.ListProcesses(ctx, project)
	if err != nil {
		return err
	}
	fmt.Println("processes:")
	for _, p := range procs {
		fmt.Printf("  %s  qty=%d expose=%s\n", p.Name, p.Quantity, p.Expose)
	}
	return nil
}
