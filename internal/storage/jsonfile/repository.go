package jsonfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"policy-workbench/internal/application"
)

type Repository struct {
	mu    sync.RWMutex
	state application.WorkspaceState
	path  string
}

var _ application.Repository = (*Repository)(nil)

func Open(path string, initial application.WorkspaceState) (*Repository, error) {
	repository := &Repository{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		repository.state = initial.Clone()
		return repository, repository.save(repository.state)
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &repository.state); err != nil {
		return nil, fmt.Errorf("read saved state: %w", err)
	}
	state := repository.state
	if state.Version != application.StateVersion || state.SimulationTime.IsZero() || state.Policies == nil || state.Transfers == nil {
		return nil, fmt.Errorf("unsupported or incomplete state file; use a new -data path")
	}
	return repository, nil
}

func (repository *Repository) Snapshot() application.WorkspaceState {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	return repository.state.Clone()
}

func (repository *Repository) Update(change func(*application.WorkspaceState) error) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	next := repository.state.Clone()
	if err := change(&next); err != nil {
		return err
	}
	if err := repository.save(next); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	repository.state = next.Clone()
	return nil
}

func (repository *Repository) save(state application.WorkspaceState) error {
	if err := os.MkdirAll(filepath.Dir(repository.path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(repository.path), ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), repository.path)
}
