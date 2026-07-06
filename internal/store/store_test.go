package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
)

func TestProjectBootstrapAndJobQueue(t *testing.T) {
	ctx := context.Background()
	db, driver, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(ctx, db, driver); err != nil {
		t.Fatal(err)
	}
	s := New(db, driver)

	workspaceID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	project := &domain.Project{
		WorkspaceID: workspaceID,
		Name:        "demo",
	}
	env := &domain.Environment{
		TargetType:   "stub",
		TargetConfig: json.RawMessage(`{"namespace":"default"}`),
	}
	if err := s.CreateProject(ctx, project, env); err != nil {
		t.Fatal(err)
	}

	svc, err := s.GetServiceByProjectAndName(ctx, project.ID, project.PrimaryService)
	if err != nil {
		t.Fatal(err)
	}
	devEnv, err := s.GetEnvironmentByProjectAndName(ctx, project.ID, "dev")
	if err != nil {
		t.Fatal(err)
	}
	processes, err := s.ListProcesses(ctx, svc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(processes) != 1 || processes[0].Name != "web" {
		t.Fatalf("expected default web process, got %+v", processes)
	}

	val := "8080"
	if err := s.MergeConfigVars(ctx, svc.ID, devEnv.ID, map[string]*string{"PORT": &val}); err != nil {
		t.Fatal(err)
	}

	err = s.Transact(ctx, func(tx *sql.Tx) error {
		release := &domain.Release{
			ServiceID:       svc.ID,
			Version:         1,
			ArtifactRef:     "demo:v1",
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

	job, err := s.LeaseNext(ctx, "test-worker", []domain.JobType{domain.JobTypeDeploy}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if job == nil {
		t.Fatal("expected leased job")
	}
	if job.Type != domain.JobTypeDeploy {
		t.Fatalf("unexpected job type %s", job.Type)
	}
}