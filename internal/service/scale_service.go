package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/store"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

type ScaleService struct {
	store      *store.Store
	appService *AppService
}

func NewScaleService(s *store.Store, appService *AppService) *ScaleService {
	return &ScaleService{store: s, appService: appService}
}

type ScaleResult struct {
	Process domain.ProcessType `json:"process"`
	Job     domain.Job         `json:"job"`
}

func (s *ScaleService) ScaleProcess(ctx context.Context, appName, processName string, quantity int) (*ScaleResult, error) {
	if quantity < 0 {
		return nil, fmt.Errorf("%w: quantity must be >= 0", launchpad.ErrBadRequest)
	}
	app, err := s.appService.GetApp(ctx, appName)
	if err != nil {
		return nil, err
	}

	var result ScaleResult
	err = s.store.Transact(ctx, func(tx *sql.Tx) error {
		if err := s.store.UpdateProcessQuantity(ctx, tx, app.ID, processName, quantity); err != nil {
			return err
		}

		payload, _ := json.Marshal(domain.ScalePayload{
			AppID: app.ID, ProcessName: processName, Quantity: quantity,
		})
		job := &domain.Job{
			Type:         domain.JobTypeScale,
			ResourceType: "app",
			ResourceID:   app.ID,
			Payload:      payload,
		}
		if err := s.store.EnqueueJob(ctx, tx, job); err != nil {
			return err
		}
		result = ScaleResult{
			Process: domain.ProcessType{AppID: app.ID, Name: processName, Quantity: quantity},
			Job:     *job,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	pt, err := s.store.GetProcessType(ctx, app.ID, processName)
	if err == nil {
		result.Process = *pt
	}
	return &result, nil
}