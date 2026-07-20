// Package conformance defines a shared checklist for target.Target backends.
package conformance

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/target"
)

// Run exercises Deploy, Status, Logs, Scale, Rollback, and Destroy.
// Targets may be slow (stub sleeps); timeout is generous.
func Run(t *testing.T, tgt target.Target) {
	t.Helper()
	if tgt == nil {
		t.Fatal("nil target")
	}
	if tgt.Type() == "" {
		t.Fatal("target Type() empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	project := domain.Project{
		ID:             uuid.New(),
		Name:           "conform-" + tgt.Type(),
		PrimaryService: "conform-" + tgt.Type(),
		Status:         domain.ProjectStatusCreated,
	}
	svc := domain.Service{
		ID:        uuid.New(),
		ProjectID: project.ID,
		Name:      project.PrimaryService,
		Kind:      "web",
	}
	env := domain.Environment{
		ID:           uuid.New(),
		ProjectID:    project.ID,
		Name:         "dev",
		TargetType:   tgt.Type(),
		TargetConfig: json.RawMessage(`{"namespace":"default"}`),
	}
	release := domain.Release{
		ID:              uuid.New(),
		ServiceID:       svc.ID,
		Version:         1,
		ArtifactRef:     "conform:v1",
		ConfigResolved:  map[string]string{"PORT": "8080"},
		ProcessSnapshot: map[string]domain.ProcessSnapshot{"web": {Quantity: 1, Expose: "http"}},
		Status:          domain.ReleaseStatusPending,
	}
	procs := []domain.Process{{
		ID:        uuid.New(),
		ServiceID: svc.ID,
		Name:      "web",
		Quantity:  1,
		Expose:    "http",
	}}

	deployRes, err := tgt.Deploy(ctx, target.DeployRequest{
		Project:     project,
		Service:     svc,
		Environment: env,
		Release:     release,
		Processes:   procs,
		Config:      release.ConfigResolved,
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if deployRes == nil || deployRes.TargetRef == "" {
		t.Fatalf("Deploy: empty result %+v", deployRes)
	}

	st, err := tgt.Status(ctx, target.StatusRequest{
		Project:     project,
		Service:     svc,
		Environment: env,
		Processes:   procs,
	})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st == nil {
		t.Fatal("Status: nil")
	}

	rc, err := tgt.Logs(ctx, target.LogsRequest{
		Project:     project,
		Service:     svc,
		Environment: env,
		ProcessName: "web",
	})
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	if rc != nil {
		_, _ = io.Copy(io.Discard, rc)
		_ = rc.Close()
	}

	if err := tgt.Scale(ctx, target.ScaleRequest{
		Project:     project,
		Service:     svc,
		Environment: env,
		ProcessName: "web",
		Quantity:    2,
	}); err != nil {
		t.Fatalf("Scale: %v", err)
	}

	release2 := release
	release2.ID = uuid.New()
	release2.Version = 2
	release2.ArtifactRef = "conform:v2"
	rb, err := tgt.Rollback(ctx, target.RollbackRequest{
		Project:     project,
		Service:     svc,
		Environment: env,
		Release:     release2,
		Processes:   procs,
		Config:      release2.ConfigResolved,
	})
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if rb == nil || rb.TargetRef == "" {
		t.Fatalf("Rollback: empty result %+v", rb)
	}

	if err := tgt.Destroy(ctx, target.DestroyRequest{
		Project:     project,
		Service:     svc,
		Environment: env,
	}); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
}
