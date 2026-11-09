package application

import (
	"fmt"
	"time"

	"policy-workbench/internal/domain"
)

// StateVersion 4 stores amount conditions as a nested amount_policy.
const StateVersion = 4

// WorkspaceState is the single persisted snapshot of this local simulation.
type WorkspaceState struct {
	Version        int               `json:"version"`
	SimulationTime time.Time         `json:"now"`
	IDSequence     int               `json:"next_id"`
	Policies       []domain.Policy   `json:"policies"`
	Transfers      []domain.Transfer `json:"transfers"`
}

func (state *WorkspaceState) AllocateID(prefix string) string {
	state.IDSequence++
	return fmt.Sprintf("%s-%04d", prefix, state.IDSequence)
}

func (state WorkspaceState) Clone() WorkspaceState {
	policies := make([]domain.Policy, len(state.Policies))
	for index, policy := range state.Policies {
		policies[index] = policy.Clone()
	}
	transfers := make([]domain.Transfer, len(state.Transfers))
	for index, transfer := range state.Transfers {
		transfers[index] = transfer.Clone()
	}
	state.Policies, state.Transfers = policies, transfers
	return state
}

// Repository serializes the complete change and publishes it only after saving.
type Repository interface {
	Snapshot() WorkspaceState
	Update(change func(*WorkspaceState) error) error
}

type WorkspaceView struct {
	SimulationTime time.Time              `json:"now"`
	ProfileID      string                 `json:"profile_id"`
	Wallets        []domain.WalletBalance `json:"wallets"`
	Catalog        domain.Catalog         `json:"catalog"`
	Policies       []domain.Policy        `json:"policies"`
	Transfers      []domain.Transfer      `json:"transfers"`
}

type ReorderPoliciesCommand struct {
	ProfileID string   `json:"profile_id"`
	IDs       []string `json:"ids"`
}

type TransferActionCommand struct {
	ProfileID string                `json:"profile_id"`
	Action    domain.TransferAction `json:"action"`
}
