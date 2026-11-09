package application

import (
	"fmt"
	"slices"
	"time"

	"policy-workbench/internal/domain"
)

// spendingKey identifies one rolling accumulator: a source wallet's total over
// one interval. Profile and currency are implied by the wallet but kept in the
// key so it is self-describing and safe to move to an external store.
//
//	spend:{profile_id}:{wallet_id}:{currency}:{interval}  e.g. spend:treasury:operations:USD:24h
func spendingKey(profileID, walletID string, currency domain.Currency, interval domain.LimitInterval) string {
	return fmt.Sprintf("spend:%s:%s:%s:%s", profileID, walletID, currency, interval)
}

type spendEntry struct {
	transferID string
	createdAt  time.Time
	amount     domain.Decimal
}

// spendWindow holds entries in creation order with their running total.
type spendWindow struct {
	window  time.Duration
	entries []spendEntry
	total   domain.Decimal
}

// spendingCache is a derived, in-memory index of wallet totals per interval.
// Persisted transfers remain the source of truth; Rebuild recreates it.
// Callers serialize access (Service.mu).
type spendingCache struct {
	windows map[string]*spendWindow
}

func newSpendingCache(transfers []domain.Transfer) (*spendingCache, error) {
	cache := &spendingCache{}
	return cache, cache.Rebuild(transfers)
}

func (cache *spendingCache) Rebuild(transfers []domain.Transfer) error {
	cache.windows = map[string]*spendWindow{}
	ordered := slices.Clone(transfers)
	slices.SortStableFunc(ordered, func(a, b domain.Transfer) int { return a.CreatedAt.Compare(b.CreatedAt) })
	for _, transfer := range ordered {
		if err := cache.Add(transfer); err != nil {
			return err
		}
	}
	return nil
}

// Used returns the wallet's total within the interval ending at now, evicting
// expired entries. The simulation clock only moves forward between rebuilds.
func (cache *spendingCache) Used(profileID, walletID string, currency domain.Currency, interval domain.LimitInterval, now time.Time) domain.Decimal {
	window, ok := cache.windows[spendingKey(profileID, walletID, currency, interval)]
	if !ok {
		return domain.ZeroMoney()
	}
	expired := 0
	for expired < len(window.entries) && !domain.InSpendingWindow(window.entries[expired].createdAt, now, window.window) {
		window.total = window.total.Subtract(window.entries[expired].amount)
		expired++
	}
	window.entries = window.entries[expired:]
	return window.total
}

// Add records a fund-holding transfer in every interval of its wallet.
func (cache *spendingCache) Add(transfer domain.Transfer) error {
	if !domain.ConsumesFunds(transfer.Status) {
		return nil
	}
	amount, err := domain.ParseMoney(transfer.Amount)
	if err != nil {
		return fmt.Errorf("invalid amount in transfer %s: %w", transfer.ID, err)
	}
	for _, interval := range domain.LimitIntervals {
		key := spendingKey(transfer.ProfileID, transfer.WalletID, transfer.Currency, interval)
		window, ok := cache.windows[key]
		if !ok {
			window = &spendWindow{window: interval.Duration(), total: domain.ZeroMoney()}
			cache.windows[key] = window
		}
		entry := spendEntry{transfer.ID, transfer.CreatedAt, amount}
		position := len(window.entries)
		for position > 0 && window.entries[position-1].createdAt.After(entry.createdAt) {
			position--
		}
		window.entries = slices.Insert(window.entries, position, entry)
		window.total = window.total.Add(amount)
	}
	return nil
}

// Remove releases a transfer that no longer holds funds.
func (cache *spendingCache) Remove(transfer domain.Transfer) {
	for _, interval := range domain.LimitIntervals {
		window, ok := cache.windows[spendingKey(transfer.ProfileID, transfer.WalletID, transfer.Currency, interval)]
		if !ok {
			continue
		}
		index := slices.IndexFunc(window.entries, func(entry spendEntry) bool { return entry.transferID == transfer.ID })
		if index < 0 {
			continue
		}
		window.total = window.total.Subtract(window.entries[index].amount)
		window.entries = slices.Delete(window.entries, index, index+1)
	}
}
