package jsonfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"policy-workbench/internal/application"
	"policy-workbench/internal/domain"
)

func initialState() application.WorkspaceState {
	return application.WorkspaceState{Version: application.StateVersion, SimulationTime: time.Date(2026, 1, 12, 12, 0, 0, 0, time.UTC), Policies: []domain.Policy{}, Transfers: []domain.Transfer{}}
}

func TestSaveFailureDoesNotPublishState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	repository, err := Open(path, initialState())
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(repository.Snapshot())
	original, _ := os.ReadFile(path)
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	repository.path = filepath.Join(blocked, "state.json")
	err = repository.Update(func(state *application.WorkspaceState) error { state.AllocateID("tx"); return nil })
	if err == nil {
		t.Fatal("expected persistence failure")
	}
	after, _ := json.Marshal(repository.Snapshot())
	current, _ := os.ReadFile(path)
	if string(before) != string(after) || string(original) != string(current) {
		t.Fatal("failed save changed state")
	}
}

func TestSnapshotsAndCallbackValuesCannotMutateRepository(t *testing.T) {
	repository, err := Open(filepath.Join(t.TempDir(), "state.json"), initialState())
	if err != nil {
		t.Fatal(err)
	}
	var retained *application.WorkspaceState
	err = repository.Update(func(state *application.WorkspaceState) error {
		policy := domain.Policy{ID: "example", Name: "Original"}
		state.Policies = append(state.Policies, policy)
		decision := domain.PolicyDecision{MatchedPolicy: &policy, Trace: []domain.RuleEvaluation{{Checks: []domain.ConditionResult{{Name: "Original"}}}}}
		state.Transfers = append(state.Transfers, domain.NewTransfer("tx", domain.TransferRequest{}, domain.CurrencyUSD, decision, state.SimulationTime))
		retained = state
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	retained.Policies[0].Name = "Changed callback"
	snapshot := repository.Snapshot()
	snapshot.Policies[0].Name = "Changed snapshot policy"
	snapshot.Transfers[0].Decision.MatchedPolicy.Name = "Changed snapshot"
	snapshot.Transfers[0].Decision.Trace[0].Checks[0].Name = "Changed condition"
	current := repository.Snapshot()
	if current.Policies[0].Name != "Original" || current.Transfers[0].Decision.MatchedPolicy.Name != "Original" || current.Transfers[0].Decision.Trace[0].Checks[0].Name != "Original" {
		t.Fatal("mutable data escaped the repository boundary")
	}
}

func TestConcurrentUpdatesAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	repository, err := Open(path, initialState())
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for index := 0; index < 16; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if err := repository.Update(func(state *application.WorkspaceState) error { state.AllocateID("tx"); return nil }); err != nil {
				t.Error(err)
			}
		}()
	}
	workers.Wait()
	loaded, err := Open(path, initialState())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Snapshot().IDSequence != 16 {
		t.Fatal("concurrent changes were lost")
	}
}

func TestCorruptStateIsNotReseeded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	_ = os.WriteFile(path, []byte("not json"), 0600)
	if _, err := Open(path, initialState()); err == nil {
		t.Fatal("corrupt file silently reseeded")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "not json" {
		t.Fatal("corrupt file overwritten")
	}
}

func TestPreviousStateVersionIsNotSilentlyReinterpreted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	for _, version := range []int{2, 3} {
		previous := initialState()
		previous.Version = version
		data, err := json.Marshal(previous)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(path, initialState()); err == nil {
			t.Fatal("previous state version accepted", version)
		}
		current, err := os.ReadFile(path)
		if err != nil || string(current) != string(data) {
			t.Fatal("unsupported state overwritten")
		}
	}
}
