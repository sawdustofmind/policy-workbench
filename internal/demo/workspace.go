package demo

import (
	"policy-workbench/internal/application"
	"policy-workbench/internal/domain"
	"time"
)

func Workspace(catalog domain.Catalog, now time.Time) application.WorkspaceState {
	now = now.UTC().Truncate(time.Second)
	state := application.WorkspaceState{Version: application.StateVersion, SimulationTime: now, IDSequence: 100, Policies: []domain.Policy{}, Transfers: []domain.Transfer{}}
	state.Policies = []domain.Policy{
		{ID: "policy-restricted", ProfileID: "treasury", Name: "Restricted USD recipients", SenderType: domain.SenderAny, ReceiverType: domain.ReceiverRecipientGroup, ReceiverID: "restricted", Currency: domain.CurrencyUSD, Action: domain.PolicyReject, CreatedAt: now.Add(-72 * time.Hour)},
		{ID: "policy-daily", ProfileID: "treasury", Name: "Operations payments", SenderType: domain.SenderWalletGroup, SenderID: "daily-wallets", ReceiverType: domain.ReceiverAny, AmountPolicy: domain.CountLimit("10000"), Currency: domain.CurrencyUSD, Action: domain.PolicyAllow, CreatedAt: now.Add(-72 * time.Hour)},
		{ID: "policy-review", ProfileID: "treasury", Name: "Treasury fallback", SenderType: domain.SenderAny, ReceiverType: domain.ReceiverAny, Currency: domain.CurrencyUSD, Action: domain.PolicyAllow, CreatedAt: now.Add(-72 * time.Hour)},
		{ID: "policy-europe", ProfileID: "treasury", Name: "Europe payments", SenderType: domain.SenderWallet, SenderID: "europe", ReceiverType: domain.ReceiverRecipientGroup, ReceiverID: "suppliers", Currency: domain.CurrencyEUR, Action: domain.PolicyAllow, CreatedAt: now.Add(-72 * time.Hour)},
		{ID: "policy-studio", ProfileID: "studio", Name: "Studio USD payments", SenderType: domain.SenderAny, ReceiverType: domain.ReceiverAny, AmountPolicy: domain.CountLimit("5000"), Currency: domain.CurrencyUSD, Action: domain.PolicyAllow, CreatedAt: now.Add(-72 * time.Hour)},
		{ID: "policy-studio-eur", ProfileID: "studio", Name: "Studio EUR payments", SenderType: domain.SenderAny, ReceiverType: domain.ReceiverAny, AmountPolicy: domain.CountLimit("5000"), Currency: domain.CurrencyEUR, Action: domain.PolicyAllow, CreatedAt: now.Add(-72 * time.Hour)},
	}
	fixtures := []struct {
		age     time.Duration
		request domain.TransferRequest
		status  domain.TransferStatus
	}{
		{26 * time.Hour, domain.TransferRequest{ProfileID: "treasury", InitiatorID: "alex", WalletID: "operations", RecipientID: "supplier", Amount: "4200", Note: "Supplier invoice"}, domain.StatusConfirmed},
		{3 * time.Hour, domain.TransferRequest{ProfileID: "treasury", InitiatorID: "sam", WalletID: "operations", RecipientID: "supplier", Amount: "1800", Note: "Monthly subscription"}, domain.StatusConfirmed},
		{time.Hour, domain.TransferRequest{ProfileID: "treasury", InitiatorID: "alex", WalletID: "reserve", RecipientID: "partner", Amount: "6000", Note: "Reserve allocation"}, domain.StatusPendingSignature},
		{25 * time.Minute, domain.TransferRequest{ProfileID: "treasury", InitiatorID: "jordan", WalletID: "operations", RecipientID: "restricted", Amount: "250", Note: "Recipient check"}, domain.StatusRejectedByPolicy},
		{40 * time.Minute, domain.TransferRequest{ProfileID: "studio", InitiatorID: "casey", WalletID: "studio-wallet", RecipientID: "studio-supplier", Amount: "600", Note: "Design services"}, domain.StatusPendingSignature},
	}
	for _, fixture := range fixtures {
		state.SimulationTime = now.Add(-fixture.age)
		usage := func(interval domain.LimitInterval) (domain.Decimal, error) {
			return domain.WalletSpend(state.Transfers, fixture.request.ProfileID, fixture.request.WalletID, state.SimulationTime, interval.Duration())
		}
		decision, err := domain.EvaluatePolicies(state.Policies, catalog, fixture.request, usage)
		if err != nil {
			panic("invalid demo transfer: " + err.Error())
		}
		wallet, _ := catalog.FindWallet(fixture.request.ProfileID, fixture.request.WalletID)
		transfer := domain.NewTransfer(state.AllocateID("tx"), fixture.request, wallet.Currency, decision, state.SimulationTime)
		transfer.Status = fixture.status
		state.Transfers = append(state.Transfers, transfer)
	}
	state.SimulationTime = now
	return state
}
