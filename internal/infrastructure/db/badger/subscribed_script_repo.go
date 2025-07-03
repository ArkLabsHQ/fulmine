package badgerdb

import (
	"context"
	"errors"
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

type SubscribedScript struct {
	Script string
}

type subscribedScriptRepository struct {
	store *badgerhold.Store
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

func (r *subscribedScriptRepository) Add(ctx context.Context, scripts []string) (count int, err error) {
	count = 0
	for _, script := range scripts {
		err := r.store.Insert(script, SubscribedScript{Script: script})
		if errors.Is(err, badgerhold.ErrKeyExists) {
			continue
		} else if err != nil {
			return count, fmt.Errorf("failed to insert script %s: %w", script, err)
		}
		count++
	}

	return count, nil
}
func (r *subscribedScriptRepository) Get(ctx context.Context) ([]string, error) {
	var currentScripts []SubscribedScript
	err := r.store.Find(&currentScripts, nil)
	if err != nil && !errors.Is(err, badgerhold.ErrNotFound) {
		return nil, fmt.Errorf("failed to get all subscribed scripts: %w", err)
	}

	scripts := make([]string, 0, len(currentScripts))
	for _, script := range currentScripts {
		scripts = append(scripts, script.Script)
	}

	return scripts, nil
}
func (r *subscribedScriptRepository) Delete(ctx context.Context, scripts []string) (count int, err error) {
	for _, script := range scripts {
		err = r.store.Delete(script, SubscribedScript{})
		if errors.Is(err, badgerhold.ErrNotFound) {
			continue
		}
		if err != nil {
			return count, fmt.Errorf("failed to delete script %s: %w", script, err)
		}
		count++
	}
	return count, nil
}

func (s *subscribedScriptRepository) Close() {
	// nolint:all
	s.store.Close()
}
