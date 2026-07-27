package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/launchpad/launchpad/pkg/apiclient"
)

func findReleaseByVersion(ctx context.Context, client *apiclient.Client, project string, version int) (*apiclient.Release, error) {
	releases, err := client.ListReleases(ctx, project)
	if err != nil {
		return nil, err
	}
	for i := range releases {
		if releases[i].Version == version {
			return &releases[i], nil
		}
	}
	return nil, fmt.Errorf("release v%d not found", version)
}

func listReleasesJSON(ctx context.Context, client *apiclient.Client, cfg Config) error {
	project, err := requireProject(cfg)
	if err != nil {
		return err
	}
	releases, err := client.ListReleases(ctx, project)
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(releases, "", "  ")
	fmt.Println(string(b))
	return nil
}
