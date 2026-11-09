package domain

import "fmt"

type WalletBalance struct {
	Wallet
	Balance   string `json:"balance"`
	Reserved  string `json:"reserved"`
	Available string `json:"available"`
	Spent24h  string `json:"spent_24h"` // Rolling totals used by interval amount limits.
	Spent30d  string `json:"spent_30d"`
}

func CalculateWalletBalance(wallet Wallet, transfers []Transfer) (WalletBalance, error) {
	opening, err := ParseMoney(wallet.OpeningBalance)
	if err != nil {
		return WalletBalance{}, fmt.Errorf("invalid opening balance for %s: %w", wallet.ID, err)
	}
	spent, _ := ParseMoney("0")
	reserved, _ := ParseMoney("0")
	for _, transfer := range transfers {
		if transfer.ProfileID != wallet.ProfileID || transfer.WalletID != wallet.ID {
			continue
		}
		if !ConsumesFunds(transfer.Status) {
			continue
		}
		if transfer.Currency != wallet.Currency {
			return WalletBalance{}, fmt.Errorf("transfer %s currency differs from its wallet", transfer.ID)
		}
		amount, err := ParseMoney(transfer.Amount)
		if err != nil {
			return WalletBalance{}, fmt.Errorf("invalid amount in transfer %s: %w", transfer.ID, err)
		}
		if transfer.Status == StatusConfirmed {
			spent = spent.Add(amount)
		} else {
			reserved = reserved.Add(amount)
		}
	}
	if spent.Add(reserved).Compare(opening) > 0 {
		return WalletBalance{}, fmt.Errorf("transfers exceed opening balance for %s", wallet.ID)
	}
	balance := opening.Subtract(spent)
	return WalletBalance{Wallet: wallet, Balance: balance.String(), Reserved: reserved.String(), Available: balance.Subtract(reserved).String()}, nil
}
