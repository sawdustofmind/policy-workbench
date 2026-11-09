package domain

import (
	"slices"
	"strings"
	"unicode/utf8"
)

func ValidatePolicy(p Policy, c Catalog) error {
	if !c.HasProfile(p.ProfileID) {
		return InvalidRequest("unknown profile")
	}
	if strings.TrimSpace(p.Name) == "" || utf8.RuneCountInString(p.Name) > 80 {
		return InvalidRequest("policy name must contain 1 to 80 characters")
	}
	if p.InitiatorID != "" && !c.HasMember(p.ProfileID, p.InitiatorID) {
		return InvalidRequest("initiator does not belong to this profile")
	}
	switch p.SenderType {
	case SenderAny:
		if p.SenderID != "" {
			return InvalidRequest("Any sender must not have a sender ID")
		}
	case SenderWallet:
		if _, ok := c.FindWallet(p.ProfileID, p.SenderID); !ok {
			return InvalidRequest("unknown sender wallet in this profile")
		}
	case SenderWalletGroup:
		if !hasGroup(c.WalletGroups, p.ProfileID, p.SenderID) {
			return InvalidRequest("unknown wallet group in this profile")
		}
	default:
		return InvalidRequest("unknown sender type")
	}
	switch p.ReceiverType {
	case ReceiverAny:
		if p.ReceiverID != "" {
			return InvalidRequest("Any receiver must not have a receiver ID")
		}
	case ReceiverRecipient:
		if _, ok := c.FindRecipient(p.ProfileID, p.ReceiverID); !ok {
			return InvalidRequest("unknown recipient in this profile")
		}
	case ReceiverRecipientGroup:
		if !hasGroup(c.RecipientGroups, p.ProfileID, p.ReceiverID) {
			return InvalidRequest("unknown recipient group in this profile")
		}
	default:
		return InvalidRequest("unknown receiver type")
	}
	if !p.Currency.Valid() {
		return InvalidRequest("currency must be USD or EUR")
	}
	if err := validateAmountPolicy(p.AmountPolicy); err != nil {
		return err
	}
	if !slices.Contains([]PolicyAction{PolicyAllow, PolicyReject}, p.Action) {
		return InvalidRequest("action must be ALLOW or REJECT")
	}
	return nil
}

func validateAmountPolicy(rule *AmountPolicy) error {
	if rule == nil {
		return nil
	}
	if _, err := ParseMoney(rule.Amount); err != nil {
		return InvalidRequest("amount limit: %v", err)
	}
	switch rule.AmountType {
	case AmountTypeCount:
		if rule.Interval != nil {
			return InvalidRequest("a per-transfer amount limit must not have an interval")
		}
	case AmountTypeInterval:
		if rule.Interval == nil || !rule.Interval.Valid() {
			return InvalidRequest("interval must be 24h or 30d")
		}
	default:
		return InvalidRequest("amount type must be count or interval")
	}
	return nil
}
