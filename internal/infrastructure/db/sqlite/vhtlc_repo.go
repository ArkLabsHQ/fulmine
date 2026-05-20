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

func (r *vhtlcRepository) GetByIds(ctx context.Context, ids []string) ([]domain.Vhtlc, error) {
	rows, err := r.querier.ListVHTLCsByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Vhtlc, 0, len(rows))
	for _, row := range rows {
		vhtlcs, err := toVhtlc(row)
		if err != nil {
			return nil, err
		}
		out = append(out, vhtlcs)
	}
	return out, nil
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

	hasPkScript := row.NonInteractiveReceiverPkscript.Valid
	hasPubKey := row.NonInteractiveIntrospectorPubkey.Valid
	if hasPkScript != hasPubKey {
		return domain.Vhtlc{}, fmt.Errorf(
			"inconsistent non-interactive data: both receiver pkScript and introspector pubkey must be set together",
		)
	}
	hasExtraPacket := row.NonInteractiveExtraPacket.Valid && row.NonInteractiveExtraPacket.String != ""
	if !hasPkScript && hasExtraPacket {
		return domain.Vhtlc{}, fmt.Errorf(
			"orphan non-interactive extra packet without claim fields",
		)
	}

	var extraPacket []byte
	if hasPkScript && hasPubKey {
		pkScript, err := hex.DecodeString(row.NonInteractiveReceiverPkscript.String)
		if err != nil {
			return domain.Vhtlc{}, fmt.Errorf(
				"decode non-interactive receiver pkScript: %w", err,
			)
		}
		pubBytes, err := hex.DecodeString(row.NonInteractiveIntrospectorPubkey.String)
		if err != nil {
			return domain.Vhtlc{}, fmt.Errorf(
				"decode non-interactive introspector pubkey: %w", err,
			)
		}
		pub, err := btcec.ParsePubKey(pubBytes)
		if err != nil {
			return domain.Vhtlc{}, fmt.Errorf(
				"parse non-interactive introspector pubkey: %w", err,
			)
		}
		opts.NonInteractiveClaim = &vhtlc.NonInteractiveClaimOpts{
			ReceiverPkScript:   pkScript,
			IntrospectorPubKey: pub,
		}
		if hasExtraPacket {
			extraPacket, err = hex.DecodeString(row.NonInteractiveExtraPacket.String)
			if err != nil {
				return domain.Vhtlc{}, fmt.Errorf(
					"decode non-interactive extra packet: %w", err,
				)
			}
		}
	}

	return domain.NewVhtlc(opts, extraPacket), nil
}

func toVhtlcRow(vhtlc domain.Vhtlc) queries.InsertVHTLCParams {
	preimageHash := vhtlc.PreimageHash
	sender := vhtlc.Sender.SerializeCompressed()
	receiver := vhtlc.Receiver.SerializeCompressed()
	server := hex.EncodeToString(vhtlc.Server.SerializeCompressed())

	vhtlcId := domain.GetVhtlcId(preimageHash, sender, receiver)

	params := queries.InsertVHTLCParams{
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
	if vhtlc.NonInteractiveClaim != nil {
		params.NonInteractiveReceiverPkscript = sql.NullString{
			String: hex.EncodeToString(vhtlc.NonInteractiveClaim.ReceiverPkScript),
			Valid:  true,
		}
		params.NonInteractiveIntrospectorPubkey = sql.NullString{
			String: hex.EncodeToString(
				vhtlc.NonInteractiveClaim.IntrospectorPubKey.SerializeCompressed(),
			),
			Valid: true,
		}
		if len(vhtlc.ExtraPacket) > 0 {
			params.NonInteractiveExtraPacket = sql.NullString{
				String: hex.EncodeToString(vhtlc.ExtraPacket),
				Valid:  true,
			}
		}
	}
	return params
}
