package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/store"
	"github.com/launchpad/launchpad/internal/target"
	"github.com/launchpad/launchpad/internal/target/stub"
)

func TestWorkerDeployJob(t *testing.T) {
	ctx := context.Background()
	db, driver, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(ctx, db, driver); err != nil {
		t.Fatal(err)
	}
	st := store.New(db, driver)

	workspaceID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	project := &domain.Project{WorkspaceID: workspaceID, Name: "deploy-app"}
	env := &domain.Environment{TargetType: "stub", TargetConfig: json.RawMessage(`{"namespace":"default"}`)}
	if err := st.CreateProject(ctx, project, env); err != nil {
		t.Fatal(err)
	}

	svc, err := st.GetServiceByProjectAndName(ctx, project.ID, project.PrimaryService)
	if err != nil {
		t.Fatal(err)
	}
	devEnv, err := st.GetEnvironmentByProjectAndName(ctx, project.ID, "dev")
	if err != nil {
		t.Fatal(err)
	}

	err = st.Transact(ctx, func(tx *sql.Tx) error {
		release := &domain.Release{
			ServiceID:       svc.ID,
			Version:         1,
			ArtifactRef:     "deploy-app:v1",
			ConfigResolved:  map[string]string{"PORT": "8080"},
			ProcessSnapshot: map[string]domain.ProcessSnapshot{"web": {Quantity: 1, Expose: "http"}},
			Status:          domain.ReleaseStatusPending,
		}
		if err := st.CreateRelease(ctx, tx, release); err != nil {
			return err
		}
		deployment := &domain.Deployment{
			ServiceID:     svc.ID,
			EnvironmentID: devEnv.ID,
			ReleaseID:     release.ID,
		}
		if err := st.CreateDeployment(ctx, tx, deployment); err != nil {
			return err
		}
		payload, _ := json.Marshal(domain.DeployPayload{
			DeploymentID:  deployment.ID,
			ServiceID:     svc.ID,
			EnvironmentID: devEnv.ID,
			ReleaseID:     release.ID,
		})
		return st.EnqueueJob(ctx, tx, &domain.Job{
			Type:         domain.JobTypeDeploy,
			ResourceType: "deployment",
			ResourceID:   deployment.ID,
			Payload:      payload,
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	registry := target.NewRegistry()
	registry.Register(stub.New())
	worker := NewWorker(st, registry, "test", slog.New(slog.NewTextHandler(os.Stdout, nil)))

	job, err := st.LeaseNext(ctx, "test", []domain.JobType{domain.JobTypeDeploy}, time.Minute)
	if err != nil || job == nil {
		t.Fatal("expected deploy job")
	}
	if err := worker.handleDeploy(ctx, job); err != nil {
		t.Fatal(err)
	}

	updated, err := st.GetDeployment(ctx, job.ResourceID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != domain.DeploymentRunning {
		t.Fatalf("expected deployment running, got %s", updated.Status)
	}

	updatedProject, err := st.GetProjectByID(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedProject.Status != domain.ProjectStatusRunning {
		t.Fatalf("expected project running, got %s", updatedProject.Status)
	}
}

func TestWorkerUsesReleaseSnapshotNotLiveConfig(t *testing.T) {
	ctx := context.Background()
	db, driver, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(ctx, db, driver); err != nil {
		t.Fatal(err)
	}
	st := store.New(db, driver)

	workspaceID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	project := &domain.Project{WorkspaceID: workspaceID, Name: "snap-deploy"}
	if err := st.CreateProject(ctx, project, &domain.Environment{TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}
	svc, err := st.GetServiceByProjectAndName(ctx, project.ID, project.PrimaryService)
	if err != nil {
		t.Fatal(err)
	}
	devEnv, err := st.GetEnvironmentByProjectAndName(ctx, project.ID, "dev")
	if err != nil {
		t.Fatal(err)
	}

	var jobID uuid.UUID
	err = st.Transact(ctx, func(tx *sql.Tx) error {
		release := &domain.Release{
			ServiceID:       svc.ID,
			Version:         1,
			ArtifactRef:     "snap:v1",
			ConfigResolved:  map[string]string{"PORT": "1111"},
			ProcessSnapshot: map[string]domain.ProcessSnapshot{"web": {Command: "serve", Quantity: 2, Expose: "http"}},
			Status:          domain.ReleaseStatusPending,
		}
		if err := st.CreateRelease(ctx, tx, release); err != nil {
			return err
		}
		deployment := &domain.Deployment{
			ServiceID: svc.ID, EnvironmentID: devEnv.ID, ReleaseID: release.ID,
		}
		if err := st.CreateDeployment(ctx, tx, deployment); err != nil {
			return err
		}
		payload, _ := json.Marshal(domain.DeployPayload{
			DeploymentID: deployment.ID, ServiceID: svc.ID, EnvironmentID: devEnv.ID, ReleaseID: release.ID,
		})
		job := &domain.Job{Type: domain.JobTypeDeploy, ResourceType: "deployment", ResourceID: deployment.ID, Payload: payload}
		if err := st.EnqueueJob(ctx, tx, job); err != nil {
			return err
		}
		jobID = job.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Mutate live config after enqueue — worker must ignore this.
	live := "9999"
	if err := st.MergeConfigVars(ctx, svc.ID, devEnv.ID, map[string]*string{"PORT": &live}); err != nil {
		t.Fatal(err)
	}

	job, err := st.GetJob(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(st, target.NewRegistry(), "test", slog.New(slog.NewTextHandler(os.Stdout, nil)))
	// Registry empty — only need loadDeployContext
	dc, err := worker.loadDeployContext(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	if dc.Config["PORT"] != "1111" {
		t.Fatalf("expected snapshot config PORT=1111, got %+v", dc.Config)
	}
	if len(dc.Processes) != 1 || dc.Processes[0].Command != "serve" || dc.Processes[0].Quantity != 2 {
		t.Fatalf("expected process from snapshot, got %+v", dc.Processes)
	}
}

func TestWorkerSupersedesPreviousRunning(t *testing.T) {
	ctx := context.Background()
	db, driver, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(ctx, db, driver); err != nil {
		t.Fatal(err)
	}
	st := store.New(db, driver)

	workspaceID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	project := &domain.Project{WorkspaceID: workspaceID, Name: "super-app"}
	if err := st.CreateProject(ctx, project, &domain.Environment{TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}
	svc, err := st.GetServiceByProjectAndName(ctx, project.ID, project.PrimaryService)
	if err != nil {
		t.Fatal(err)
	}
	devEnv, err := st.GetEnvironmentByProjectAndName(ctx, project.ID, "dev")
	if err != nil {
		t.Fatal(err)
	}

	var firstID, secondID uuid.UUID
	err = st.Transact(ctx, func(tx *sql.Tx) error {
		r1 := &domain.Release{
			ServiceID: svc.ID, Version: 1, ArtifactRef: "a:1",
			ConfigResolved: map[string]string{}, ProcessSnapshot: map[string]domain.ProcessSnapshot{"web": {Quantity: 1, Expose: "http"}},
			Status: domain.ReleaseStatusSucceeded,
		}
		if err := st.CreateRelease(ctx, tx, r1); err != nil {
			return err
		}
		d1 := &domain.Deployment{ServiceID: svc.ID, EnvironmentID: devEnv.ID, ReleaseID: r1.ID, Status: domain.DeploymentPending}
		if err := st.CreateDeployment(ctx, tx, d1); err != nil {
			return err
		}
		firstID = d1.ID
		if err := st.UpdateDeploymentStatus(ctx, tx, d1.ID, domain.DeploymentPending, domain.DeploymentDeploying, ""); err != nil {
			return err
		}
		return st.UpdateDeploymentStatus(ctx, tx, d1.ID, domain.DeploymentDeploying, domain.DeploymentRunning, "ok")
	})
	if err != nil {
		t.Fatal(err)
	}

	err = st.Transact(ctx, func(tx *sql.Tx) error {
		r2 := &domain.Release{
			ServiceID: svc.ID, Version: 2, ArtifactRef: "a:2",
			ConfigResolved: map[string]string{}, ProcessSnapshot: map[string]domain.ProcessSnapshot{"web": {Quantity: 1, Expose: "http"}},
		}
		if err := st.CreateRelease(ctx, tx, r2); err != nil {
			return err
		}
		d2 := &domain.Deployment{ServiceID: svc.ID, EnvironmentID: devEnv.ID, ReleaseID: r2.ID}
		if err := st.CreateDeployment(ctx, tx, d2); err != nil {
			return err
		}
		secondID = d2.ID
		payload, _ := json.Marshal(domain.DeployPayload{
			DeploymentID: d2.ID, ServiceID: svc.ID, EnvironmentID: devEnv.ID, ReleaseID: r2.ID,
		})
		return st.EnqueueJob(ctx, tx, &domain.Job{
			Type: domain.JobTypeDeploy, ResourceType: "deployment", ResourceID: d2.ID, Payload: payload,
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	registry := target.NewRegistry()
	registry.Register(stub.New())
	worker := NewWorker(st, registry, "test", slog.New(slog.NewTextHandler(os.Stdout, nil)))
	job, err := st.LeaseNext(ctx, "test", []domain.JobType{domain.JobTypeDeploy}, time.Minute)
	if err != nil || job == nil {
		t.Fatal("expected job")
	}
	if err := worker.handleDeploy(ctx, job); err != nil {
		t.Fatal(err)
	}

	first, err := st.GetDeployment(ctx, firstID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != domain.DeploymentSuperseded {
		t.Fatalf("expected first superseded, got %s", first.Status)
	}
	second, err := st.GetDeployment(ctx, secondID)
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != domain.DeploymentRunning {
		t.Fatalf("expected second running, got %s", second.Status)
	}
}
