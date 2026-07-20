package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

// ConfigWrite is a single key update. Value nil deletes the key.
// Sensitivity nil means sticky (existing type, or plain for new keys).
type ConfigWrite struct {
	Value       *string
	Sensitivity *string
}

func (s *Store) ListConfigVars(ctx context.Context, serviceID, environmentID uuid.UUID) (map[string]string, error) {
	vals, _, err := s.ListConfigVarsWithSensitivityTx(ctx, nil, serviceID, environmentID)
	return vals, err
}

func (s *Store) ListConfigVarsTx(ctx context.Context, tx *sql.Tx, serviceID, environmentID uuid.UUID) (map[string]string, error) {
	vals, _, err := s.ListConfigVarsWithSensitivityTx(ctx, tx, serviceID, environmentID)
	return vals, err
}

func (s *Store) ListConfigVarsWithSensitivityTx(ctx context.Context, tx *sql.Tx, serviceID, environmentID uuid.UUID) (map[string]string, map[string]string, error) {
	rows, err := s.exec(tx).QueryContext(ctx, s.q(`
		SELECT key, value, COALESCE(sensitivity, 'plain') FROM config_vars
		WHERE service_id = ? AND environment_id = ? ORDER BY key`),
		serviceID.String(), environmentID.String())
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	return s.scanConfigRows(rows)
}

func (s *Store) MergeConfigVarsTx(ctx context.Context, tx *sql.Tx, serviceID, environmentID uuid.UUID, updates map[string]*string) error {
	writes := make(map[string]ConfigWrite, len(updates))
	for k, v := range updates {
		writes[k] = ConfigWrite{Value: v}
	}
	return s.MergeConfigWritesTx(ctx, tx, "service", serviceID, environmentID, uuid.Nil, writes)
}

func (s *Store) MergeConfigVars(ctx context.Context, serviceID, environmentID uuid.UUID, updates map[string]*string) error {
	return s.Transact(ctx, func(tx *sql.Tx) error {
		return s.MergeConfigVarsTx(ctx, tx, serviceID, environmentID, updates)
	})
}

func (s *Store) ListSharedConfigVars(ctx context.Context, projectID, environmentID uuid.UUID) (map[string]string, error) {
	vals, _, err := s.ListSharedConfigVarsWithSensitivityTx(ctx, nil, projectID, environmentID)
	return vals, err
}

func (s *Store) ListSharedConfigVarsTx(ctx context.Context, tx *sql.Tx, projectID, environmentID uuid.UUID) (map[string]string, error) {
	vals, _, err := s.ListSharedConfigVarsWithSensitivityTx(ctx, tx, projectID, environmentID)
	return vals, err
}

func (s *Store) ListSharedConfigVarsWithSensitivityTx(ctx context.Context, tx *sql.Tx, projectID, environmentID uuid.UUID) (map[string]string, map[string]string, error) {
	rows, err := s.exec(tx).QueryContext(ctx, s.q(`
		SELECT key, value, COALESCE(sensitivity, 'plain') FROM shared_config_vars
		WHERE project_id = ? AND environment_id = ? ORDER BY key`),
		projectID.String(), environmentID.String())
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	return s.scanConfigRows(rows)
}

func (s *Store) MergeSharedConfigVarsTx(ctx context.Context, tx *sql.Tx, projectID, environmentID uuid.UUID, updates map[string]*string) error {
	writes := make(map[string]ConfigWrite, len(updates))
	for k, v := range updates {
		writes[k] = ConfigWrite{Value: v}
	}
	return s.MergeConfigWritesTx(ctx, tx, "shared", uuid.Nil, environmentID, projectID, writes)
}

// MergeConfigWritesTx merges writes into service or shared layer with sticky sensitivity.
// layer is "service" (uses serviceID) or "shared" (uses projectID).
func (s *Store) MergeConfigWritesTx(ctx context.Context, tx *sql.Tx, layer string, serviceID, environmentID, projectID uuid.UUID, writes map[string]ConfigWrite) error {
	if len(writes) == 0 {
		return nil
	}
	var existingSens map[string]string
	var err error
	switch layer {
	case "service":
		_, existingSens, err = s.ListConfigVarsWithSensitivityTx(ctx, tx, serviceID, environmentID)
	case "shared":
		_, existingSens, err = s.ListSharedConfigVarsWithSensitivityTx(ctx, tx, projectID, environmentID)
	default:
		return fmt.Errorf("%w: unknown config layer %q", launchpad.ErrBadRequest, layer)
	}
	if err != nil {
		return err
	}
	if existingSens == nil {
		existingSens = map[string]string{}
	}

	exec := s.exec(tx)
	now := formatTime(s.driver, time.Now().UTC())
	for key, w := range writes {
		if w.Value == nil {
			if layer == "shared" {
				if _, err := exec.ExecContext(ctx, s.q(`
					DELETE FROM shared_config_vars WHERE project_id = ? AND environment_id = ? AND key = ?`),
					projectID.String(), environmentID.String(), key); err != nil {
					return err
				}
			} else {
				if _, err := exec.ExecContext(ctx, s.q(`
					DELETE FROM config_vars WHERE service_id = ? AND environment_id = ? AND key = ?`),
					serviceID.String(), environmentID.String(), key); err != nil {
					return err
				}
			}
			continue
		}
		if w.Sensitivity != nil {
			if domain.NormalizeSensitivity(*w.Sensitivity) == "" {
				return fmt.Errorf("%w: sensitivity must be plain or secret", launchpad.ErrBadRequest)
			}
		}
		sens := domain.EffectiveSensitivity(existingSens[key], w.Sensitivity)
		stored, err := s.sealValue(*w.Value, sens)
		if err != nil {
			return err
		}
		if layer == "shared" {
			_, err = exec.ExecContext(ctx, s.q(`
				INSERT INTO shared_config_vars (project_id, environment_id, key, value, sensitivity, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(project_id, environment_id, key) DO UPDATE SET
					value = excluded.value, sensitivity = excluded.sensitivity, updated_at = excluded.updated_at`),
				projectID.String(), environmentID.String(), key, stored, sens, now, now,
			)
		} else {
			_, err = exec.ExecContext(ctx, s.q(`
				INSERT INTO config_vars (service_id, environment_id, key, value, sensitivity, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(service_id, environment_id, key) DO UPDATE SET
					value = excluded.value, sensitivity = excluded.sensitivity, updated_at = excluded.updated_at`),
				serviceID.String(), environmentID.String(), key, stored, sens, now, now,
			)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// ResolveConfig merges shared then service layers (service wins).
func (s *Store) ResolveConfigTx(ctx context.Context, tx *sql.Tx, projectID, serviceID, environmentID uuid.UUID) (map[string]string, error) {
	vals, _, err := s.ResolveConfigWithSensitivityTx(ctx, tx, projectID, serviceID, environmentID)
	return vals, err
}

func (s *Store) ResolveConfig(ctx context.Context, projectID, serviceID, environmentID uuid.UUID) (map[string]string, error) {
	return s.ResolveConfigTx(ctx, nil, projectID, serviceID, environmentID)
}

// ResolveConfigWithSensitivityTx returns resolved values and winning sensitivity per key.
func (s *Store) ResolveConfigWithSensitivityTx(ctx context.Context, tx *sql.Tx, projectID, serviceID, environmentID uuid.UUID) (map[string]string, map[string]string, error) {
	sharedVals, sharedSens, err := s.ListSharedConfigVarsWithSensitivityTx(ctx, tx, projectID, environmentID)
	if err != nil {
		return nil, nil, err
	}
	svcVals, svcSens, err := s.ListConfigVarsWithSensitivityTx(ctx, tx, serviceID, environmentID)
	if err != nil {
		return nil, nil, err
	}
	out := make(map[string]string, len(sharedVals)+len(svcVals))
	for k, v := range sharedVals {
		out[k] = v
	}
	for k, v := range svcVals {
		out[k] = v
	}
	return out, domain.ResolveSensitivityWinner(sharedSens, svcSens), nil
}

func (s *Store) ResolveConfigWithSensitivity(ctx context.Context, projectID, serviceID, environmentID uuid.UUID) (map[string]string, map[string]string, error) {
	return s.ResolveConfigWithSensitivityTx(ctx, nil, projectID, serviceID, environmentID)
}

func (s *Store) scanConfigRows(rows *sql.Rows) (map[string]string, map[string]string, error) {
	vals := make(map[string]string)
	sens := make(map[string]string)
	for rows.Next() {
		var key, value, sensitivity string
		if err := rows.Scan(&key, &value, &sensitivity); err != nil {
			return nil, nil, err
		}
		if sensitivity == "" {
			sensitivity = domain.SensitivityPlain
		}
		opened, err := s.openValue(value, sensitivity)
		if err != nil {
			return nil, nil, fmt.Errorf("config key %q: %w", key, err)
		}
		vals[key] = opened
		sens[key] = sensitivity
	}
	return vals, sens, rows.Err()
}
