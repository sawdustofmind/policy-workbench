package domain

import (
	"fmt"
	"slices"
)

// EvaluatePolicies selects the first matching policy. usage supplies the
// source wallet's rolling totals for interval amount limits.
func EvaluatePolicies(policies []Policy, catalog Catalog, request TransferRequest, usage SpendingLookup) (PolicyDecision, error) {
	if err := ValidateTransfer(request, catalog); err != nil {
		return PolicyDecision{}, err
	}
	wallet, _ := catalog.FindWallet(request.ProfileID, request.WalletID)
	amount, err := ParseMoney(request.Amount)
	if err != nil {
		return PolicyDecision{}, err
	}
	used := map[LimitInterval]Decimal{}
	spent := func(interval LimitInterval) (Decimal, error) {
		if total, ok := used[interval]; ok {
			return total, nil
		}
		if usage == nil {
			return Decimal{}, fmt.Errorf("no spending data for interval limits")
		}
		total, err := usage(interval)
		if err != nil {
			return Decimal{}, err
		}
		used[interval] = total
		return total, nil
	}
	decision := PolicyDecision{Outcome: OutcomeNoMatch, Trace: []RuleEvaluation{}}
	for _, policy := range policies {
		if policy.ProfileID != request.ProfileID {
			continue
		}
		sender := policy.SenderType == SenderAny || policy.SenderType == SenderWallet && policy.SenderID == request.WalletID ||
			policy.SenderType == SenderWalletGroup && groupContains(catalog.WalletGroups, request.ProfileID, policy.SenderID, request.WalletID)
		receiver := policy.ReceiverType == ReceiverAny || policy.ReceiverType == ReceiverRecipient && policy.ReceiverID == request.RecipientID ||
			policy.ReceiverType == ReceiverRecipientGroup && groupContains(catalog.RecipientGroups, request.ProfileID, policy.ReceiverID, request.RecipientID)
		checks := []ConditionResult{
			{"Initiator", policy.InitiatorID == "" || policy.InitiatorID == request.InitiatorID, "Any or selected profile member"},
			{"Sender", sender, "Wallet or wallet-group membership"},
			{"Receiver", receiver, "Recipient or recipient-group membership"},
			{"Currency", policy.Currency == wallet.Currency, string(wallet.Currency) + " compared with " + string(policy.Currency)},
		}
		check, err := amountCheck(policy.AmountPolicy, amount, wallet, spent)
		if err != nil {
			return PolicyDecision{}, err
		}
		checks = append(checks, check)
		matched := !slices.ContainsFunc(checks, func(check ConditionResult) bool { return !check.Matched })
		decision.Trace = append(decision.Trace, RuleEvaluation{policy.ID, policy.Name, matched, checks})
		if matched {
			decision.PolicyID, decision.PolicyName, decision.Outcome = policy.ID, policy.Name, DecisionOutcome(policy.Action)
			matchedPolicy := policy.Clone()
			decision.MatchedPolicy = &matchedPolicy
			return decision, nil
		}
	}
	return decision, nil
}

func amountCheck(rule *AmountPolicy, amount Decimal, wallet Wallet, spent func(LimitInterval) (Decimal, error)) (ConditionResult, error) {
	if rule == nil {
		return ConditionResult{"Amount", true, "No amount restriction"}, nil
	}
	limit, err := ParseMoney(rule.Amount)
	if err != nil {
		return ConditionResult{}, InvalidRequest("amount limit: %v", err)
	}
	switch rule.AmountType {
	case AmountTypeCount:
		return ConditionResult{"Amount", amount.Compare(limit) <= 0, amount.String() + " <= " + rule.Amount}, nil
	case AmountTypeInterval:
		if rule.Interval == nil || !rule.Interval.Valid() {
			return ConditionResult{}, InvalidRequest("interval must be 24h or 30d")
		}
		used, err := spent(*rule.Interval)
		if err != nil {
			return ConditionResult{}, err
		}
		detail := fmt.Sprintf("%s used %s + %s <= %s in last %s", wallet.Name, used, amount, rule.Amount, rule.Interval)
		return ConditionResult{"Interval amount", used.Add(amount).Compare(limit) <= 0, detail}, nil
	}
	return ConditionResult{}, InvalidRequest("amount type must be count or interval")
}
