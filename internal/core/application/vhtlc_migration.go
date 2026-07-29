package application

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/client-lib/identity"
	"github.com/arkade-os/go-sdk/contract"
	vhtlchandler "github.com/arkade-os/go-sdk/contract/handlers/vhtlc"
	hdidentity "github.com/arkade-os/go-sdk/identity"
	"github.com/arkade-os/go-sdk/types"
	"github.com/btcsuite/btcd/btcec/v2"
	log "github.com/sirupsen/logrus"
)

type legacyVhtlcStore interface {
	HasLegacy(ctx context.Context) (bool, error)
	GetLegacy(ctx context.Context) ([]domain.LegacyVhtlc, error)
	DropLegacy(ctx context.Context) error
	Get(ctx context.Context, id string) (*domain.Vhtlc, error)
	Add(ctx context.Context, v domain.Vhtlc) error
}

type vhtlcImporter interface {
	GetContracts(ctx context.Context, opts ...contract.FilterOption) ([]types.Contract, error)
	ImportContract(ctx context.Context, c types.Contract) error
}

func (s *Service) migrateVhtlcs(ctx context.Context) {
	identitySvc := s.Identity()

	if identitySvc.GetType() == hdidentity.Type {
		return
	}

	keyId := ""
	keyRef, err := identitySvc.GetKey(ctx, keyId)
	if err != nil {
		log.WithError(err).Warn("vhtlc migration: failed to resolve identity key")
		return
	}

	handler, err := s.ContractManager().Registry().GetHandler(types.ContractTypeVHTLC)
	if err != nil {
		log.WithError(err).Warn("vhtlc migration: no vhtlc handler")
		return
	}
	buildContract := func(c context.Context, args vhtlchandler.ContractArgs) (*types.Contract, error) {
		return handler.NewContract(c, args)
	}

	migrated, skipped, err := migrateVhtlcs(
		ctx, s.dbSvc.VHTLC(), s.ContractManager(), keyRef, buildContract,
	)
	if err != nil {
		log.WithError(err).Error("vhtlc migration failed")
		return
	}
	if migrated > 0 || skipped > 0 {
		log.Debugf("vhtlc migration: migrated %d vhtlcs, skipped %d", migrated, skipped)
	}
}

// migrateVhtlcs rebuilds each legacy vhtlc's script, imports it in the sdk contract manager,
// and records {id, script} in the new vhtlc table. It is idempotent and drops vhtlc_legacy only
// when no row was skipped.
func migrateVhtlcs(
	ctx context.Context, store legacyVhtlcStore, importer vhtlcImporter, keyRef *identity.KeyRef,
	buildContract func(context.Context, vhtlchandler.ContractArgs) (*types.Contract, error),
) (migrated int, skipped int, err error) {
	has, err := store.HasLegacy(ctx)
	if err != nil {
		return 0, 0, err
	}
	if !has {
		return 0, 0, nil
	}

	rows, err := store.GetLegacy(ctx)
	if err != nil {
		return 0, 0, err
	}

	for _, row := range rows {
		args, id, err := buildVhtlcContractArgs(row, keyRef)
		if err != nil {
			log.WithError(err).Warnf(
				"vhtlc migration: skipping row (preimage %s)", row.PreimageHash,
			)
			skipped++
			continue
		}

		// Already migrated in a prior pass.
		if _, err := store.Get(ctx, id); err == nil {
			continue
		}

		c, err := buildContract(ctx, args)
		if err != nil {
			log.WithError(err).Warnf("vhtlc migration: skipping %s (build contract failed)", id)
			skipped++
			continue
		}

		tracked, err := importer.GetContracts(ctx, contract.WithScripts([]string{c.Script}))
		if err != nil {
			log.WithError(err).Warnf("vhtlc migration: skipping %s (lookup contract failed)", id)
			skipped++
			continue
		}
		if len(tracked) == 0 {
			if err := importer.ImportContract(ctx, *c); err != nil {
				log.WithError(err).Warnf(
					"vhtlc migration: skipping %s (import contract failed)", id,
				)
				skipped++
				continue
			}
		}

		if err := store.Add(ctx, domain.NewVhtlc(id, c.Script)); err != nil {
			log.WithError(err).Warnf("vhtlc migration: skipping %s (persist contract failed)", id)
			skipped++
			continue
		}
		migrated++
	}

	if skipped == 0 {
		if err := store.DropLegacy(ctx); err != nil {
			return migrated, skipped, fmt.Errorf("failed to drop vhtlc_legacy: %w", err)
		}
	}
	return migrated, skipped, nil
}

// buildVhtlcContractArgs turns a legacy parameter row into the go-sdk VHTLC
// handler args, wiring our identity key to whichever side (sender/receiver) we
// own, and returns the recomputed vhtlc id.
func buildVhtlcContractArgs(
	row domain.LegacyVhtlc, ourKeyRef *identity.KeyRef,
) (vhtlchandler.ContractArgs, string, error) {
	var empty vhtlchandler.ContractArgs

	preimage, err := hex.DecodeString(row.PreimageHash)
	if err != nil {
		return empty, "", fmt.Errorf("bad preimage hash: %w", err)
	}
	senderBytes, err := hex.DecodeString(row.Sender)
	if err != nil {
		return empty, "", fmt.Errorf("bad sender: %w", err)
	}
	receiverBytes, err := hex.DecodeString(row.Receiver)
	if err != nil {
		return empty, "", fmt.Errorf("bad receiver: %w", err)
	}
	sender, err := btcec.ParsePubKey(senderBytes)
	if err != nil {
		return empty, "", fmt.Errorf("bad sender pubkey: %w", err)
	}
	receiver, err := btcec.ParsePubKey(receiverBytes)
	if err != nil {
		return empty, "", fmt.Errorf("bad receiver pubkey: %w", err)
	}
	serverBytes, err := hex.DecodeString(row.Server)
	if err != nil {
		return empty, "", fmt.Errorf("bad server: %w", err)
	}
	server, err := btcec.ParsePubKey(serverBytes)
	if err != nil {
		return empty, "", fmt.Errorf("bad server pubkey: %w", err)
	}

	args := vhtlchandler.ContractArgs{
		Sender:         sender,
		Receiver:       receiver,
		Signer:         server,
		PreimageHash:   preimage,
		RefundLocktime: arklib.AbsoluteLocktime(row.RefundLocktime),
		UnilateralClaimDelay: arklib.RelativeLocktime{
			Type:  arklib.RelativeLocktimeType(row.UnilateralClaimDelayType),
			Value: uint32(row.UnilateralClaimDelayValue),
		},
		UnilateralRefundDelay: arklib.RelativeLocktime{
			Type:  arklib.RelativeLocktimeType(row.UnilateralRefundDelayType),
			Value: uint32(row.UnilateralRefundDelayValue),
		},
		UnilateralRefundWithoutReceiverDelay: arklib.RelativeLocktime{
			Type:  arklib.RelativeLocktimeType(row.UnilateralRefundWithoutReceiverDelayType),
			Value: uint32(row.UnilateralRefundWithoutReceiverDelayValue),
		},
	}

	ourCompressed := ourKeyRef.PubKey.SerializeCompressed()
	switch {
	case bytes.Equal(receiver.SerializeCompressed(), ourCompressed):
		args.ReceiverKeyId = ourKeyRef.Id
	case bytes.Equal(sender.SerializeCompressed(), ourCompressed):
		args.SenderKeyId = ourKeyRef.Id
	default:
		return empty, "", fmt.Errorf("vhtlc not owned by wallet key")
	}

	id := domain.GetVhtlcId(preimage, sender.SerializeCompressed(), receiver.SerializeCompressed())
	return args, id, nil
}
