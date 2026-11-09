package domain_test

import (
	"strings"
	"testing"
	"time"

	"policy-workbench/internal/demo"
	"policy-workbench/internal/domain"
)

func sampleInput() domain.TransferRequest {
	return domain.TransferRequest{ProfileID: "treasury", InitiatorID: "alex", WalletID: "operations", RecipientID: "supplier", Amount: "100"}
}
func samplePolicy(id string) domain.Policy {
	return domain.Policy{ID: id, ProfileID: "treasury", Name: id, SenderType: domain.SenderAny, ReceiverType: domain.ReceiverAny, Currency: domain.CurrencyUSD, Action: domain.PolicyAllow}
}
func evaluate(t *testing.T, policies []domain.Policy, request domain.TransferRequest) domain.PolicyDecision {
	t.Helper()
	decision, err := domain.EvaluatePolicies(policies, demo.Catalog(), request, noSpending)
	if err != nil {
		t.Fatal(err)
	}
	return decision
}
func TestExactMoney(t *testing.T) {
	for _, value := range []string{"-1", "+1", "1e3", "NaN", "1.", ".1", "01", "0.001", ""} {
		if _, err := domain.ParseMoney(value); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
	large, _ := domain.ParseMoney("9007199254740993.99")
	cent, _ := domain.ParseMoney("0.01")
	if got := large.Add(cent).String(); got != "9007199254740994" {
		t.Fatal("addition lost precision", got)
	}
	if got := large.Subtract(cent).String(); got != "9007199254740993.98" {
		t.Fatal("subtraction lost precision", got)
	}
	one, _ := domain.ParseMoney("1.25")
	two, _ := domain.ParseMoney("1.75")
	if got := one.Subtract(two).String(); got != "-0.5" {
		t.Fatal("incorrect signed difference", got)
	}
	maximum := strings.Repeat("9", 76) + ".99"
	if _, err := domain.ParseMoney(maximum); err != nil {
		t.Fatal("maximum representation rejected", err)
	}
	if _, err := domain.ParseMoney(strings.Repeat("9", 79)); err == nil {
		t.Fatal("oversized input accepted")
	}
}
func TestAmountBoundaryAndFallthrough(t *testing.T) {
	first, fallback := samplePolicy("bounded"), samplePolicy("fallback")
	first.AmountPolicy = domain.CountLimit("100")
	for _, tc := range []struct{ amount, expected string }{{"99.99", "bounded"}, {"100", "bounded"}, {"100.01", "fallback"}} {
		request := sampleInput()
		request.Amount = tc.amount
		if got := evaluate(t, []domain.Policy{first, fallback}, request); got.PolicyID != tc.expected {
			t.Errorf("%s selected %s", tc.amount, got.PolicyID)
		}
	}
	first.AmountPolicy = domain.CountLimit("0")
	if evaluate(t, []domain.Policy{first, fallback}, sampleInput()).PolicyID != fallback.ID {
		t.Fatal("zero treated as unrestricted")
	}
	first.AmountPolicy = nil
	if evaluate(t, []domain.Policy{first, fallback}, sampleInput()).PolicyID != first.ID {
		t.Fatal("blank upper bound did not match")
	}
}
func TestEverySelectorCanExcludePolicy(t *testing.T) {
	cases := []struct {
		name   string
		change func(*domain.Policy)
	}{
		{"profile", func(p *domain.Policy) { p.ProfileID = "studio" }},
		{"initiator", func(p *domain.Policy) { p.InitiatorID = "sam" }},
		{"wallet", func(p *domain.Policy) { p.SenderType, p.SenderID = domain.SenderWallet, "reserve" }},
		{"wallet group", func(p *domain.Policy) { p.SenderType, p.SenderID = domain.SenderWalletGroup, "studio-wallets" }},
		{"recipient", func(p *domain.Policy) { p.ReceiverType, p.ReceiverID = domain.ReceiverRecipient, "partner" }},
		{"recipient group", func(p *domain.Policy) { p.ReceiverType, p.ReceiverID = domain.ReceiverRecipientGroup, "restricted" }},
		{"currency", func(p *domain.Policy) { p.Currency = domain.CurrencyEUR }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			first, fallback := samplePolicy("first"), samplePolicy("fallback")
			first.Action = domain.PolicyReject
			tc.change(&first)
			if got := evaluate(t, []domain.Policy{first, fallback}, sampleInput()); got.PolicyID != fallback.ID {
				t.Fatal("unrelated reject affected transfer", got)
			}
		})
	}
}
func TestGroupsOrderAndEUR(t *testing.T) {
	first, allow := samplePolicy("group-reject"), samplePolicy("allow")
	first.SenderType, first.SenderID = domain.SenderWalletGroup, "daily-wallets"
	first.ReceiverType, first.ReceiverID = domain.ReceiverRecipientGroup, "suppliers"
	first.Action = domain.PolicyReject
	if got := evaluate(t, []domain.Policy{first, allow}, sampleInput()); got.Outcome != domain.OutcomeReject || len(got.Trace) != 1 {
		t.Fatal("first match did not stop", got)
	}
	if got := evaluate(t, []domain.Policy{allow, first}, sampleInput()); got.Outcome != domain.OutcomeAllow || len(got.Trace) != 1 {
		t.Fatal("reordered allow did not stop", got)
	}
	if evaluate(t, nil, sampleInput()).Outcome != domain.OutcomeNoMatch {
		t.Fatal("empty rules allowed transfer")
	}
	request := sampleInput()
	request.WalletID = "europe"
	if evaluate(t, []domain.Policy{allow}, request).Outcome != domain.OutcomeNoMatch {
		t.Fatal("USD rule matched EUR wallet")
	}
	allow.Currency = domain.CurrencyEUR
	if evaluate(t, []domain.Policy{allow}, request).Outcome != domain.OutcomeAllow {
		t.Fatal("EUR rule did not match EUR wallet")
	}
}
func TestInputAndPolicyValidation(t *testing.T) {
	for _, amount := range []string{"0", "-1", "1e3", "1.001"} {
		request := sampleInput()
		request.Amount = amount
		if domain.ValidateTransfer(request, demo.Catalog()) == nil {
			t.Errorf("accepted %q", amount)
		}
	}
	request := sampleInput()
	request.WalletID = "studio-wallet"
	if domain.ValidateTransfer(request, demo.Catalog()) == nil {
		t.Fatal("another profile wallet accepted")
	}
	request = sampleInput()
	request.RecipientID = "studio-supplier"
	if domain.ValidateTransfer(request, demo.Catalog()) == nil {
		t.Fatal("another profile recipient accepted")
	}
	policy := samplePolicy("test")
	policy.Currency = "TOKEN"
	if domain.ValidatePolicy(policy, demo.Catalog()) == nil {
		t.Fatal("unsupported currency accepted")
	}
	policy = samplePolicy("test")
	policy.AmountPolicy = domain.CountLimit("0.001")
	if domain.ValidatePolicy(policy, demo.Catalog()) == nil {
		t.Fatal("fractional cent accepted")
	}
}
func TestTextLimitsAndFrozenDecision(t *testing.T) {
	policy := samplePolicy("text")
	policy.Name = strings.Repeat("\u00e9", 80)
	if err := domain.ValidatePolicy(policy, demo.Catalog()); err != nil {
		t.Fatal(err)
	}
	policy.Name += "a"
	if domain.ValidatePolicy(policy, demo.Catalog()) == nil {
		t.Fatal("long policy name accepted")
	}
	policy = samplePolicy("snapshot")
	decision := evaluate(t, []domain.Policy{policy}, sampleInput())
	transfer := domain.NewTransfer("tx", sampleInput(), domain.CurrencyUSD, decision, time.Now())
	decision.MatchedPolicy.Name = "changed"
	decision.Trace[0].Checks[0].Detail = "changed"
	if transfer.Decision.MatchedPolicy.Name != "snapshot" || transfer.Decision.Trace[0].Checks[0].Detail == "changed" {
		t.Fatal("decision data shared")
	}
}
