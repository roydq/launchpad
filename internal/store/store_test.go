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

func TestAppAndJobQueue(t *testing.T) {
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

	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	app := &domain.App{
		TeamID:       teamID,
		Name:         "demo",
		TargetType:   "stub",
		TargetConfig: json.RawMessage(`{"namespace":"default"}`),
	}
	if err := s.CreateApp(ctx, app); err != nil {
		t.Fatal(err)
	}

	val := "8080"
	if err := s.MergeConfigVars(ctx, app.ID, map[string]*string{"PORT": &val}); err != nil {
		t.Fatal(err)
	}

	err = s.Transact(ctx, func(tx *sql.Tx) error {
		release := &domain.Release{
			AppID:          app.ID,
			Version:        1,
			ConfigSnapshot: map[string]string{"PORT": "8080"},
			ImageRef:       "demo:v1",
			Status:         domain.ReleaseStatusPending,
		}
		if err := s.CreateRelease(ctx, tx, release); err != nil {
			return err
		}
		deployment := &domain.Deployment{AppID: app.ID, ReleaseID: release.ID}
		if err := s.CreateDeployment(ctx, tx, deployment); err != nil {
			return err
		}
		payload, _ := json.Marshal(domain.DeployPayload{
			DeploymentID: deployment.ID,
			AppID:        app.ID,
			ReleaseID:    release.ID,
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