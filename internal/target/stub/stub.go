package stub

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/launchpad/launchpad/internal/target"
)

// Target is a development backend that simulates successful deploys.
type Target struct{}

func New() *Target { return &Target{} }

func (t *Target) Type() string { return "stub" }

func (t *Target) Deploy(ctx context.Context, req target.DeployRequest) (*target.DeployResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(500 * time.Millisecond):
	}
	return &target.DeployResult{
		TargetRef: fmt.Sprintf("stub-%s-%s-v%d", req.Project.Name, req.Service.Name, req.Release.Version),
		ProcessState: map[string]target.ProcessState{
			"web": {Desired: 1, Ready: 1},
		},
	}, nil
}

func (t *Target) Scale(ctx context.Context, req target.ScaleRequest) error {
	return nil
}

func (t *Target) Destroy(ctx context.Context, req target.DestroyRequest) error {
	return nil
}

func (t *Target) Rollback(ctx context.Context, req target.RollbackRequest) (*target.DeployResult, error) {
	return t.Deploy(ctx, target.DeployRequest{
		Project:     req.Project,
		Service:     req.Service,
		Environment: req.Environment,
		Release:     req.Release,
		Processes:   req.Processes,
		Config:      req.Config,
	})
}

func (t *Target) Status(ctx context.Context, req target.StatusRequest) (*target.RuntimeStatus, error) {
	return &target.RuntimeStatus{Ready: true, Message: "stub target ready"}, nil
}

func (t *Target) Logs(ctx context.Context, req target.LogsRequest) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("stub log line\n")), nil
}