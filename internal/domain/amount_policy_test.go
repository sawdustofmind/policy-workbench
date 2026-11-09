package domain_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"policy-workbench/internal/demo"
	"policy-workbench/internal/domain"
)

func noSpending(domain.LimitInterval) (domain.Decimal, error) { return domain.ZeroMoney(), nil }

func spendingOf(values map[domain.LimitInterval]string) domain.SpendingLookup {
	return func(interval domain.LimitInterval) (domain.Decimal, error) {
		if value, ok := values[interval]; ok {
			return domain.ParseMoney(value)
		}
		return domain.ZeroMoney(), nil
	}
}

func TestAmountPolicyValidation(t *testing.T) {
	interval := domain.Interval24h
	invalid := domain.LimitInterval(7 * 24 * time.Hour)
	for _, tc := range []struct {
		name  string
		rule  *domain.AmountPolicy
		valid bool
	}{
		{"any amount", nil, true},
		{"count", domain.CountLimit("100.50"), true},
		{"count zero", domain.CountLimit("0"), true},
		{"interval 24h", domain.IntervalLimit("10000", domain.Interval24h), true},
		{"interval 30d", domain.IntervalLimit("10000", domain.Interval30d), true},
		{"count with interval", &domain.AmountPolicy{Amount: "1", AmountType: domain.AmountTypeCount, Interval: &interval}, false},
		{"interval without interval", &domain.AmountPolicy{Amount: "1", AmountType: domain.AmountTypeInterval}, false},
		{"unsupported interval", &domain.AmountPolicy{Amount: "1", AmountType: domain.AmountTypeInterval, Interval: &invalid}, false},
		{"blank amount", domain.CountLimit(""), false},
		{"fractional cent", domain.IntervalLimit("0.001", domain.Interval24h), false},
		{"unknown type", &domain.AmountPolicy{Amount: "1", AmountType: "weekly"}, false},
	} {
		policy := samplePolicy("rule")
		policy.AmountPolicy = tc.rule
		if err := domain.ValidatePolicy(policy, demo.Catalog()); (err == nil) != tc.valid {
			t.Errorf("%s: valid=%v, err=%v", tc.name, tc.valid, err)
		}
		policy.Action = domain.PolicyReject
		if err := domain.ValidatePolicy(policy, demo.Catalog()); (err == nil) != tc.valid {
			t.Errorf("%s (REJECT): valid=%v, err=%v", tc.name, tc.valid, err)
		}
	}
}

func TestAmountPolicyJSON(t *testing.T) {
	policy := samplePolicy("json")
	policy.AmountPolicy = domain.IntervalLimit("10000", domain.Interval30d)
	data, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"amount_policy":{"amount":"10000","amount_type":"interval","interval":"30d"}`) {
		t.Fatal("unexpected wire format", string(data))
	}
	var decoded domain.Policy
	if err := json.Unmarshal(data, &decoded); err != nil || *decoded.AmountPolicy.Interval != domain.Interval30d {
		t.Fatal("interval did not round-trip", err)
	}
	if err := json.Unmarshal([]byte(`{"amount":"1","amount_type":"interval","interval":"7d"}`), &domain.AmountPolicy{}); err == nil {
		t.Fatal("unsupported interval label decoded")
	}
	if err := json.Unmarshal([]byte(`{"amount":"1","amount_type":"interval","interval":86400000000000}`), &domain.AmountPolicy{}); err == nil {
		t.Fatal("numeric interval decoded")
	}
	policy.AmountPolicy = nil
	data, _ = json.Marshal(policy)
	if strings.Contains(string(data), "amount_policy") {
		t.Fatal("nil amount policy serialized", string(data))
	}
}

func TestIntervalLimitUsesWalletTotal(t *testing.T) {
	limited, fallback := samplePolicy("limited"), samplePolicy("fallback")
	limited.AmountPolicy = domain.IntervalLimit("10000", domain.Interval24h)
	usage := spendingOf(map[domain.LimitInterval]string{domain.Interval24h: "6000", domain.Interval30d: "9999"})
	for _, tc := range []struct{ amount, expected string }{{"4000", "limited"}, {"4000.00", "limited"}, {"3999.99", "limited"}, {"4000.01", "fallback"}} {
		request := sampleInput()
		request.Amount = tc.amount
		decision, err := domain.EvaluatePolicies([]domain.Policy{limited, fallback}, demo.Catalog(), request, usage)
		if err != nil {
			t.Fatal(err)
		}
		if decision.PolicyID != tc.expected {
			t.Errorf("%s selected %s", tc.amount, decision.PolicyID)
		}
	}
	request := sampleInput()
	request.Amount = "4000"
	decision, _ := domain.EvaluatePolicies([]domain.Policy{limited}, demo.Catalog(), request, usage)
	check := decision.Trace[0].Checks[len(decision.Trace[0].Checks)-1]
	if check.Name != "Interval amount" || check.Detail != "Operations used 6000 + 4000 <= 10000 in last 24h" {
		t.Fatal("unexpected trace", check)
	}
	limited.AmountPolicy = domain.IntervalLimit("10000", domain.Interval30d)
	request.Amount = "2"
	if decision, _ := domain.EvaluatePolicies([]domain.Policy{limited, fallback}, demo.Catalog(), request, usage); decision.PolicyID != "fallback" {
		t.Fatal("30d limit did not use the 30d total")
	}
}

func TestIntervalLookupIsSkippedWithoutIntervalRules(t *testing.T) {
	calls := 0
	usage := func(domain.LimitInterval) (domain.Decimal, error) { calls++; return domain.ZeroMoney(), nil }
	count := samplePolicy("count")
	count.AmountPolicy = domain.CountLimit("1")
	if _, err := domain.EvaluatePolicies([]domain.Policy{count, samplePolicy("any")}, demo.Catalog(), sampleInput(), usage); err != nil || calls != 0 {
		t.Fatal("lookup called for non-interval rules", calls, err)
	}
	first, second := samplePolicy("first"), samplePolicy("second")
	first.AmountPolicy = domain.IntervalLimit("1", domain.Interval24h)
	second.AmountPolicy = domain.IntervalLimit("2", domain.Interval24h)
	if _, err := domain.EvaluatePolicies([]domain.Policy{first, second}, demo.Catalog(), sampleInput(), usage); err != nil || calls != 1 {
		t.Fatal("interval total not reused within one evaluation", calls, err)
	}
}

func TestWalletSpendWindow(t *testing.T) {
	now := time.Date(2026, 1, 12, 12, 0, 0, 0, time.UTC)
	transfer := func(id, wallet, amount string, age time.Duration, status domain.TransferStatus) domain.Transfer {
		return domain.Transfer{ID: id, TransferRequest: domain.TransferRequest{ProfileID: "treasury", WalletID: wallet, Amount: amount}, Currency: domain.CurrencyUSD, Status: status, CreatedAt: now.Add(-age)}
	}
	transfers := []domain.Transfer{
		transfer("exact-24h", "operations", "1", 24*time.Hour, domain.StatusConfirmed),
		transfer("inside-24h", "operations", "10", 23*time.Hour, domain.StatusSent),
		transfer("pending", "operations", "100", time.Minute, domain.StatusPendingSignature),
		transfer("cancelled", "operations", "1000", time.Minute, domain.StatusCancelled),
		transfer("policy-rejected", "operations", "1000", time.Minute, domain.StatusRejectedByPolicy),
		transfer("other-wallet", "reserve", "1000", time.Minute, domain.StatusConfirmed),
		transfer("inside-30d", "operations", "10000", 719*time.Hour, domain.StatusConfirmed),
		transfer("exact-30d", "operations", "100000", 720*time.Hour, domain.StatusConfirmed),
	}
	for interval, expected := range map[domain.LimitInterval]string{domain.Interval24h: "110", domain.Interval30d: "10111"} {
		got, err := domain.WalletSpend(transfers, "treasury", "operations", now, interval.Duration())
		if err != nil || got.String() != expected {
			t.Errorf("%s: got %s, want %s (%v)", interval, got, expected, err)
		}
	}
}
