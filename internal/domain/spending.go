package domain

import (
	"fmt"
	"math/big"
	"time"
)

// ZeroMoney returns an exact zero amount.
func ZeroMoney() Decimal { return Decimal{new(big.Int), 0} }

// ConsumesFunds reports whether a transfer in this status holds or has spent
// wallet funds. The same statuses count toward interval amount limits.
func ConsumesFunds(status TransferStatus) bool {
	return status == StatusPendingSignature || status == StatusSent || status == StatusConfirmed
}

// InSpendingWindow reports whether a transfer created at createdAt is inside
// the rolling window (now-window, now].
func InSpendingWindow(createdAt, now time.Time, window time.Duration) bool {
	return createdAt.After(now.Add(-window))
}

// SpendingLookup returns how much the transfer's source wallet has used within
// the given rolling interval, before the transfer being evaluated.
type SpendingLookup func(LimitInterval) (Decimal, error)

// WalletSpend is the reference calculation of a wallet's rolling total. The
// application keeps an incremental cache that must agree with it.
func WalletSpend(transfers []Transfer, profileID, walletID string, now time.Time, window time.Duration) (Decimal, error) {
	total := ZeroMoney()
	for _, transfer := range transfers {
		if transfer.ProfileID != profileID || transfer.WalletID != walletID || !ConsumesFunds(transfer.Status) || !InSpendingWindow(transfer.CreatedAt, now, window) {
			continue
		}
		amount, err := ParseMoney(transfer.Amount)
		if err != nil {
			return Decimal{}, fmt.Errorf("invalid amount in transfer %s: %w", transfer.ID, err)
		}
		total = total.Add(amount)
	}
	return total, nil
}
