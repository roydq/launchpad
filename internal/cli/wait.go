package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/launchpad/launchpad/pkg/apiclient"
)

// terminalJob reports whether a job status is finished and if it succeeded.
func terminalJob(status string) (done bool, ok bool) {
	switch status {
	case "succeeded":
		return true, true
	case "failed", "dead":
		return true, false
	default:
		return false, false
	}
}

// waitForJob polls until the job reaches a terminal status or timeout.
func waitForJob(ctx context.Context, client *apiclient.Client, jobID string, timeout time.Duration, interval time.Duration) error {
	if jobID == "" {
		return fmt.Errorf("no job id to wait on")
	}
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	deadline := time.Now().Add(timeout)
	var last string
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for job %s (last status=%s)", timeout, jobID, last)
		}
		job, err := client.GetJob(ctx, jobID)
		if err != nil {
			return err
		}
		last = job.Status
		done, ok := terminalJob(job.Status)
		if job.Status != "" {
			line := fmt.Sprintf("job %s status=%s", jobID, job.Status)
			if job.LastError != "" {
				line += " error=" + job.LastError
			}
			fmt.Println(line)
		}
		if done {
			if ok {
				fmt.Println("deploy succeeded")
				return nil
			}
			errMsg := job.LastError
			if errMsg == "" {
				errMsg = job.Status
			}
			return fmt.Errorf("deploy failed: %s", errMsg)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}
