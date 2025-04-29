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
	vtxoRolloverDir = "vtxo_rollover"
)

type vtxoRolloverRepository struct {
	store *badgerhold.Store
}

func NewVtxoRolloverRepository(
	baseDir string, logger badger.Logger,
) (domain.VtxoRolloverRepository, error) {
	var dir string
	if len(baseDir) > 0 {
		dir = filepath.Join(baseDir, vtxoRolloverDir)
	}
	store, err := createDB(dir, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open vtxo rollover store: %s", err)
	}
	return &vtxoRolloverRepository{store}, nil
}

func (s *vtxoRolloverRepository) AddTarget(
	ctx context.Context, target domain.VtxoRolloverTarget,
) error {
	if ctx.Value("tx") != nil {
		tx := ctx.Value("tx").(*badger.Txn)
		if err := s.store.TxInsert(tx, target.Address, target); err != nil {
			if errors.Is(err, badgerhold.ErrKeyExists) {
				return nil
			}
			return err
		}
	}
	if err := s.store.Insert(target.Address, target); err != nil {
		if errors.Is(err, badgerhold.ErrKeyExists) {
			return nil
		}
		return err
	}

	return nil
}

func (s *vtxoRolloverRepository) GetTarget(
	ctx context.Context, address string,
) (*domain.VtxoRolloverTarget, error) {
	var target domain.VtxoRolloverTarget
	var err error

	if ctx.Value("tx") != nil {
		tx := ctx.Value("tx").(*badger.Txn)
		err = s.store.TxGet(tx, address, &target)
	} else {
		err = s.store.Get(address, &target)
	}

	if err != nil {
		if err == badgerhold.ErrNotFound {
			return nil, fmt.Errorf("rollover target not found for address: %s", address)
		}
		return nil, err
	}

	return &target, nil
}

func (s *vtxoRolloverRepository) GetAllTargets(
	ctx context.Context,
) ([]domain.VtxoRolloverTarget, error) {
	var targets []domain.VtxoRolloverTarget
	var err error

	if ctx.Value("tx") != nil {
		tx := ctx.Value("tx").(*badger.Txn)
		err = s.store.TxFind(tx, &targets, nil)
	} else {
		err = s.store.Find(&targets, nil)
	}

	if err != nil {
		return nil, err
	}

	return targets, nil
}

func (s *vtxoRolloverRepository) DeleteTarget(
	ctx context.Context, address string,
) error {
	var target domain.VtxoRolloverTarget

	if ctx.Value("tx") != nil {
		tx := ctx.Value("tx").(*badger.Txn)
		err := s.store.TxGet(tx, address, &target)
		if err != nil {
			if errors.Is(err, badgerhold.ErrNotFound) {
				return nil // Already removed, no error
			}
			return err
		}
		return s.store.TxDelete(tx, address, target)
	}

	err := s.store.Get(address, &target)
	if err != nil {
		if errors.Is(err, badgerhold.ErrNotFound) {
			return nil
		}
		return err
	}

	return s.store.Delete(address, target)
}

func (s *vtxoRolloverRepository) Close() {
	// nolint:all
	s.store.Close()
}
