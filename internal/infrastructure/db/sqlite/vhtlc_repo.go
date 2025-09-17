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
		return fmt.Errorf("vHTLC withID %s already exists", optsParams.ID)
	}

	if err := r.querier.InsertVHTLC(ctx, optsParams); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("vHTLC with ID %s already exists", optsParams.ID)
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

func toVhtlcRow(vhtlc domain.Vhtlc) queries.InsertVHTLCParams {
	preimageHash := vhtlc.PreimageHash
	sender := vhtlc.Sender.SerializeCompressed()
	receiver := vhtlc.Receiver.SerializeCompressed()
	server := hex.EncodeToString(vhtlc.Server.SerializeCompressed())

	vhtlcId := domain.CreateVhtlcId(preimageHash, sender, receiver)

	return queries.InsertVHTLCParams{
		ID:                                       vhtlcId,
		PreimageHash:                             hex.EncodeToString(preimageHash),
		Sender:                                   hex.EncodeToString(sender),
		Receiver:                                 hex.EncodeToString(receiver),
		Server:                                   server,
		RefundLocktime:                           int64(vhtlc.RefundLocktime),
		UnilateralClaimDelayType:                 int64(vhtlc.UnilateralClaimDelay.Type),
		UnilateralClaimDelayValue:                int64(vhtlc.UnilateralClaimDelay.Value),
		UnilateralRefundDelayType:                int64(vhtlc.UnilateralRefundDelay.Type),
		UnilateralRefundDelayValue:               int64(vhtlc.UnilateralRefundDelay.Value),
		UnilateralRefundWithoutReceiverDelayType: int64(vhtlc.UnilateralRefundWithoutReceiverDelay.Type),
		UnilateralRefundWithoutReceiverDelayValue: int64(vhtlc.UnilateralRefundWithoutReceiverDelay.Value),
	}
}
