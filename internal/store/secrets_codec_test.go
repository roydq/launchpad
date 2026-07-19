package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/secrets"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

func testBox(t *testing.T) *secrets.Box {
	t.Helper()
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i + 7)
	}
	box, err := secrets.ParseKey(base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatal(err)
	}
	return box
}

func TestSecretConfigEncryptedAtRest(t *testing.T) {
	ctx := context.Background()
	db, driver, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(ctx, db, driver); err != nil {
		t.Fatal(err)
	}
	st := New(db, driver).WithSecrets(testBox(t))

	ws := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	project := &domain.Project{WorkspaceID: ws, Name: "enc-app"}
	env := &domain.Environment{
		TargetType:   "stub",
		TargetConfig: json.RawMessage(`{"namespace":"default"}`),
	}
	if err := st.CreateProject(ctx, project, env); err != nil {
		t.Fatal(err)
	}
	svc, err := st.GetServiceByProjectAndName(ctx, project.ID, project.PrimaryService)
	if err != nil {
		t.Fatal(err)
	}
	envRow, err := st.GetEnvironmentByProjectAndName(ctx, project.ID, "dev")
	if err != nil {
		t.Fatal(err)
	}

	sec := domain.SensitivitySecret
	plain := "postgres://secret-value"
	if err := st.MergeConfigWritesTx(ctx, nil, "service", svc.ID, envRow.ID, uuid.Nil, map[string]ConfigWrite{
		"DATABASE_URL": {Value: &plain, Sensitivity: &sec},
		"PORT":         {Value: strPtr("3000")},
	}); err != nil {
		t.Fatal(err)
	}

	// Raw DB row must be ciphertext for secret, plaintext for plain.
	var rawVal, rawSens string
	err = db.QueryRowContext(ctx, `
		SELECT value, sensitivity FROM config_vars
		WHERE service_id = ? AND environment_id = ? AND key = ?`,
		svc.ID.String(), envRow.ID.String(), "DATABASE_URL").Scan(&rawVal, &rawSens)
	if err != nil {
		t.Fatal(err)
	}
	if rawSens != domain.SensitivitySecret {
		t.Fatalf("sensitivity: %q", rawSens)
	}
	if !secrets.IsCiphertext(rawVal) {
		t.Fatalf("expected ciphertext in DB, got %q", rawVal)
	}
	if strings.Contains(rawVal, "postgres://") {
		t.Fatal("DB leaked plaintext secret")
	}

	// Store list returns decrypted plaintext.
	vals, sens, err := st.ListConfigVarsWithSensitivityTx(ctx, nil, svc.ID, envRow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if vals["DATABASE_URL"] != plain {
		t.Fatalf("list decrypt: %q", vals["DATABASE_URL"])
	}
	if sens["DATABASE_URL"] != domain.SensitivitySecret {
		t.Fatalf("sens: %v", sens)
	}
	if vals["PORT"] != "3000" {
		t.Fatalf("PORT: %q", vals["PORT"])
	}

	// Release snapshot seals secrets; scan opens them.
	rel := &domain.Release{
		ServiceID:         svc.ID,
		Version:           1,
		ArtifactRef:       "img:1",
		ConfigResolved:    map[string]string{"DATABASE_URL": plain, "PORT": "3000"},
		ConfigSensitivity: map[string]string{"DATABASE_URL": domain.SensitivitySecret, "PORT": domain.SensitivityPlain},
		ProcessSnapshot:   map[string]domain.ProcessSnapshot{"web": {Quantity: 1, Expose: "http"}},
		Status:            domain.ReleaseStatusPending,
	}
	if err := st.CreateRelease(ctx, nil, rel); err != nil {
		t.Fatal(err)
	}
	var relJSON string
	err = db.QueryRowContext(ctx, `SELECT config_resolved FROM releases WHERE id = ?`, rel.ID.String()).Scan(&relJSON)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(relJSON, "postgres://") {
		t.Fatalf("release JSON leaked secret: %s", relJSON)
	}
	if !strings.Contains(relJSON, secrets.CipherPrefix) {
		t.Fatalf("release JSON should contain ciphertext: %s", relJSON)
	}

	loaded, err := st.GetReleaseByID(ctx, rel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ConfigResolved["DATABASE_URL"] != plain {
		t.Fatalf("loaded release: %v", loaded.ConfigResolved)
	}
}

func TestSecretWriteRequiresKey(t *testing.T) {
	ctx := context.Background()
	db, driver, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(ctx, db, driver); err != nil {
		t.Fatal(err)
	}
	st := New(db, driver) // no secrets box

	ws := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	project := &domain.Project{WorkspaceID: ws, Name: "nokey"}
	env := &domain.Environment{
		TargetType:   "stub",
		TargetConfig: json.RawMessage(`{}`),
	}
	if err := st.CreateProject(ctx, project, env); err != nil {
		t.Fatal(err)
	}
	svc, err := st.GetServiceByProjectAndName(ctx, project.ID, project.PrimaryService)
	if err != nil {
		t.Fatal(err)
	}
	envRow, err := st.GetEnvironmentByProjectAndName(ctx, project.ID, "dev")
	if err != nil {
		t.Fatal(err)
	}
	sec := domain.SensitivitySecret
	v := "secret"
	err = st.MergeConfigWritesTx(ctx, nil, "service", svc.ID, envRow.ID, uuid.Nil, map[string]ConfigWrite{
		"S": {Value: &v, Sensitivity: &sec},
	})
	if err == nil || !errors.Is(err, launchpad.ErrBadRequest) {
		t.Fatalf("expected ErrBadRequest, got %v", err)
	}
	// Plain still works without key.
	p := "ok"
	if err := st.MergeConfigWritesTx(ctx, nil, "service", svc.ID, envRow.ID, uuid.Nil, map[string]ConfigWrite{
		"PORT": {Value: &p},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyPlaintextSecretStillReadable(t *testing.T) {
	ctx := context.Background()
	db, driver, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(ctx, db, driver); err != nil {
		t.Fatal(err)
	}
	// Insert S1-style plaintext secret without going through seal.
	st := New(db, driver).WithSecrets(testBox(t))
	ws := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	project := &domain.Project{WorkspaceID: ws, Name: "legacy"}
	env := &domain.Environment{TargetType: "stub", TargetConfig: json.RawMessage(`{}`)}
	if err := st.CreateProject(ctx, project, env); err != nil {
		t.Fatal(err)
	}
	svc, err := st.GetServiceByProjectAndName(ctx, project.ID, project.PrimaryService)
	if err != nil {
		t.Fatal(err)
	}
	envRow, err := st.GetEnvironmentByProjectAndName(ctx, project.ID, "dev")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO config_vars (service_id, environment_id, key, value, sensitivity, created_at, updated_at)
		VALUES (?, ?, 'LEGACY', 'plaintext-secret', 'secret', datetime('now'), datetime('now'))`,
		svc.ID.String(), envRow.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	vals, _, err := st.ListConfigVarsWithSensitivityTx(ctx, nil, svc.ID, envRow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if vals["LEGACY"] != "plaintext-secret" {
		t.Fatalf("legacy: %q", vals["LEGACY"])
	}
}

func strPtr(s string) *string { return &s }
