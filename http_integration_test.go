package main

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"policy-workbench/internal/application"
	"policy-workbench/internal/demo"
	"policy-workbench/internal/domain"
	"policy-workbench/internal/httpapi"
	"policy-workbench/internal/storage/jsonfile"
)

func testServer(t *testing.T) (*httptest.Server, *application.Service, *jsonfile.Repository) {
	t.Helper()
	catalog := demo.Catalog()
	seed := func(now time.Time) application.WorkspaceState { return demo.Workspace(catalog, now) }
	repository, err := jsonfile.Open(filepath.Join(t.TempDir(), "state.json"), seed(time.Date(2026, 1, 12, 12, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	service, err := application.NewService(repository, catalog, seed)
	if err != nil {
		t.Fatal(err)
	}
	assets, err := fs.Sub(embeddedFiles, "web")
	if err != nil {
		t.Fatal(err)
	}
	brief, err := embeddedFiles.ReadFile("TASK.md")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.NewHandler(service, assets, brief))
	t.Cleanup(server.Close)
	return server, service, repository
}

func apiRequest(t *testing.T, server *httptest.Server, method, path string, input any, status int) []byte {
	t.Helper()
	var body []byte
	if input != nil {
		body, _ = json.Marshal(input)
	}
	request, err := http.NewRequest(method, server.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode != status {
		t.Fatalf("%s %s: got %d want %d: %s", method, path, response.StatusCode, status, data)
	}
	return data
}

func sampleTransfer() domain.TransferRequest {
	return domain.TransferRequest{ProfileID: "treasury", InitiatorID: "alex", WalletID: "operations", RecipientID: "supplier", Amount: "100"}
}

func TestHTTPCreateSignPersistAndClock(t *testing.T) {
	server, _, repository := testServer(t)
	for _, path := range []string{"/", "/app.js", "/api.js", "/policies.js", "/transfers.js", "/presentation.js", "/ui.js", "/catalog.js", "/wallets.js", "/style.css", "/mark.svg", "/brief"} {
		if len(apiRequest(t, server, "GET", path, nil, 200)) == 0 {
			t.Fatal("empty embedded asset", path)
		}
	}
	input := sampleTransfer()
	input.WalletID = "reserve"
	var transfer domain.Transfer
	if err := json.Unmarshal(apiRequest(t, server, "POST", "/api/transfers", input, 201), &transfer); err != nil {
		t.Fatal(err)
	}
	if transfer.Status != domain.StatusPendingSignature {
		t.Fatal("expected immediate signing")
	}
	for _, action := range []domain.TransferAction{domain.ActionSign, domain.ActionConfirm} {
		apiRequest(t, server, "POST", "/api/transfers/"+transfer.ID+"/actions", application.TransferActionCommand{ProfileID: "treasury", Action: action}, 200)
	}
	apiRequest(t, server, "POST", "/api/transfers/"+transfer.ID+"/actions", application.TransferActionCommand{ProfileID: "treasury", Action: domain.ActionFail}, 409)
	before := repository.Snapshot().SimulationTime
	apiRequest(t, server, "POST", "/api/clock", map[string]int{"hours": 1}, 200)
	apiRequest(t, server, "POST", "/api/clock", map[string]int{"hours": -1}, 400)
	state := repository.Snapshot()
	if !state.SimulationTime.Equal(before.Add(time.Hour)) || state.Transfers[len(state.Transfers)-1].Status != domain.StatusConfirmed {
		t.Fatal("workflow or clock not persisted")
	}
	apiRequest(t, server, "POST", "/api/transfers", map[string]any{"profile_id": "treasury", "created_at": "2000-01-01T00:00:00Z"}, 400)
	apiRequest(t, server, "GET", "/api/state?profile_id=unknown", nil, 400)
}

func TestHTTPPolicyOrderDeletionAndProfileIsolation(t *testing.T) {
	server, _, repository := testServer(t)
	apiRequest(t, server, "PUT", "/api/policies/order", application.ReorderPoliciesCommand{ProfileID: "treasury", IDs: []string{"policy-review", "policy-restricted", "policy-daily", "policy-europe"}}, 200)
	apiRequest(t, server, "PUT", "/api/policies/order", application.ReorderPoliciesCommand{ProfileID: "treasury", IDs: []string{"policy-review", "policy-review", "policy-studio"}}, 400)
	apiRequest(t, server, "DELETE", "/api/policies/policy-review?profile_id=studio", nil, 404)
	var created domain.Transfer
	if err := json.Unmarshal(apiRequest(t, server, "POST", "/api/transfers", sampleTransfer(), 201), &created); err != nil {
		t.Fatal(err)
	}
	if created.Decision.PolicyID != "policy-review" {
		t.Fatal("new policy order not used")
	}
	apiRequest(t, server, "DELETE", "/api/policies/policy-review?profile_id=treasury", nil, 200)
	state := repository.Snapshot()
	if state.Transfers[len(state.Transfers)-1].Decision.PolicyID != "policy-review" {
		t.Fatal("deletion changed recorded decision")
	}
	policy := domain.Policy{ProfileID: "treasury", Name: "New policy", SenderType: "any", ReceiverType: "any", Currency: domain.CurrencyUSD, Action: domain.PolicyReject}
	apiRequest(t, server, "POST", "/api/policies", policy, 201)
	apiRequest(t, server, "POST", "/api/reset", struct{}{}, 200)
	if len(repository.Snapshot().Policies) != 6 {
		t.Fatal("reset did not restore both profiles")
	}
}

func TestHTTPNoMatchIsRecorded(t *testing.T) {
	server, service, _ := testServer(t)
	view, err := service.Workspace("treasury")
	if err != nil {
		t.Fatal(err)
	}
	for _, policy := range view.Policies {
		if err := service.DeletePolicy("treasury", policy.ID); err != nil {
			t.Fatal(err)
		}
	}
	var transfer domain.Transfer
	if err := json.Unmarshal(apiRequest(t, server, "POST", "/api/transfers", sampleTransfer(), 201), &transfer); err != nil {
		t.Fatal(err)
	}
	if transfer.Status != domain.StatusNoPolicyFound || transfer.Decision.Outcome != "NO_MATCH" {
		t.Fatal("no-match outcome not recorded")
	}
}

func TestHTTPRefusesRemovedApprovalInputs(t *testing.T) {
	server, _, repository := testServer(t)
	policy := map[string]any{"profile_id": "treasury", "name": "Obsolete input", "sender_type": "any", "receiver_type": "any", "currency": "USD", "action": "ALLOW", "approvals": 1}
	apiRequest(t, server, "POST", "/api/policies", policy, 400)
	var transfer domain.Transfer
	if err := json.Unmarshal(apiRequest(t, server, "POST", "/api/transfers", sampleTransfer(), 201), &transfer); err != nil {
		t.Fatal(err)
	}
	path := "/api/transfers/" + transfer.ID + "/actions"
	apiRequest(t, server, "POST", path, map[string]string{"profile_id": "treasury", "action": "approve"}, 400)
	apiRequest(t, server, "POST", path, map[string]string{"profile_id": "treasury", "action": "sign", "user_id": "sam"}, 400)
	state := repository.Snapshot()
	if state.Transfers[len(state.Transfers)-1].Status != domain.StatusPendingSignature {
		t.Fatal("invalid input changed transfer")
	}
}

func TestHTTPTransferCurrencyIsServerOwned(t *testing.T) {
	server, _, _ := testServer(t)
	input := map[string]string{"profile_id": "treasury", "initiator_id": "alex", "wallet_id": "operations", "recipient_id": "supplier", "amount": "100", "currency": "EUR"}
	apiRequest(t, server, "POST", "/api/transfers", input, 400)
	delete(input, "currency")
	var transfer domain.Transfer
	if err := json.Unmarshal(apiRequest(t, server, "POST", "/api/transfers", input, 201), &transfer); err != nil {
		t.Fatal(err)
	}
	if transfer.Currency != domain.CurrencyUSD {
		t.Fatal("wrong wallet currency", transfer.Currency)
	}
}

func TestHTTPAmountPolicy(t *testing.T) {
	server, _, _ := testServer(t)
	base := func(amount any) map[string]any {
		return map[string]any{"profile_id": "treasury", "name": "Amount rule", "sender_type": "any", "receiver_type": "any", "currency": "USD", "action": "ALLOW", "amount_policy": amount}
	}
	legacy := base(nil)
	delete(legacy, "amount_policy")
	legacy["amount_limit"] = "100"
	apiRequest(t, server, "POST", "/api/policies", legacy, 400)
	for _, invalid := range []map[string]any{
		{"amount": "100", "amount_type": "count", "interval": "24h"},
		{"amount": "100", "amount_type": "interval"},
		{"amount": "100", "amount_type": "interval", "interval": "7d"},
		{"amount": "", "amount_type": "count"},
	} {
		apiRequest(t, server, "POST", "/api/policies", base(invalid), 400)
	}
	var created domain.Policy
	if err := json.Unmarshal(apiRequest(t, server, "POST", "/api/policies", base(map[string]any{"amount": "10000", "amount_type": "interval", "interval": "30d"}), 201), &created); err != nil {
		t.Fatal(err)
	}
	if created.AmountPolicy == nil || *created.AmountPolicy.Interval != domain.Interval30d {
		t.Fatal("interval policy not stored", created.AmountPolicy)
	}
	apiRequest(t, server, "POST", "/api/policies", base(nil), 201)
	var view struct {
		Wallets []struct {
			ID       string `json:"id"`
			Spent24h string `json:"spent_24h"`
			Spent30d string `json:"spent_30d"`
		} `json:"wallets"`
	}
	if err := json.Unmarshal(apiRequest(t, server, "GET", "/api/state?profile_id=treasury", nil, 200), &view); err != nil {
		t.Fatal(err)
	}
	if view.Wallets[0].ID != "operations" || view.Wallets[0].Spent24h != "1800" || view.Wallets[0].Spent30d != "6000" {
		t.Fatal("wallet spending totals missing", view.Wallets[0])
	}
}
