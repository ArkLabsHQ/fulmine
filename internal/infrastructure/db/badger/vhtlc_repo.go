package badgerdb

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/btcsuite/btcd/btcec/v2"
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
	var opts []vhtlcData
	err := r.store.Find(&opts, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get all vHTLC options: %w", err)
	}

	var vhtlcList []domain.Vhtlc
	for _, opt := range opts {
		vHTLC, err := opt.toVhtlc()
		if err != nil {
			return nil, fmt.Errorf("failed to convert data to opts: %w", err)
		}
		vhtlcList = append(vhtlcList, vHTLC)
	}
	return vhtlcList, nil
}

// Get retrieves a specific VHTLC option by preimage hash
func (r *vhtlcRepository) Get(ctx context.Context, preimageHash string) (*domain.Vhtlc, error) {
	var dataOpts vhtlcData
	err := r.store.Get(preimageHash, &dataOpts)
	if errors.Is(err, badgerhold.ErrNotFound) {
		return nil, fmt.Errorf("vHTLC with preimage hash %s not found", preimageHash)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get vHTLC option: %w", err)
	}

	vHTLC, err := dataOpts.toVhtlc()
	if err != nil {
		return nil, fmt.Errorf("failed to convert data to opts: %w", err)
	}

	return &vHTLC, nil
}

// Add stores a new VHTLC option in the database
func (r *vhtlcRepository) Add(ctx context.Context, vhtlc domain.Vhtlc) error {
	data := vhtlcData{
		Id:                                   vhtlc.Id,
		PreimageHash:                         hex.EncodeToString(vhtlc.PreimageHash),
		Sender:                               hex.EncodeToString(vhtlc.Sender.SerializeCompressed()),
		Receiver:                             hex.EncodeToString(vhtlc.Receiver.SerializeCompressed()),
		Server:                               hex.EncodeToString(vhtlc.Server.SerializeCompressed()),
		RefundLocktime:                       vhtlc.RefundLocktime,
		UnilateralClaimDelay:                 vhtlc.UnilateralClaimDelay,
		UnilateralRefundDelay:                vhtlc.UnilateralRefundDelay,
		UnilateralRefundWithoutReceiverDelay: vhtlc.UnilateralRefundWithoutReceiverDelay,
	}

	if err := r.store.Insert(data.Id, data); err != nil {
		if errors.Is(err, badgerhold.ErrKeyExists) {
			return fmt.Errorf("vHTLC with id %s already exists", data.Id)
		}
		return err
	}
	return nil
}

func (r *vhtlcRepository) GetByScripts(ctx context.Context, scripts []string) ([]domain.Vhtlc, error) {
	if len(scripts) == 0 {
		return nil, nil
	}

	scriptSet := make(map[string]struct{}, len(scripts))
	for _, script := range scripts {
		scriptSet[script] = struct{}{}
	}

	vhtlcs, err := r.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get all VHTLCs: %w", err)
	}

	out := make([]domain.Vhtlc, 0, len(scripts))
	for _, v := range vhtlcs {
		lockingScript, err := getVhtlcLockingScript(v)
		if err != nil {
			continue
		}
		if _, ok := scriptSet[lockingScript]; ok {
			out = append(out, v)
		}
	}
	return out, nil
}

// GetScripts returns Taproot locking scripts for all VHTLCs.
func (r *vhtlcRepository) GetScripts(ctx context.Context) ([]string, error) {
	vhtlcs, err := r.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get all VHTLCs: %w", err)
	}

	allScripts := make([]string, 0)
	for _, v := range vhtlcs {
		lockingScript, err := getVhtlcLockingScript(v)
		if err != nil {
			continue
		}
		allScripts = append(allScripts, lockingScript)
	}

	return allScripts, nil
}

func getVhtlcLockingScript(v domain.Vhtlc) (string, error) {
	return vhtlc.LockingScriptHexFromOpts(v.Opts)
}

func (s *vhtlcRepository) Close() {
	// nolint:all
	s.store.Close()
}

type vhtlcData struct {
	Id                                   string
	PreimageHash                         string
	Sender                               string
	Receiver                             string
	Server                               string
	RefundLocktime                       arklib.AbsoluteLocktime
	UnilateralClaimDelay                 arklib.RelativeLocktime
	UnilateralRefundDelay                arklib.RelativeLocktime
	UnilateralRefundWithoutReceiverDelay arklib.RelativeLocktime
}

func (d *vhtlcData) toVhtlc() (domain.Vhtlc, error) {
	senderBytes, err := hex.DecodeString(d.Sender)
	if err != nil {
		return domain.Vhtlc{}, err
	}
	receiverBytes, err := hex.DecodeString(d.Receiver)
	if err != nil {
		return domain.Vhtlc{}, err
	}
	serverBytes, err := hex.DecodeString(d.Server)
	if err != nil {
		return domain.Vhtlc{}, err
	}

	sender, err := btcec.ParsePubKey(senderBytes)
	if err != nil {
		return domain.Vhtlc{}, err
	}
	receiver, err := btcec.ParsePubKey(receiverBytes)
	if err != nil {
		return domain.Vhtlc{}, err
	}
	server, err := btcec.ParsePubKey(serverBytes)
	if err != nil {
		return domain.Vhtlc{}, err
	}

	preimageHashBytes, err := hex.DecodeString(d.PreimageHash)
	if err != nil {
		return domain.Vhtlc{}, err
	}

	opts := vhtlc.Opts{
		Sender:                               sender,
		Receiver:                             receiver,
		Server:                               server,
		RefundLocktime:                       d.RefundLocktime,
		UnilateralClaimDelay:                 d.UnilateralClaimDelay,
		UnilateralRefundDelay:                d.UnilateralRefundDelay,
		UnilateralRefundWithoutReceiverDelay: d.UnilateralRefundWithoutReceiverDelay,
		PreimageHash:                         preimageHashBytes,
	}

	return domain.NewVhtlc(opts), nil
}
