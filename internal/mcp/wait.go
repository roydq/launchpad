package mcp

import (
	"context"
	"time"

	"github.com/launchpad/launchpad/pkg/apiclient"
)

func waitEnabled(wait *bool) bool {
	if wait == nil {
		return true
	}
	return *wait
}

func waitForJob(ctx context.Context, cl *apiclient.Client, jobID string, timeoutSec int) (*apiclient.Job, error) {
	if jobID == "" {
		return nil, errJSON("no job id to wait on", nil)
	}
	if timeoutSec <= 0 {
		timeoutSec = 300
	}
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	var last *apiclient.Job
	for {
		if err := ctx.Err(); err != nil {
			return last, wrapErr(err)
		}
		job, err := cl.GetJob(ctx, jobID)
		if err != nil {
			return last, wrapErr(err)
		}
		last = job
		switch job.Status {
		case "succeeded":
			return job, nil
		case "failed", "dead":
			return job, errJSON("deploy failed", map[string]any{
				"job_id":     job.ID,
				"status":     job.Status,
				"last_error": job.LastError,
			})
		}
		if !time.Now().Before(deadline) {
			return job, errJSON("timed out waiting for job", map[string]any{
				"job_id":      job.ID,
				"last_status": job.Status,
			})
		}
		select {
		case <-ctx.Done():
			return job, wrapErr(ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func deployResultOut(result *apiclient.DeployResult, job *apiclient.Job, waited bool) map[string]any {
	out := map[string]any{}
	if result != nil {
		out["deployment"] = result.Deployment
		out["job"] = result.Job
	}
	if waited && job != nil {
		out["job"] = job
		out["wait"] = map[string]any{"status": job.Status}
	}
	return out
}
