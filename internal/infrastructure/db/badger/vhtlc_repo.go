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

type vhtlcRepository struct {
	store *badgerhold.Store
}

func NewVHTLCRepository(baseDir string, logger badger.Logger) (domain.VHTLCRepository, error) {
	var dir string
	if len(baseDir) > 0 {
		dir = filepath.Join(baseDir, "vhtlc")
	}
	store, err := createDB(dir, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open vHTLC store: %s", err)
	}
	return &vhtlcRepository{store}, nil
}

// GetAll retrieves all VHTLC options from the database
func (r *vhtlcRepository) GetAll(ctx context.Context) ([]domain.Vhtlc, error) {
	var vhtlcList []domain.Vhtlc
	if err := r.store.Find(&vhtlcList, nil); err != nil {
		return nil, fmt.Errorf("failed to get all vHTLC options: %w", err)
	}

	return vhtlcList, nil
}

func (r *vhtlcRepository) GetByIds(ctx context.Context, ids []string) ([]domain.Vhtlc, error) {
	if len(ids) == 0 {
		return []domain.Vhtlc{}, nil
	}

	var out []domain.Vhtlc
	if err := r.store.Find(&out, badgerhold.Where("Id").In(badgerhold.Slice(ids)...)); err != nil {
		return nil, fmt.Errorf("failed to get vHTLCs by ids: %w", err)
	}

	return out, nil
}

// Get retrieves a specific VHTLC by its id.
func (r *vhtlcRepository) Get(ctx context.Context, id string) (*domain.Vhtlc, error) {
	var vhtlc domain.Vhtlc
	if err := r.store.Get(id, &vhtlc); err != nil {
		if errors.Is(err, badgerhold.ErrNotFound) {
			return nil, fmt.Errorf("vHTLC with id %s not found", id)
		}
		return nil, fmt.Errorf("failed to get vHTLC option: %w", err)
	}

	return &vhtlc, nil
}

// Add stores a new VHTLC option in the database
func (r *vhtlcRepository) Add(ctx context.Context, vhtlc domain.Vhtlc) error {
	if err := r.store.Insert(vhtlc.Id, vhtlc); err != nil {
		if errors.Is(err, badgerhold.ErrKeyExists) {
			return fmt.Errorf("vHTLC with id %s already exists", vhtlc.Id)
		}
		return err
	}
	return nil
}

func (r *vhtlcRepository) HasLegacy(ctx context.Context) (bool, error) { return false, nil }

func (r *vhtlcRepository) GetLegacy(ctx context.Context) ([]domain.LegacyVhtlc, error) {
	return nil, nil
}

func (r *vhtlcRepository) DropLegacy(ctx context.Context) error { return nil }

func (s *vhtlcRepository) Close() {
	// nolint:all
	s.store.Close()
}
