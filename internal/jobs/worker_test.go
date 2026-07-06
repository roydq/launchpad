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
			ConfigResolved:  map[string]string{},
			ProcessSnapshot: map[string]domain.ProcessSnapshot{"web": {Quantity: 1}},
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