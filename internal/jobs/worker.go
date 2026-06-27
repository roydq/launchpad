package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

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
	if err := w.handleDeploy(ctx, job); err != nil {
		w.logger.Error("job failed", "job_id", job.ID, "error", err)
		_ = w.store.CompleteJob(ctx, job.ID, domain.JobStatusFailed, err.Error())
		return err
	}
	return w.store.CompleteJob(ctx, job.ID, domain.JobStatusSucceeded, "")
}

func (w *Worker) handleDeploy(ctx context.Context, job *domain.Job) error {
	var payload domain.DeployPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}

	deployment, err := w.store.GetDeployment(ctx, payload.DeploymentID)
	if err != nil {
		return err
	}
	app, err := w.store.GetAppByID(ctx, payload.AppID)
	if err != nil {
		return err
	}

	targetBackend, err := w.registry.Get(app.TargetType)
	if err != nil {
		return fmt.Errorf("target %q: %w", app.TargetType, err)
	}

	release, err := w.getReleaseForDeployment(ctx, deployment)
	if err != nil {
		return err
	}
	processes, err := w.store.ListProcessTypes(ctx, app.ID)
	if err != nil {
		return err
	}
	config, err := w.store.ListConfigVars(ctx, app.ID)
	if err != nil {
		return err
	}

	if err := w.store.Transact(ctx, func(tx *sql.Tx) error {
		return w.store.UpdateDeploymentStatus(ctx, tx, deployment.ID, domain.DeploymentPending, domain.DeploymentDeploying, "deploying to target")
	}); err != nil {
		return err
	}

	result, deployErr := targetBackend.Deploy(ctx, target.DeployRequest{
		App: *app, Release: *release, Processes: processes, Config: config, ImageRef: release.ImageRef,
	})

	if deployErr != nil {
		return w.markDeployFailed(ctx, deployment, release, app, deployErr)
	}

	return w.store.Transact(ctx, func(tx *sql.Tx) error {
		if err := w.store.UpdateDeploymentStatus(ctx, tx, deployment.ID, domain.DeploymentDeploying, domain.DeploymentRunning, result.TargetRef); err != nil {
			return err
		}
		if err := w.store.UpdateReleaseStatus(ctx, tx, release.ID, domain.ReleaseStatusSucceeded); err != nil {
			return err
		}
		return w.store.ClearActiveDeployment(ctx, tx, app.ID, domain.AppStatusRunning)
	})
}

func (w *Worker) markDeployFailed(ctx context.Context, deployment *domain.Deployment, release *domain.Release, app *domain.App, deployErr error) error {
	return w.store.Transact(ctx, func(tx *sql.Tx) error {
		status := domain.DeploymentDeploying
		if deployment.Status == domain.DeploymentPending {
			status = domain.DeploymentPending
		}
		if err := w.store.UpdateDeploymentStatus(ctx, tx, deployment.ID, status, domain.DeploymentFailed, deployErr.Error()); err != nil {
			if err == launchpad.ErrConflict {
				_ = w.store.UpdateDeploymentStatus(ctx, tx, deployment.ID, domain.DeploymentPending, domain.DeploymentFailed, deployErr.Error())
			} else {
				return err
			}
		}
		if err := w.store.UpdateReleaseStatus(ctx, tx, release.ID, domain.ReleaseStatusFailed); err != nil {
			return err
		}
		return w.store.ClearActiveDeployment(ctx, tx, app.ID, domain.AppStatusFailed)
	})
}

func (w *Worker) getReleaseForDeployment(ctx context.Context, deployment *domain.Deployment) (*domain.Release, error) {
	releases, err := w.store.ListReleases(ctx, deployment.AppID)
	if err != nil {
		return nil, err
	}
	for i := range releases {
		if releases[i].ID == deployment.ReleaseID {
			return &releases[i], nil
		}
	}
	return nil, launchpad.ErrNotFound
}