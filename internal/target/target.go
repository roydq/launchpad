package target

import (
	"context"
	"fmt"
	"io"

	"github.com/launchpad/launchpad/internal/domain"
)

type DeployRequest struct {
	App       domain.App
	Release   domain.Release
	Processes []domain.ProcessType
	Config    map[string]string
	ImageRef  string
}

type DeployResult struct {
	TargetRef    string
	ProcessState map[string]ProcessState
}

type ProcessState struct {
	Desired int
	Ready   int
}

type ScaleRequest struct {
	App         domain.App
	ProcessName string
	Quantity    int
}

type DestroyRequest struct {
	App domain.App
}

type RollbackRequest struct {
	App       domain.App
	Release   domain.Release
	Processes []domain.ProcessType
	Config    map[string]string
}

type StatusRequest struct {
	App       domain.App
	Processes []domain.ProcessType
}

type LogsRequest struct {
	App         domain.App
	ProcessName string
}

type RuntimeStatus struct {
	Ready        bool
	ProcessState map[string]ProcessState
	Message      string
}

type Target interface {
	Type() string
	Deploy(ctx context.Context, req DeployRequest) (*DeployResult, error)
	Scale(ctx context.Context, req ScaleRequest) error
	Destroy(ctx context.Context, req DestroyRequest) error
	Rollback(ctx context.Context, req RollbackRequest) (*DeployResult, error)
	Status(ctx context.Context, req StatusRequest) (*RuntimeStatus, error)
	Logs(ctx context.Context, req LogsRequest) (io.ReadCloser, error)
}

type Registry struct {
	targets map[string]Target
}

func NewRegistry() *Registry {
	return &Registry{targets: make(map[string]Target)}
}

func (r *Registry) Register(t Target) {
	r.targets[t.Type()] = t
}

func (r *Registry) Get(typeName string) (Target, error) {
	t, ok := r.targets[typeName]
	if !ok {
		return nil, fmt.Errorf("unknown target: %s", typeName)
	}
	return t, nil
}