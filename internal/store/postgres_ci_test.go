package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
)

// LAUNCHPAD_TEST_DATABASE_URL gates Postgres integration tests (CI job test-postgres).
// Default unit tests use SQLite :memory: and skip this file's cases.
func postgresTestURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("LAUNCHPAD_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("LAUNCHPAD_TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	return url
}

func TestPostgresMigrateBootstrapLease(t *testing.T) {
	ctx := context.Background()
	url := postgresTestURL(t)

	db, driver, err := Open(ctx, url)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer db.Close()
	if driver != DriverPostgres {
		t.Fatalf("expected postgres driver, got %s", driver)
	}

	// Isolate concurrent CI runs / re-runs.
	suffix := uuid.New().String()[:8]
	if err := Migrate(ctx, db, driver); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s := New(db, driver)

	workspaceID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	// Ensure default workspace exists (migrations may seed it).
	project := &domain.Project{
		WorkspaceID: workspaceID,
		Name:        "pg-ci-" + suffix,
	}
	env := &domain.Environment{
		TargetType:   "stub",
		TargetConfig: json.RawMessage(`{"namespace":"default"}`),
	}
	if err := s.CreateProject(ctx, project, env); err != nil {
		t.Fatalf("create project: %v", err)
	}

	svc, err := s.GetServiceByProjectAndName(ctx, project.ID, project.PrimaryService)
	if err != nil {
		t.Fatal(err)
	}
	devEnv, err := s.GetEnvironmentByProjectAndName(ctx, project.ID, "dev")
	if err != nil {
		t.Fatal(err)
	}

	val := "8080"
	if err := s.MergeConfigVars(ctx, svc.ID, devEnv.ID, map[string]*string{"PORT": &val}); err != nil {
		t.Fatal(err)
	}
	cfg, err := s.ListConfigVars(ctx, svc.ID, devEnv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cfg["PORT"] != "8080" {
		t.Fatalf("config: %+v", cfg)
	}

	var deploymentID uuid.UUID
	err = s.Transact(ctx, func(tx *sql.Tx) error {
		release := &domain.Release{
			ServiceID:       svc.ID,
			Version:         1,
			ArtifactRef:     "pg-ci:v1",
			ConfigResolved:  map[string]string{"PORT": "8080"},
			ProcessSnapshot: map[string]domain.ProcessSnapshot{"web": {Quantity: 1}},
			Status:          domain.ReleaseStatusPending,
		}
		if err := s.CreateRelease(ctx, tx, release); err != nil {
			return err
		}
		deployment := &domain.Deployment{
			ServiceID:     svc.ID,
			EnvironmentID: devEnv.ID,
			ReleaseID:     release.ID,
		}
		if err := s.CreateDeployment(ctx, tx, deployment); err != nil {
			return err
		}
		deploymentID = deployment.ID
		payload, _ := json.Marshal(domain.DeployPayload{
			DeploymentID:  deployment.ID,
			ServiceID:     svc.ID,
			EnvironmentID: devEnv.ID,
			ReleaseID:     release.ID,
		})
		return s.EnqueueJob(ctx, tx, &domain.Job{
			Type:         domain.JobTypeDeploy,
			ResourceType: "deployment",
			ResourceID:   deployment.ID,
			Payload:      payload,
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	job, err := s.LeaseNext(ctx, "pg-ci-worker", []domain.JobType{domain.JobTypeDeploy}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if job == nil {
		t.Fatal("expected leased job")
	}
	if job.ResourceID != deploymentID {
		t.Fatalf("job resource %s want %s", job.ResourceID, deploymentID)
	}
	if job.Status != domain.JobStatusLeased {
		t.Fatalf("status %s", job.Status)
	}
	// SKIP LOCKED path: second lease should not return same job.
	job2, err := s.LeaseNext(ctx, "pg-ci-worker-2", []domain.JobType{domain.JobTypeDeploy}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if job2 != nil && job2.ID == job.ID {
		t.Fatal("same job leased twice")
	}
	if err := s.CompleteJob(ctx, job.ID, domain.JobStatusSucceeded, ""); err != nil {
		t.Fatal(err)
	}
}
