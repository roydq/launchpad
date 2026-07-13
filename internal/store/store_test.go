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

func TestResolveConfigSharedThenService(t *testing.T) {
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
	project := &domain.Project{WorkspaceID: workspaceID, Name: "cfg-layers"}
	if err := s.CreateProject(ctx, project, &domain.Environment{TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}
	svc, _ := s.GetServiceByProjectAndName(ctx, project.ID, project.PrimaryService)
	env, _ := s.GetEnvironmentByProjectAndName(ctx, project.ID, "dev")
	sharedVal := "from-shared"
	svcVal := "from-service"
	both := "service-wins"
	_ = s.Transact(ctx, func(tx *sql.Tx) error {
		if err := s.MergeSharedConfigVarsTx(ctx, tx, project.ID, env.ID, map[string]*string{
			"LOG_LEVEL": &sharedVal, "BOTH": &sharedVal,
		}); err != nil {
			return err
		}
		return s.MergeConfigVarsTx(ctx, tx, svc.ID, env.ID, map[string]*string{
			"PORT": &svcVal, "BOTH": &both,
		})
	})
	resolved, err := s.ResolveConfig(ctx, project.ID, svc.ID, env.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved["LOG_LEVEL"] != "from-shared" || resolved["PORT"] != "from-service" || resolved["BOTH"] != "service-wins" {
		t.Fatalf("%+v", resolved)
	}
}

func TestCreateSecondEnvironmentAndChangesetPin(t *testing.T) {
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
	project := &domain.Project{WorkspaceID: workspaceID, Name: "multi-env"}
	if err := s.CreateProject(ctx, project, &domain.Environment{TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}
	staging := &domain.Environment{
		ProjectID:    project.ID,
		Name:         "staging",
		TargetType:   "stub",
		TargetConfig: json.RawMessage(`{"namespace":"staging"}`),
	}
	if err := s.CreateEnvironment(ctx, staging); err != nil {
		t.Fatal(err)
	}
	envs, err := s.ListEnvironments(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 2 {
		t.Fatalf("expected 2 envs, got %d", len(envs))
	}

	err = s.Transact(ctx, func(tx *sql.Tx) error {
		cs, err := s.GetOrCreateOpenChangeset(ctx, tx, project.ID)
		if err != nil {
			return err
		}
		if cs.EnvironmentID != nil {
			t.Fatalf("expected nil pin on fresh changeset")
		}
		return s.SetChangesetEnvironment(ctx, tx, cs.ID, staging.ID)
	})
	if err != nil {
		t.Fatal(err)
	}
	open, err := s.GetOpenChangeset(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if open.EnvironmentID == nil || *open.EnvironmentID != staging.ID {
		t.Fatalf("expected pin to staging, got %v", open.EnvironmentID)
	}
}

func TestActiveDeploymentUniqueAndSupersede(t *testing.T) {
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
	project := &domain.Project{WorkspaceID: workspaceID, Name: "uniq-demo"}
	if err := s.CreateProject(ctx, project, &domain.Environment{TargetType: "stub"}); err != nil {
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

	var first *domain.Deployment
	err = s.Transact(ctx, func(tx *sql.Tx) error {
		r1 := &domain.Release{
			ServiceID: svc.ID, Version: 1, ArtifactRef: "img:1",
			ConfigResolved: map[string]string{}, ProcessSnapshot: map[string]domain.ProcessSnapshot{"web": {Quantity: 1, Expose: "http"}},
		}
		if err := s.CreateRelease(ctx, tx, r1); err != nil {
			return err
		}
		first = &domain.Deployment{ServiceID: svc.ID, EnvironmentID: devEnv.ID, ReleaseID: r1.ID}
		return s.CreateDeployment(ctx, tx, first)
	})
	if err != nil {
		t.Fatal(err)
	}

	err = s.Transact(ctx, func(tx *sql.Tx) error {
		r2 := &domain.Release{
			ServiceID: svc.ID, Version: 2, ArtifactRef: "img:2",
			ConfigResolved: map[string]string{}, ProcessSnapshot: map[string]domain.ProcessSnapshot{"web": {Quantity: 1, Expose: "http"}},
		}
		if err := s.CreateRelease(ctx, tx, r2); err != nil {
			return err
		}
		second := &domain.Deployment{ServiceID: svc.ID, EnvironmentID: devEnv.ID, ReleaseID: r2.ID}
		return s.CreateDeployment(ctx, tx, second)
	})
	if err == nil {
		t.Fatal("expected unique violation for second active deployment")
	}

	if err := s.Transact(ctx, func(tx *sql.Tx) error {
		return s.UpdateDeploymentStatus(ctx, tx, first.ID, domain.DeploymentPending, domain.DeploymentDeploying, "go")
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Transact(ctx, func(tx *sql.Tx) error {
		return s.UpdateDeploymentStatus(ctx, tx, first.ID, domain.DeploymentDeploying, domain.DeploymentRunning, "ok")
	}); err != nil {
		t.Fatal(err)
	}

	var secondID uuid.UUID
	err = s.Transact(ctx, func(tx *sql.Tx) error {
		r3 := &domain.Release{
			ServiceID: svc.ID, Version: 3, ArtifactRef: "img:3",
			ConfigResolved: map[string]string{}, ProcessSnapshot: map[string]domain.ProcessSnapshot{"web": {Quantity: 1, Expose: "http"}},
		}
		if err := s.CreateRelease(ctx, tx, r3); err != nil {
			return err
		}
		second := &domain.Deployment{ServiceID: svc.ID, EnvironmentID: devEnv.ID, ReleaseID: r3.ID}
		if err := s.CreateDeployment(ctx, tx, second); err != nil {
			return err
		}
		secondID = second.ID
		if err := s.UpdateDeploymentStatus(ctx, tx, second.ID, domain.DeploymentPending, domain.DeploymentDeploying, "go"); err != nil {
			return err
		}
		return s.SupersedeRunningDeployments(ctx, tx, svc.ID, devEnv.ID, second.ID)
	})
	if err != nil {
		t.Fatal(err)
	}

	prev, err := s.GetDeployment(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if prev.Status != domain.DeploymentSuperseded {
		t.Fatalf("expected superseded, got %s", prev.Status)
	}
	cur, err := s.GetDeployment(ctx, secondID)
	if err != nil {
		t.Fatal(err)
	}
	if cur.Status != domain.DeploymentDeploying {
		t.Fatalf("expected deploying, got %s", cur.Status)
	}
}