package application

import (
	"time"

	"policy-workbench/internal/domain"
)

func (service *Service) AdvanceClock(hours int) (time.Time, error) {
	if hours < 1 || hours > 8760 {
		return time.Time{}, domain.InvalidRequest("advance between 1 and 8760 whole hours")
	}
	var now time.Time
	err := service.repository.Update(func(state *WorkspaceState) error {
		state.SimulationTime = state.SimulationTime.Add(time.Duration(hours) * time.Hour)
		now = state.SimulationTime
		return nil
	})
	return now, err
}

func (service *Service) ResetWorkspace() error {
	service.mu.Lock()
	defer service.mu.Unlock()
	var transfers []domain.Transfer
	err := service.repository.Update(func(state *WorkspaceState) error {
		*state = service.seedWorkspace(time.Now())
		transfers = state.Transfers
		return nil
	})
	if err != nil {
		return err
	}
	return service.spending.Rebuild(transfers)
}
