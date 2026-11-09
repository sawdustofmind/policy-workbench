package application_test

import (
	"math/rand"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"policy-workbench/internal/application"
	"policy-workbench/internal/domain"
	"policy-workbench/internal/storage/jsonfile"
)

// addTopPolicy creates a treasury policy and moves it to the top.
func addTopPolicy(t *testing.T, service *application.Service, policy domain.Policy) string {
	t.Helper()
	created, err := service.CreatePolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	view, err := service.Workspace(policy.ProfileID)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{created.ID}
	for _, existing := range view.Policies {
		if existing.ID != created.ID {
			ids = append(ids, existing.ID)
		}
	}
	if err := service.ReorderPolicies(application.ReorderPoliciesCommand{ProfileID: policy.ProfileID, IDs: ids}); err != nil {
		t.Fatal(err)
	}
	return created.ID
}

func operationsLimit(amount string, interval domain.LimitInterval) domain.Policy {
	return domain.Policy{ProfileID: "treasury", Name: "Operations interval", SenderType: domain.SenderWallet, SenderID: "operations", ReceiverType: domain.ReceiverAny, AmountPolicy: domain.IntervalLimit(amount, interval), Currency: domain.CurrencyUSD, Action: domain.PolicyAllow}
}

func create(t *testing.T, service *application.Service, wallet, amount string) domain.Transfer {
	t.Helper()
	request := transferRequest()
	request.WalletID, request.Amount = wallet, amount
	transfer, err := service.CreateTransfer(request)
	if err != nil {
		t.Fatal(err)
	}
	return transfer
}

func spent(t *testing.T, service *application.Service, wallet string) (string, string) {
	t.Helper()
	balance := walletBalance(t, service, wallet)
	return balance.Spent24h, balance.Spent30d
}

func TestIntervalLimitAccumulatesPerSenderWallet(t *testing.T) {
	service, _ := testService(t, filepath.Join(t.TempDir(), "state.json"))
	if s24, s30 := spent(t, service, "operations"); s24 != "1800" || s30 != "6000" {
		t.Fatal("unexpected demo totals", s24, s30)
	}
	limitID := addTopPolicy(t, service, operationsLimit("10000", domain.Interval24h))

	first := create(t, service, "operations", "6000")
	if first.Decision.PolicyID != limitID {
		t.Fatal("6000 should fit 1800 + 6000 <= 10000", first.Decision.PolicyID)
	}
	second := create(t, service, "operations", "5000")
	if second.Decision.PolicyID != "policy-daily" {
		t.Fatal("5000 should exceed the 24h total and fall through", second.Decision.PolicyID)
	}
	// Transfers allowed by other policies still count toward the wallet total.
	small := create(t, service, "operations", "100")
	if small.Decision.PolicyID == limitID {
		t.Fatal("total including the fallback transfer was ignored")
	}
	if s24, _ := spent(t, service, "operations"); s24 != "12900" {
		t.Fatal("unexpected 24h total", s24)
	}
	// Another wallet in the same group does not touch Operations' total.
	create(t, service, "reserve", "500")
	if s24, _ := spent(t, service, "operations"); s24 != "12900" {
		t.Fatal("another wallet changed Operations total", s24)
	}
	// Releasing funds frees the interval total.
	for _, transfer := range []domain.Transfer{second, small} {
		if _, err := service.ApplyTransferAction(transfer.ID, application.TransferActionCommand{ProfileID: "treasury", Action: domain.ActionCancel}); err != nil {
			t.Fatal(err)
		}
	}
	exact := create(t, service, "operations", "2200")
	if exact.Decision.PolicyID != limitID {
		t.Fatal("inclusive boundary 7800 + 2200 <= 10000 did not match", exact.Decision.PolicyID)
	}
	if _, err := service.AdvanceClock(24); err != nil {
		t.Fatal(err)
	}
	if s24, s30 := spent(t, service, "operations"); s24 != "0" || s30 != "14200" {
		t.Fatal("24h window did not roll over independently of 30d", s24, s30)
	}
	if create(t, service, "operations", "10000").Decision.PolicyID != limitID {
		t.Fatal("limit not available after the window passed")
	}
}

func TestThirtyDayLimit(t *testing.T) {
	service, _ := testService(t, filepath.Join(t.TempDir(), "state.json"))
	limitID := addTopPolicy(t, service, operationsLimit("7000", domain.Interval30d))
	if create(t, service, "operations", "1000").Decision.PolicyID != limitID {
		t.Fatal("6000 + 1000 <= 7000 should match")
	}
	if _, err := service.AdvanceClock(24); err != nil {
		t.Fatal(err)
	}
	if create(t, service, "operations", "1").Decision.PolicyID == limitID {
		t.Fatal("30d total reset after one day")
	}
	if _, err := service.AdvanceClock(30 * 24); err != nil {
		t.Fatal(err)
	}
	if create(t, service, "operations", "7000").Decision.PolicyID != limitID {
		t.Fatal("30d total did not roll over")
	}
}

func assertCacheMatchesTransfers(t *testing.T, service *application.Service, repository *jsonfile.Repository) {
	t.Helper()
	state := repository.Snapshot()
	view, err := service.Workspace("treasury")
	if err != nil {
		t.Fatal(err)
	}
	for _, wallet := range view.Wallets {
		for interval, got := range map[domain.LimitInterval]string{domain.Interval24h: wallet.Spent24h, domain.Interval30d: wallet.Spent30d} {
			want, err := domain.WalletSpend(state.Transfers, "treasury", wallet.ID, state.SimulationTime, interval.Duration())
			if err != nil {
				t.Fatal(err)
			}
			if got != want.String() {
				t.Fatalf("%s %s: cache %s, transfers %s", wallet.ID, interval, got, want)
			}
		}
	}
}

func TestSpendingCacheMatchesTransfers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	service, repository := testService(t, path)
	addTopPolicy(t, service, operationsLimit("20000", domain.Interval24h))
	random := rand.New(rand.NewSource(7))
	wallets := []string{"operations", "reserve"}
	actions := []domain.TransferAction{domain.ActionSign, domain.ActionConfirm, domain.ActionCancel, domain.ActionReject, domain.ActionFail}
	for step := 0; step < 300; step++ {
		switch random.Intn(5) {
		case 0, 1:
			request := transferRequest()
			request.WalletID = wallets[random.Intn(len(wallets))]
			request.Amount = strconv.Itoa(1 + random.Intn(3000))
			_, _ = service.CreateTransfer(request) // insufficient funds is a valid outcome
		case 2, 3:
			state := repository.Snapshot()
			transfer := state.Transfers[random.Intn(len(state.Transfers))]
			_, _ = service.ApplyTransferAction(transfer.ID, application.TransferActionCommand{ProfileID: transfer.ProfileID, Action: actions[random.Intn(len(actions))]})
		case 4:
			if _, err := service.AdvanceClock(1 + random.Intn(30)); err != nil {
				t.Fatal(err)
			}
		}
		assertCacheMatchesTransfers(t, service, repository)
	}
	restarted, restartedRepository := testService(t, path)
	assertCacheMatchesTransfers(t, restarted, restartedRepository)
	if err := restarted.ResetWorkspace(); err != nil {
		t.Fatal(err)
	}
	assertCacheMatchesTransfers(t, restarted, restartedRepository)
	if s24, s30 := spent(t, restarted, "operations"); s24 != "1800" || s30 != "6000" {
		t.Fatal("reset did not restore demo totals", s24, s30)
	}
}

func TestConcurrentCreatesRespectIntervalLimit(t *testing.T) {
	service, _ := testService(t, filepath.Join(t.TempDir(), "state.json"))
	limitID := addTopPolicy(t, service, operationsLimit("2800", domain.Interval24h)) // 1000 left after 1800
	var workers sync.WaitGroup
	var mu sync.Mutex
	matched := 0
	for index := 0; index < 20; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			request := transferRequest()
			transfer, err := service.CreateTransfer(request)
			if err != nil {
				t.Error(err)
				return
			}
			if transfer.Decision.PolicyID == limitID {
				mu.Lock()
				matched++
				mu.Unlock()
			}
		}()
	}
	workers.Wait()
	if matched != 10 {
		t.Fatal("interval limit matched", matched, "transfers of 100 within 1000 remaining")
	}
}
