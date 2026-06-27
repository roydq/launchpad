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
	job, err := w.store.LeaseNext(ctx, w.workerID, []domain.JobType{
		domain.JobTypeDeploy,
		domain.JobTypeScale,
		domain.JobTypeRollback,
	}, w.lease)
	if err != nil || job == nil {
		return err
	}

	w.logger.Info("leased job", "job_id", job.ID, "type", job.Type)
	var handleErr error
	switch job.Type {
	case domain.JobTypeDeploy:
		handleErr = w.handleDeploy(ctx, job)
	case domain.JobTypeScale:
		handleErr = w.handleScale(ctx, job)
	case domain.JobTypeRollback:
		handleErr = w.handleRollback(ctx, job)
	default:
		handleErr = fmt.Errorf("unsupported job type: %s", job.Type)
	}

	if handleErr != nil {
		w.logger.Error("job failed", "job_id", job.ID, "error", handleErr)
		_ = w.store.CompleteJob(ctx, job.ID, domain.JobStatusFailed, handleErr.Error())
		return handleErr
	}
	return w.store.CompleteJob(ctx, job.ID, domain.JobStatusSucceeded, "")
}

func (w *Worker) handleDeploy(ctx context.Context, job *domain.Job) error {
	payload, deployment, app, release, err := w.loadDeployContext(ctx, job)
	if err != nil {
		return err
	}
	_ = payload

	targetBackend, err := w.registry.Get(app.TargetType)
	if err != nil {
		return fmt.Errorf("target %q: %w", app.TargetType, err)
	}

	processes, config, err := w.loadRuntimeState(ctx, app)
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
	return w.markDeploySucceeded(ctx, deployment, release, app, result.TargetRef)
}

func (w *Worker) handleRollback(ctx context.Context, job *domain.Job) error {
	var payload domain.RollbackPayload
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
	release, err := w.store.GetReleaseByID(ctx, deployment.ReleaseID)
	if err != nil {
		return err
	}

	targetBackend, err := w.registry.Get(app.TargetType)
	if err != nil {
		return fmt.Errorf("target %q: %w", app.TargetType, err)
	}

	processes, config, err := w.loadRuntimeState(ctx, app)
	if err != nil {
		return err
	}

	if err := w.store.Transact(ctx, func(tx *sql.Tx) error {
		return w.store.UpdateDeploymentStatus(ctx, tx, deployment.ID, domain.DeploymentPending, domain.DeploymentDeploying,
			fmt.Sprintf("rolling back to v%d", payload.TargetReleaseVersion))
	}); err != nil {
		return err
	}

	result, rollbackErr := targetBackend.Rollback(ctx, target.RollbackRequest{
		App: *app, Release: *release, Processes: processes, Config: config,
	})
	if rollbackErr != nil {
		return w.markDeployFailed(ctx, deployment, release, app, rollbackErr)
	}
	return w.markDeploySucceeded(ctx, deployment, release, app, result.TargetRef)
}

func (w *Worker) handleScale(ctx context.Context, job *domain.Job) error {
	var payload domain.ScalePayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
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

	pt, err := w.store.GetProcessType(ctx, app.ID, payload.ProcessName)
	if err != nil {
		return err
	}
	if pt.Quantity == payload.Quantity {
		w.logger.Info("scale skipped, already at desired quantity", "process", payload.ProcessName, "quantity", payload.Quantity)
		return nil
	}

	return targetBackend.Scale(ctx, target.ScaleRequest{
		App: *app, ProcessName: payload.ProcessName, Quantity: payload.Quantity,
	})
}

func (w *Worker) loadDeployContext(ctx context.Context, job *domain.Job) (domain.DeployPayload, *domain.Deployment, *domain.App, *domain.Release, error) {
	var payload domain.DeployPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return payload, nil, nil, nil, err
	}
	deployment, err := w.store.GetDeployment(ctx, payload.DeploymentID)
	if err != nil {
		return payload, nil, nil, nil, err
	}
	app, err := w.store.GetAppByID(ctx, payload.AppID)
	if err != nil {
		return payload, nil, nil, nil, err
	}
	release, err := w.store.GetReleaseByID(ctx, payload.ReleaseID)
	if err != nil {
		return payload, nil, nil, nil, err
	}
	return payload, deployment, app, release, nil
}

func (w *Worker) loadRuntimeState(ctx context.Context, app *domain.App) ([]domain.ProcessType, map[string]string, error) {
	processes, err := w.store.ListProcessTypes(ctx, app.ID)
	if err != nil {
		return nil, nil, err
	}
	config, err := w.store.ListConfigVars(ctx, app.ID)
	if err != nil {
		return nil, nil, err
	}
	return processes, config, nil
}

func (w *Worker) markDeploySucceeded(ctx context.Context, deployment *domain.Deployment, release *domain.Release, app *domain.App, targetRef string) error {
	return w.store.Transact(ctx, func(tx *sql.Tx) error {
		if err := w.store.UpdateDeploymentStatus(ctx, tx, deployment.ID, domain.DeploymentDeploying, domain.DeploymentRunning, targetRef); err != nil {
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

