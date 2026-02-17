package sqlitedb

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/db/sqlite/sqlc/queries"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/btcsuite/btcd/btcec/v2"
	log "github.com/sirupsen/logrus"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

type vhtlcRepository struct {
	db      *sql.DB
	querier *queries.Queries
}

func NewVHTLCRepository(db *sql.DB) (domain.VHTLCRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("cannot open vhtlc repository: db is nil")
	}
	return &vhtlcRepository{db: db, querier: queries.New(db)}, nil
}

func (r *vhtlcRepository) Add(ctx context.Context, vhtlc domain.Vhtlc) error {
	optsParams := toVhtlcRow(vhtlc)
	if _, err := r.Get(ctx, optsParams.ID); err == nil {
		return fmt.Errorf("vHTLC with ID %s already exists", optsParams.ID)
	}

	if err := r.querier.InsertVHTLC(ctx, optsParams); err != nil {
		if sqlErr, ok := err.(*sqlite.Error); ok {
			if sqlErr.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY {
				return fmt.Errorf("vHTLC with ID %s already exists", optsParams.ID)
			}
		}
		return err
	}
	return nil
}

func (r *vhtlcRepository) Get(ctx context.Context, id string) (*domain.Vhtlc, error) {
	row, err := r.querier.GetVHTLC(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("vHTLC with id %s not found", id)

		}
		return nil, err
	}

	vhtlc, err := toVhtlc(row)
	if err != nil {
		return nil, err
	}

	return &vhtlc, nil
}

func (r *vhtlcRepository) GetAll(ctx context.Context) ([]domain.Vhtlc, error) {
	rows, err := r.querier.ListVHTLC(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Vhtlc, 0, len(rows))
	for _, row := range rows {
		vhtlc, err := toVhtlc(row)
		if err != nil {
			return nil, err
		}
		out = append(out, vhtlc)
	}
	return out, nil
}

func (r *vhtlcRepository) GetByScripts(ctx context.Context, scripts []string) ([]domain.Vhtlc, error) {
	if len(scripts) == 0 {
		return nil, nil
	}

	rows, err := r.querier.ListVHTLC(ctx)
	if err != nil {
		return nil, err
	}

	scriptSet := make(map[string]struct{}, len(scripts))
	for _, script := range scripts {
		scriptSet[script] = struct{}{}
	}

	out := make([]domain.Vhtlc, 0, len(scripts))
	for _, row := range rows {
		if _, ok := scriptSet[row.Script]; !ok {
			continue
		}
		v, err := toVhtlc(row)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}

	return out, nil
}

func (r *vhtlcRepository) GetScripts(ctx context.Context) ([]string, error) {
	vhtlcs, err := r.querier.ListVHTLC(ctx)
	if err != nil {
		return nil, err
	}

	allScripts := make([]string, 0, len(vhtlcs))
	for _, vhtlc := range vhtlcs {
		if vhtlc.Script == "" || vhtlc.Script == "[]" {
			continue
		}
		allScripts = append(allScripts, vhtlc.Script)
	}

	return allScripts, nil
}

func (r *vhtlcRepository) Close() {
	if r.db != nil {
		r.db.Close()
	}
}

func toVhtlc(row queries.Vhtlc) (domain.Vhtlc, error) {
	senderBytes, err := hex.DecodeString(row.Sender)
	if err != nil {
		return domain.Vhtlc{}, err
	}
	receiverBytes, err := hex.DecodeString(row.Receiver)
	if err != nil {
		return domain.Vhtlc{}, err
	}
	serverBytes, err := hex.DecodeString(row.Server)
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

	preimageHashBytes, err := hex.DecodeString(row.PreimageHash)
	if err != nil {
		return domain.Vhtlc{}, err
	}

	unilateralClaimDelay := arklib.RelativeLocktime{
		Type:  arklib.RelativeLocktimeType(row.UnilateralClaimDelayType),
		Value: uint32(row.UnilateralClaimDelayValue),
	}
	unilateralRefundDelay := arklib.RelativeLocktime{
		Type:  arklib.RelativeLocktimeType(row.UnilateralRefundDelayType),
		Value: uint32(row.UnilateralRefundDelayValue),
	}
	unilateralRefundWithoutReceiverDelay := arklib.RelativeLocktime{
		Type:  arklib.RelativeLocktimeType(row.UnilateralRefundWithoutReceiverDelayType),
		Value: uint32(row.UnilateralRefundWithoutReceiverDelayValue),
	}

	opts := vhtlc.Opts{
		Sender:                               sender,
		Receiver:                             receiver,
		Server:                               server,
		RefundLocktime:                       arklib.AbsoluteLocktime(row.RefundLocktime),
		UnilateralClaimDelay:                 unilateralClaimDelay,
		UnilateralRefundDelay:                unilateralRefundDelay,
		UnilateralRefundWithoutReceiverDelay: unilateralRefundWithoutReceiverDelay,
		PreimageHash:                         preimageHashBytes,
	}

	return domain.NewVhtlc(opts), nil
}

func toVhtlcRow(v domain.Vhtlc) queries.InsertVHTLCParams {
	preimageHash := v.PreimageHash
	sender := v.Sender.SerializeCompressed()
	receiver := v.Receiver.SerializeCompressed()
	server := hex.EncodeToString(v.Server.SerializeCompressed())

	vhtlcId := domain.GetVhtlcId(preimageHash, sender, receiver)

	lockingScriptHex, err := vhtlc.LockingScriptHexFromOpts(v.Opts)
	if err != nil {
		log.WithError(err).Error("failed to derive taproot locking script for vhtlc")
		return queries.InsertVHTLCParams{
			ID:                                       vhtlcId,
			PreimageHash:                             hex.EncodeToString(preimageHash),
			Sender:                                   hex.EncodeToString(sender),
			Receiver:                                 hex.EncodeToString(receiver),
			Server:                                   server,
			RefundLocktime:                           int64(v.RefundLocktime),
			UnilateralClaimDelayType:                 int64(v.UnilateralClaimDelay.Type),
			UnilateralClaimDelayValue:                int64(v.UnilateralClaimDelay.Value),
			UnilateralRefundDelayType:                int64(v.UnilateralRefundDelay.Type),
			UnilateralRefundDelayValue:               int64(v.UnilateralRefundDelay.Value),
			UnilateralRefundWithoutReceiverDelayType: int64(v.UnilateralRefundWithoutReceiverDelay.Type),
			UnilateralRefundWithoutReceiverDelayValue: int64(v.UnilateralRefundWithoutReceiverDelay.Value),
			Script: "[]",
		}
	}

	return queries.InsertVHTLCParams{
		ID:                                       vhtlcId,
		PreimageHash:                             hex.EncodeToString(preimageHash),
		Sender:                                   hex.EncodeToString(sender),
		Receiver:                                 hex.EncodeToString(receiver),
		Server:                                   server,
		RefundLocktime:                           int64(v.RefundLocktime),
		UnilateralClaimDelayType:                 int64(v.UnilateralClaimDelay.Type),
		UnilateralClaimDelayValue:                int64(v.UnilateralClaimDelay.Value),
		UnilateralRefundDelayType:                int64(v.UnilateralRefundDelay.Type),
		UnilateralRefundDelayValue:               int64(v.UnilateralRefundDelay.Value),
		UnilateralRefundWithoutReceiverDelayType: int64(v.UnilateralRefundWithoutReceiverDelay.Type),
		UnilateralRefundWithoutReceiverDelayValue: int64(v.UnilateralRefundWithoutReceiverDelay.Value),
		Script: lockingScriptHex,
	}
}
