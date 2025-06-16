package badgerdb

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/dgraph-io/badger/v4"
	"github.com/timshannon/badgerhold/v4"
)

const (
	subscribedScriptKey = "subscribed_scripts"
	subscribedScriptDir = "subscribed_scripts"
)

type subscribedScriptRepository struct {
	store *badgerhold.Store
}

type SubscribedScripts struct {
	Scripts []string `json:"scripts"`
}

func NewSubscribedScriptRepository(baseDir string, logger badger.Logger) (domain.SubscribedScriptRepository, error) {
	var dir string
	if len(baseDir) > 0 {
		dir = filepath.Join(baseDir, subscribedScriptDir)
	}
	store, err := createDB(dir, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open round events store: %s", err)
	}
	return &subscribedScriptRepository{store}, nil
}

func (r *subscribedScriptRepository) Add(ctx context.Context, scripts []string) error {
	var currentScripts SubscribedScripts
	err := r.store.Get(subscribedScriptKey, &currentScripts)
	if err != nil && err != badgerhold.ErrNotFound {
		return fmt.Errorf("failed to get subscribed scripts: %w", err)
	}

	aggregatedScripts := SubscribedScripts{
		Scripts: append(currentScripts.Scripts, scripts...),
	}

	err = r.store.Insert(subscribedScriptKey, aggregatedScripts)
	if err != nil && err == badgerhold.ErrKeyExists {
		// If the key already exists, we can update it
		err = r.store.Update(subscribedScriptKey, aggregatedScripts)
		return err
	}

	return err
}
func (r *subscribedScriptRepository) Get(ctx context.Context) ([]string, error) {
	var currentScripts SubscribedScripts

	err := r.store.Get(subscribedScriptKey, &currentScripts)
	if err != nil && err != badgerhold.ErrNotFound {
		return nil, fmt.Errorf("failed to get subscribed scripts: %w", err)
	}

	return currentScripts.Scripts, nil
}
func (r *subscribedScriptRepository) Delete(ctx context.Context, scripts []string) (count int, err error) {
	var currentScripts SubscribedScripts
	err = r.store.Get(subscribedScriptKey, &currentScripts)

	if err != nil && err != badgerhold.ErrNotFound {
		return 0, fmt.Errorf("failed to get subscribed scripts: %w", err)
	}

	scriptSet := make(map[string]struct{})
	for _, script := range scripts {
		scriptSet[script] = struct{}{}
	}

	var updatedScripts []string
	for _, script := range currentScripts.Scripts {
		if _, exists := scriptSet[script]; !exists {
			updatedScripts = append(updatedScripts, script)
		} else {
			count++
		}

	}

	err = r.store.Update(subscribedScriptKey, SubscribedScripts{
		Scripts: updatedScripts,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to update subscribed scripts: %w", err)
	}
	return count, nil
}

func (s *subscribedScriptRepository) Close() {
	// nolint:all
	s.store.Close()
}
