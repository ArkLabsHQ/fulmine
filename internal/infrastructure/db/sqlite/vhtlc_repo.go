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
	"github.com/ark-network/ark/common"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
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

func (r *vhtlcRepository) Add(ctx context.Context, opts vhtlc.Opts) error {
	optsParams := toOptParams(opts)
	if _, err := r.Get(ctx, optsParams.PreimageHash); err == nil {
		return fmt.Errorf("vHTLC with preimage hash %s alllready exists", optsParams.PreimageHash)
	}

	if err := r.querier.InsertVHTLC(ctx, optsParams); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("vHTLC with preimage hash %s already exists", optsParams.PreimageHash)
		}
		return err
	}
	return nil
}

func (r *vhtlcRepository) Get(ctx context.Context, preimageHash string) (*vhtlc.Opts, error) {
	row, err := r.querier.GetVHTLC(ctx, preimageHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("vHTLC with preimage hash %s not found", preimageHash)

		}
		return nil, err
	}
	return toOpts(row)
}

func (r *vhtlcRepository) GetAll(ctx context.Context) ([]vhtlc.Opts, error) {
	rows, err := r.querier.ListVHTLC(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]vhtlc.Opts, 0, len(rows))
	for _, row := range rows {
		opt, err := toOpts(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *opt)
	}
	return out, nil
}

func (r *vhtlcRepository) Close() {
	if r.db != nil {
		r.db.Close()
	}
}

func toOpts(row queries.Vhtlc) (*vhtlc.Opts, error) {
	senderBytes, err := hex.DecodeString(row.Sender)
	if err != nil {
		return nil, err
	}
	receiverBytes, err := hex.DecodeString(row.Receiver)
	if err != nil {
		return nil, err
	}
	serverBytes, err := hex.DecodeString(row.Server)
	if err != nil {
		return nil, err
	}

	sender, err := secp256k1.ParsePubKey(senderBytes)
	if err != nil {
		return nil, err
	}
	receiver, err := secp256k1.ParsePubKey(receiverBytes)
	if err != nil {
		return nil, err
	}
	server, err := secp256k1.ParsePubKey(serverBytes)
	if err != nil {
		return nil, err
	}

	preimageHashBytes, err := hex.DecodeString(row.PreimageHash)
	if err != nil {
		return nil, err
	}

	unilateralClaimDelay := common.RelativeLocktime{
		Type:  common.RelativeLocktimeType(row.UnilateralClaimDelayType),
		Value: uint32(row.UnilateralClaimDelayValue),
	}
	unilateralRefundDelay := common.RelativeLocktime{
		Type:  common.RelativeLocktimeType(row.UnilateralRefundDelayType),
		Value: uint32(row.UnilateralRefundDelayValue),
	}
	unilateralRefundWithoutReceiverDelay := common.RelativeLocktime{
		Type:  common.RelativeLocktimeType(row.UnilateralRefundWithoutReceiverDelayType),
		Value: uint32(row.UnilateralRefundWithoutReceiverDelayValue),
	}

	return &vhtlc.Opts{
		Sender:                               sender,
		Receiver:                             receiver,
		Server:                               server,
		RefundLocktime:                       common.AbsoluteLocktime(row.RefundLocktime),
		UnilateralClaimDelay:                 unilateralClaimDelay,
		UnilateralRefundDelay:                unilateralRefundDelay,
		UnilateralRefundWithoutReceiverDelay: unilateralRefundWithoutReceiverDelay,
		PreimageHash:                         preimageHashBytes,
	}, nil
}

func toOptParams(opts vhtlc.Opts) queries.InsertVHTLCParams {
	preimageHash := hex.EncodeToString(opts.PreimageHash)
	sender := hex.EncodeToString(opts.Sender.SerializeCompressed())
	receiver := hex.EncodeToString(opts.Receiver.SerializeCompressed())
	server := hex.EncodeToString(opts.Server.SerializeCompressed())

	return queries.InsertVHTLCParams{
		PreimageHash:                             preimageHash,
		Sender:                                   sender,
		Receiver:                                 receiver,
		Server:                                   server,
		RefundLocktime:                           int64(opts.RefundLocktime),
		UnilateralClaimDelayType:                 int64(opts.UnilateralClaimDelay.Type),
		UnilateralClaimDelayValue:                int64(opts.UnilateralClaimDelay.Value),
		UnilateralRefundDelayType:                int64(opts.UnilateralRefundDelay.Type),
		UnilateralRefundDelayValue:               int64(opts.UnilateralRefundDelay.Value),
		UnilateralRefundWithoutReceiverDelayType: int64(opts.UnilateralRefundWithoutReceiverDelay.Type),
		UnilateralRefundWithoutReceiverDelayValue: int64(opts.UnilateralRefundWithoutReceiverDelay.Value),
	}
}
