package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/launchpad/launchpad/pkg/apiclient"
)

func parseKEYVALArgs(args []string) ([]map[string]any, error) {
	var changes []map[string]any
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("expected KEY=VALUE, got %q", arg)
		}
		changes = append(changes, map[string]any{
			"type":  "config",
			"key":   parts[0],
			"value": parts[1],
		})
	}
	return changes, nil
}

func configUnsetChange(key string) map[string]any {
	return map[string]any{
		"type":  "config",
		"key":   key,
		"value": nil,
	}
}

func parseScaleArg(scale string) (map[string]any, error) {
	parts := strings.SplitN(scale, "=", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("expected PROC=N, got %q", scale)
	}
	qty, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid quantity in %q: %w", scale, err)
	}
	return map[string]any{
		"type":     "scale",
		"process":  parts[0],
		"quantity": qty,
	}, nil
}

func imageChange(ref string) map[string]any {
	return map[string]any{
		"type":  "image",
		"image": ref,
	}
}

func loadPending(ctx context.Context, client *apiclient.Client, project string) (*apiclient.Changeset, error) {
	return client.GetChangeset(ctx, project)
}

func pendingCount(cs *apiclient.Changeset) int {
	if cs == nil {
		return 0
	}
	return len(cs.Changes)
}

func requireCleanStaging(ctx context.Context, client *apiclient.Client, project string) error {
	cs, err := loadPending(ctx, client, project)
	if err != nil {
		return err
	}
	n := pendingCount(cs)
	if n == 0 {
		return nil
	}
	return fmt.Errorf("staging has %d pending change(s); run \"launchpad diff\", \"launchpad deploy\", or \"launchpad reset\" before using --now", n)
}

func stage(ctx context.Context, client *apiclient.Client, project string, changes []map[string]any) (*apiclient.Changeset, error) {
	if len(changes) == 0 {
		return nil, fmt.Errorf("no changes to stage")
	}
	return client.StageChanges(ctx, project, changes)
}

func push(ctx context.Context, client *apiclient.Client, project, message string) (*apiclient.DeployResult, error) {
	return client.PushChangeset(ctx, project, message)
}

func printDeployResult(result *apiclient.DeployResult) {
	if result == nil {
		return
	}
	fmt.Printf("deployment queued: release v%d deployment=%s job=%s status=%s\n",
		result.Deployment.Release.Version,
		result.Deployment.ID,
		result.Job.ID,
		result.Deployment.Status,
	)
}

func maybeWaitForDeploy(ctx context.Context, client *apiclient.Client, result *apiclient.DeployResult, wait bool, timeout time.Duration) error {
	printDeployResult(result)
	if !wait || result == nil {
		return nil
	}
	return waitForJob(ctx, client, result.Job.ID, timeout, 500*time.Millisecond)
}

// latestReleaseForEnv returns the release for the latest meaningful deploy in env.
// Prefer a running deployment; else highest-version release that has any deployment in env.
func latestReleaseForEnv(ctx context.Context, client *apiclient.Client, project, env string) (*apiclient.Release, error) {
	releases, err := client.ListReleases(ctx, project)
	if err != nil {
		return nil, err
	}
	for i := range releases {
		for _, d := range releases[i].Deployments {
			if d.Environment == env && d.Status == "running" {
				return &releases[i], nil
			}
		}
	}
	for i := range releases {
		for _, d := range releases[i].Deployments {
			if d.Environment == env {
				return &releases[i], nil
			}
		}
	}
	return nil, nil
}

func stageAndMaybeNow(
	ctx context.Context,
	client *apiclient.Client,
	project string,
	changes []map[string]any,
	now bool,
	message string,
	stagedMsg string,
	wait bool,
	timeout time.Duration,
) error {
	if now {
		if err := requireCleanStaging(ctx, client, project); err != nil {
			return err
		}
	}
	if _, err := stage(ctx, client, project, changes); err != nil {
		return err
	}
	if !now {
		fmt.Println(stagedMsg)
		return nil
	}
	result, err := push(ctx, client, project, message)
	if err != nil {
		return err
	}
	return maybeWaitForDeploy(ctx, client, result, wait, timeout)
}

func configKeysSummary(changes []map[string]any) string {
	var keys []string
	for _, c := range changes {
		if k, ok := c["key"].(string); ok {
			keys = append(keys, k)
		}
	}
	return strings.Join(keys, ", ")
}
