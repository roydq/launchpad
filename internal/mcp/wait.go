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

func waitJobErr(detail string, job *apiclient.Job, cause error) error {
	extra := map[string]any{}
	if job != nil && job.ID != "" {
		extra["job_id"] = job.ID
		if job.Status != "" {
			extra["last_status"] = job.Status
		}
	}
	if cause != nil {
		extra["cause"] = cause.Error()
	}
	return errJSON(detail, extra)
}

func waitForJob(ctx context.Context, cl *apiclient.Client, last *apiclient.Job, timeoutSec int) (*apiclient.Job, error) {
	if last == nil || last.ID == "" {
		return nil, errJSON("no job id to wait on", nil)
	}
	if timeoutSec <= 0 {
		timeoutSec = 300
	}
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	jobID := last.ID
	for {
		if err := ctx.Err(); err != nil {
			return last, waitJobErr("wait canceled", last, err)
		}
		job, err := cl.GetJob(ctx, jobID)
		if err != nil {
			return last, waitJobErr("get job failed", last, err)
		}
		last = job
		switch job.Status {
		case "succeeded":
			return job, nil
		case "failed", "dead":
			return job, errJSON("deploy failed", map[string]any{
				"job_id":      job.ID,
				"status":      job.Status,
				"last_status": job.Status,
				"last_error":  job.LastError,
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
			return job, waitJobErr("wait canceled", job, ctx.Err())
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
