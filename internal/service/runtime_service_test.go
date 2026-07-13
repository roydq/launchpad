package service

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/auth"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/store"
	"github.com/launchpad/launchpad/internal/target"
	"github.com/launchpad/launchpad/internal/target/stub"
)

func TestRuntimeServiceLogsStub(t *testing.T) {
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
	project := &domain.Project{WorkspaceID: workspaceID, Name: "log-app"}
	if err := st.CreateProject(ctx, project, &domain.Environment{
		TargetType:   "stub",
		TargetConfig: json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}
	projectSvc := NewProjectService(st)
	reg := target.NewRegistry()
	reg.Register(stub.New())
	rt := NewRuntimeService(st, projectSvc, reg)
	ctx = context.WithValue(ctx, auth.ContextTeamID, workspaceID)

	rc, err := rt.Logs(ctx, "log-app", "dev", "web")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	body, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "stub log line\n" {
		t.Fatalf("body: %q", body)
	}
}
