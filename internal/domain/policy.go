package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

type PolicyAction string

const (
	PolicyAllow  PolicyAction = "ALLOW"
	PolicyReject PolicyAction = "REJECT"
)

type SenderType string

const (
	SenderAny         SenderType = "any"
	SenderWallet      SenderType = "wallet"
	SenderWalletGroup SenderType = "wallet_group"
)

type ReceiverType string

const (
	ReceiverAny            ReceiverType = "any"
	ReceiverRecipient      ReceiverType = "recipient"
	ReceiverRecipientGroup ReceiverType = "recipient_group"
)

type Policy struct {
	ID           string        `json:"id"`
	ProfileID    string        `json:"profile_id"`
	Name         string        `json:"name"`
	InitiatorID  string        `json:"initiator_id"`
	SenderType   SenderType    `json:"sender_type"`
	SenderID     string        `json:"sender_id"`
	ReceiverType ReceiverType  `json:"receiver_type"`
	ReceiverID   string        `json:"receiver_id"`
	AmountPolicy *AmountPolicy `json:"amount_policy,omitempty"` // nil means any amount.
	Currency     Currency      `json:"currency"`
	Action       PolicyAction  `json:"action"`
	CreatedAt    time.Time     `json:"created_at"`
}

// Clone returns a copy that shares no pointers with the original.
func (p Policy) Clone() Policy {
	if p.AmountPolicy != nil {
		amount := *p.AmountPolicy
		if amount.Interval != nil {
			interval := *amount.Interval
			amount.Interval = &interval
		}
		p.AmountPolicy = &amount
	}
	return p
}

type AmountType string

const (
	// AmountTypeCount limits each transfer on its own.
	AmountTypeCount AmountType = "count"
	// AmountTypeInterval limits the source wallet's total over a rolling interval.
	AmountTypeInterval AmountType = "interval"
)

// AmountPolicy is the single amount condition of a policy: either a
// per-transfer limit or a rolling per-wallet total, never both.
type AmountPolicy struct {
	Amount     string         `json:"amount"` // Inclusive upper bound, exact decimal.
	AmountType AmountType     `json:"amount_type"`
	Interval   *LimitInterval `json:"interval,omitempty"` // Required for interval, absent for count.
}

func CountLimit(amount string) *AmountPolicy {
	return &AmountPolicy{Amount: amount, AmountType: AmountTypeCount}
}

func IntervalLimit(amount string, interval LimitInterval) *AmountPolicy {
	return &AmountPolicy{Amount: amount, AmountType: AmountTypeInterval, Interval: &interval}
}

// LimitInterval is a rolling window length. Only 24h and 30d are supported;
// JSON uses those labels instead of nanoseconds.
type LimitInterval time.Duration

const (
	Interval24h = LimitInterval(24 * time.Hour)
	Interval30d = LimitInterval(30 * 24 * time.Hour)
)

var LimitIntervals = []LimitInterval{Interval24h, Interval30d}

func (i LimitInterval) Duration() time.Duration { return time.Duration(i) }

func (i LimitInterval) Valid() bool { return i == Interval24h || i == Interval30d }

func (i LimitInterval) String() string {
	switch i {
	case Interval24h:
		return "24h"
	case Interval30d:
		return "30d"
	}
	return time.Duration(i).String()
}

func (i LimitInterval) MarshalJSON() ([]byte, error) {
	if !i.Valid() {
		return nil, fmt.Errorf("unsupported interval %s", time.Duration(i))
	}
	return json.Marshal(i.String())
}

func (i *LimitInterval) UnmarshalJSON(data []byte) error {
	var label string
	if err := json.Unmarshal(data, &label); err != nil {
		return fmt.Errorf("interval must be \"24h\" or \"30d\"")
	}
	switch label {
	case "24h":
		*i = Interval24h
	case "30d":
		*i = Interval30d
	default:
		return fmt.Errorf("interval must be \"24h\" or \"30d\"")
	}
	return nil
}
