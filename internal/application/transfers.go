package application

import "policy-workbench/internal/domain"

func (service *Service) CreateTransfer(request domain.TransferRequest) (domain.Transfer, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	var transfer domain.Transfer
	err := service.repository.Update(func(state *WorkspaceState) error {
		wallet, _ := service.catalog.FindWallet(request.ProfileID, request.WalletID)
		usage := func(interval domain.LimitInterval) (domain.Decimal, error) {
			return service.spending.Used(request.ProfileID, request.WalletID, wallet.Currency, interval, state.SimulationTime), nil
		}
		decision, err := domain.EvaluatePolicies(state.Policies, service.catalog, request, usage)
		if err != nil {
			return err
		}
		if decision.Outcome == domain.OutcomeAllow {
			balance, err := domain.CalculateWalletBalance(wallet, state.Transfers)
			if err != nil {
				return err
			}
			available, err := domain.ParseMoney(balance.Available)
			if err != nil {
				return err
			}
			amount, err := domain.ParseMoney(request.Amount)
			if err != nil {
				return err
			}
			if amount.Compare(available) > 0 {
				return domain.InvalidRequest("insufficient %s balance: available %s", wallet.Currency, balance.Available)
			}
		}
		transfer = domain.NewTransfer(state.AllocateID("tx"), request, wallet.Currency, decision, state.SimulationTime)
		state.Transfers = append(state.Transfers, transfer)
		return nil
	})
	if err != nil {
		return domain.Transfer{}, err
	}
	return transfer, service.spending.Add(transfer)
}

func (service *Service) ApplyTransferAction(transferID string, command TransferActionCommand) (domain.Transfer, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	var result domain.Transfer
	err := service.repository.Update(func(state *WorkspaceState) error {
		for index := range state.Transfers {
			transfer := &state.Transfers[index]
			if transfer.ID == transferID && transfer.ProfileID == command.ProfileID {
				if err := transfer.ApplyAction(command.Action, state.SimulationTime); err != nil {
					return err
				}
				result = transfer.Clone()
				return nil
			}
		}
		return domain.ErrNotFound
	})
	if err == nil && !domain.ConsumesFunds(result.Status) {
		service.spending.Remove(result)
	}
	return result, err
}
