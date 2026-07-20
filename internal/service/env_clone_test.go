package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/auth"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/store"
)

func TestCloneEnvironmentPlainAndSecrets(t *testing.T) {
	ctx := context.Background()
	db, driver, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(ctx, db, driver); err != nil {
		t.Fatal(err)
	}
	st := store.New(db, driver).WithSecrets(testSecretsBox(t))
	ps := NewProjectService(st)

	workspaceID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	project := &domain.Project{WorkspaceID: workspaceID, Name: "clone-app"}
	if err := st.CreateProject(ctx, project, &domain.Environment{TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}
	ctx = context.WithValue(ctx, auth.ContextTeamID, workspaceID)

	svc, err := st.GetServiceByProjectAndName(ctx, project.ID, project.PrimaryService)
	if err != nil {
		t.Fatal(err)
	}
	dev, err := st.GetEnvironmentByProjectAndName(ctx, project.ID, "dev")
	if err != nil {
		t.Fatal(err)
	}

	plain := "8080"
	secret := "postgres://secret"
	sec := domain.SensitivitySecret
	pln := domain.SensitivityPlain
	if err := st.Transact(ctx, func(tx *sql.Tx) error {
		if err := st.MergeConfigWritesTx(ctx, tx, "shared", uuid.Nil, dev.ID, project.ID, map[string]store.ConfigWrite{
			"LOG_LEVEL": {Value: strPtr("info"), Sensitivity: &pln},
		}); err != nil {
			return err
		}
		return st.MergeConfigWritesTx(ctx, tx, "service", svc.ID, dev.ID, uuid.Nil, map[string]store.ConfigWrite{
			"PORT":         {Value: &plain, Sensitivity: &pln},
			"DATABASE_URL": {Value: &secret, Sensitivity: &sec},
		})
	}); err != nil {
		t.Fatal(err)
	}

	result, err := ps.CloneEnvironment(ctx, "clone-app", "dev", CloneEnvironmentInput{Name: "staging"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Environment.Name != "staging" {
		t.Fatalf("name %s", result.Environment.Name)
	}
	if result.From != "dev" {
		t.Fatalf("from %s", result.From)
	}
	if len(result.ClonedPlain) != 2 {
		t.Fatalf("cloned_plain %+v", result.ClonedPlain)
	}
	if len(result.NeedsValue) != 1 || result.NeedsValue[0] != "DATABASE_URL" {
		t.Fatalf("needs_value %+v", result.NeedsValue)
	}

	svcVals, _, err := st.ListConfigVarsWithSensitivityTx(ctx, nil, svc.ID, result.Environment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if svcVals["PORT"] != "8080" {
		t.Fatalf("PORT %q", svcVals["PORT"])
	}
	if _, ok := svcVals["DATABASE_URL"]; ok {
		t.Fatalf("secret should not be copied: %q", svcVals["DATABASE_URL"])
	}

	sharedVals, _, err := st.ListSharedConfigVarsWithSensitivityTx(ctx, nil, project.ID, result.Environment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sharedVals["LOG_LEVEL"] != "info" {
		t.Fatalf("LOG_LEVEL %q", sharedVals["LOG_LEVEL"])
	}

	if _, err := ps.CloneEnvironment(ctx, "clone-app", "dev", CloneEnvironmentInput{Name: "staging"}); err == nil {
		t.Fatal("expected conflict")
	}
	if _, err := ps.CloneEnvironment(ctx, "clone-app", "dev", CloneEnvironmentInput{Name: "dev"}); err == nil {
		t.Fatal("expected bad request same name")
	}

	r2, err := ps.CloneEnvironment(ctx, "clone-app", "dev", CloneEnvironmentInput{
		Name:   "prod",
		Target: TargetInput{Type: "stub", Namespace: "prod-ns"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var tc map[string]string
	_ = json.Unmarshal(r2.Environment.TargetConfig, &tc)
	if tc["namespace"] != "prod-ns" {
		t.Fatalf("namespace %+v", tc)
	}
}
