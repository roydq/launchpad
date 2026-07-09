package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/store"
	"github.com/launchpad/launchpad/internal/target"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

type Worker struct {
	store    *store.Store
	registry *target.Registry
	workerID string
	lease    time.Duration
	logger   *slog.Logger
}

func NewWorker(s *store.Store, registry *target.Registry, workerID string, logger *slog.Logger) *Worker {
	return &Worker{
		store:    s,
		registry: registry,
		workerID: workerID,
		lease:    5 * time.Minute,
		logger:   logger,
	}
}

func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	reclaim := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	defer reclaim.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-reclaim.C:
			if _, err := w.store.ReclaimExpiredLeases(ctx); err != nil {
				w.logger.Error("reclaim leases", "error", err)
			}
		case <-ticker.C:
			if err := w.processOne(ctx); err != nil {
				w.logger.Error("process job", "error", err)
			}
		}
	}
}

func (w *Worker) processOne(ctx context.Context) error {
	job, err := w.store.LeaseNext(ctx, w.workerID, []domain.JobType{domain.JobTypeDeploy}, w.lease)
	if err != nil || job == nil {
		return err
	}

	w.logger.Info("leased job", "job_id", job.ID, "type", job.Type)
	handleErr := w.handleDeploy(ctx, job)

	if handleErr != nil {
		w.logger.Error("job failed", "job_id", job.ID, "error", handleErr)
		_ = w.store.CompleteJob(ctx, job.ID, domain.JobStatusFailed, handleErr.Error())
		return handleErr
	}
	return w.store.CompleteJob(ctx, job.ID, domain.JobStatusSucceeded, "")
}

func (w *Worker) handleDeploy(ctx context.Context, job *domain.Job) error {
	deployCtx, err := w.loadDeployContext(ctx, job)
	if err != nil {
		return err
	}

	targetBackend, err := w.registry.Get(deployCtx.Environment.TargetType)
	if err != nil {
		return fmt.Errorf("target %q: %w", deployCtx.Environment.TargetType, err)
	}

	if err := w.store.Transact(ctx, func(tx *sql.Tx) error {
		if err := w.store.UpdateDeploymentStatus(ctx, tx, deployCtx.Deployment.ID, domain.DeploymentPending, domain.DeploymentDeploying, "deploying to target"); err != nil {
			return err
		}
		return w.store.SupersedeRunningDeployments(ctx, tx, deployCtx.Service.ID, deployCtx.Environment.ID, deployCtx.Deployment.ID)
	}); err != nil {
		return err
	}
	deployCtx.Deployment.Status = domain.DeploymentDeploying

	result, deployErr := targetBackend.Deploy(ctx, target.DeployRequest{
		Project:     *deployCtx.Project,
		Service:     *deployCtx.Service,
		Environment: *deployCtx.Environment,
		Release:     *deployCtx.Release,
		Processes:   deployCtx.Processes,
		Config:      deployCtx.Config,
	})
	if deployErr != nil {
		return w.markDeployFailed(ctx, deployCtx, deployErr)
	}
	return w.markDeploySucceeded(ctx, deployCtx, result.TargetRef)
}

type deployContext struct {
	Payload     domain.DeployPayload
	Deployment  *domain.Deployment
	Project     *domain.Project
	Service     *domain.Service
	Environment *domain.Environment
	Release     *domain.Release
	Processes   []domain.Process
	Config      map[string]string
}

func (w *Worker) loadDeployContext(ctx context.Context, job *domain.Job) (*deployContext, error) {
	var payload domain.DeployPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return nil, err
	}

	deployment, err := w.store.GetDeployment(ctx, payload.DeploymentID)
	if err != nil {
		return nil, err
	}
	service, err := w.store.GetServiceByID(ctx, payload.ServiceID)
	if err != nil {
		return nil, err
	}
	environment, err := w.store.GetEnvironmentByID(ctx, payload.EnvironmentID)
	if err != nil {
		return nil, err
	}
	project, err := w.store.GetProjectByID(ctx, service.ProjectID)
	if err != nil {
		return nil, err
	}
	release, err := w.store.GetReleaseByID(ctx, payload.ReleaseID)
	if err != nil {
		return nil, err
	}

	// Desired state comes only from the release snapshot — never live tables.
	config := release.ConfigResolved
	if config == nil {
		config = map[string]string{}
	}
	processes := processesFromSnapshot(service.ID, release.ProcessSnapshot)

	return &deployContext{
		Payload:     payload,
		Deployment:  deployment,
		Project:     project,
		Service:     service,
		Environment: environment,
		Release:     release,
		Processes:   processes,
		Config:      config,
	}, nil
}

func processesFromSnapshot(serviceID uuid.UUID, snapshot map[string]domain.ProcessSnapshot) []domain.Process {
	if len(snapshot) == 0 {
		return []domain.Process{domain.ProcessFromSnapshot(serviceID, "web", domain.ProcessSnapshot{Quantity: 1, Expose: "http"})}
	}
	out := make([]domain.Process, 0, len(snapshot))
	for name, snap := range snapshot {
		out = append(out, domain.ProcessFromSnapshot(serviceID, name, snap))
	}
	return out
}

func (w *Worker) markDeploySucceeded(ctx context.Context, deployCtx *deployContext, targetRef string) error {
	return w.store.Transact(ctx, func(tx *sql.Tx) error {
		if err := w.store.UpdateDeploymentStatus(ctx, tx, deployCtx.Deployment.ID, domain.DeploymentDeploying, domain.DeploymentRunning, targetRef); err != nil {
			return err
		}
		if err := w.store.UpdateReleaseStatus(ctx, tx, deployCtx.Release.ID, domain.ReleaseStatusSucceeded); err != nil {
			return err
		}
		return w.store.UpdateProjectStatusTx(ctx, tx, deployCtx.Project.ID, domain.ProjectStatusRunning)
	})
}

func (w *Worker) markDeployFailed(ctx context.Context, deployCtx *deployContext, deployErr error) error {
	return w.store.Transact(ctx, func(tx *sql.Tx) error {
		from := deployCtx.Deployment.Status
		if from != domain.DeploymentPending && from != domain.DeploymentDeploying {
			from = domain.DeploymentDeploying
		}
		if err := w.store.UpdateDeploymentStatus(ctx, tx, deployCtx.Deployment.ID, from, domain.DeploymentFailed, deployErr.Error()); err != nil {
			if errors.Is(err, launchpad.ErrConflict) && from != domain.DeploymentPending {
				if err2 := w.store.UpdateDeploymentStatus(ctx, tx, deployCtx.Deployment.ID, domain.DeploymentPending, domain.DeploymentFailed, deployErr.Error()); err2 != nil {
					return err2
				}
			} else {
				return err
			}
		}
		if err := w.store.UpdateReleaseStatus(ctx, tx, deployCtx.Release.ID, domain.ReleaseStatusFailed); err != nil {
			return err
		}
		return w.store.UpdateProjectStatusTx(ctx, tx, deployCtx.Project.ID, domain.ProjectStatusFailed)
	})
}
