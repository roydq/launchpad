package jobs

import (
	"context"
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

func TestWorkerScaleJob(t *testing.T) {
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

	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	app := &domain.App{TeamID: teamID, Name: "scale-app", TargetType: "stub", TargetConfig: json.RawMessage(`{"namespace":"default"}`)}
	if err := st.CreateApp(ctx, app); err != nil {
		t.Fatal(err)
	}

	payload, _ := json.Marshal(domain.ScalePayload{AppID: app.ID, ProcessName: "web", Quantity: 2})
	if err := st.EnqueueJob(ctx, nil, &domain.Job{
		Type: domain.JobTypeScale, ResourceType: "app", ResourceID: app.ID, Payload: payload,
	}); err != nil {
		t.Fatal(err)
	}

	registry := target.NewRegistry()
	registry.Register(stub.New())
	worker := NewWorker(st, registry, "test", slog.New(slog.NewTextHandler(os.Stdout, nil)))

	job, err := st.LeaseNext(ctx, "test", []domain.JobType{domain.JobTypeScale}, time.Minute)
	if err != nil || job == nil {
		t.Fatal("expected scale job")
	}
	if err := worker.handleScale(ctx, job); err != nil {
		t.Fatal(err)
	}
}