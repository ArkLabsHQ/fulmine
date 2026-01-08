package application

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/intent"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/google/uuid"

	log "github.com/sirupsen/logrus"
)

type DelegatorService struct {
	svc *Service
	fee uint64
	
	cachedDelegatorAddress *arklib.Address
}

type DelegateInfo struct {
	DelegatorPublicKey string
	Fee uint64
	DelegatorAddress string
}

func NewDelegatorService(svc *Service, fee uint64) *DelegatorService {
	s := &DelegatorService{
		svc: svc,
		fee: fee,
	}

	if err := s.restorePendingTasks(); err != nil {
		log.WithError(err).Warn("failed to restore pending tasks")
	}

	return s
}

func (s *DelegatorService) GetDelegateInfo(ctx context.Context) (*DelegateInfo, error) {
	offchainAddr, err := s.getDelegatorAddress(ctx)
	if err != nil {
		return nil, err
	}

	encodedAddr, err := offchainAddr.EncodeV0()
	if err != nil {
		return nil, err
	}

	return &DelegateInfo{
		DelegatorPublicKey: hex.EncodeToString(s.svc.publicKey.SerializeCompressed()),
		Fee: s.fee,
		DelegatorAddress: encodedAddr,
	}, nil
}

func (s *DelegatorService) Delegate(
	ctx context.Context, 
	intentMessage intent.RegisterMessage, intentProof intent.Proof, forfeit *psbt.Packet,
) error {
	if err := s.svc.isInitializedAndUnlocked(ctx); err != nil {
		return err
	}
	// TODO : validate intent
	// TODO : validate forfeit

	task, err := s.newDelegateTask(ctx, intentMessage, intentProof, forfeit)
	if err != nil {
		return err
	}

	// validate delegator fee
	if task.Fee < s.fee {
		return fmt.Errorf("delegator fee is less than the required fee (expected at least %d, got %d)", s.fee, task.Fee)
	}

	// TODO validate intent fee

	err = s.svc.dbSvc.Delegate().AddOrUpdate(ctx, *task)
	if err != nil {
		return err
	}

	// schedule task

	return nil
}

func (s *DelegatorService) newDelegateTask(
	ctx context.Context, message intent.RegisterMessage, proof intent.Proof, forfeit *psbt.Packet,
) (*domain.DelegateTask, error) {
	id := uuid.New().String()
	encodedMessage, err := message.Encode()
	if err != nil {
		return nil, fmt.Errorf("failed to encode intent message: %w", err)
	}
	encodedProof, err := proof.B64Encode()
	if err != nil {
		return nil, fmt.Errorf("failed to encode intent proof: %w", err)
	}
	encodedForfeit, err := forfeit.B64Encode()
	if err != nil {
		return nil, fmt.Errorf("failed to encode forfeit: %w", err)
	}

	delegatorAddr, err := s.getDelegatorAddress(ctx)
	if err != nil {
		return nil, err
	}

	delegatorAddrScript, err := delegatorAddr.GetPkScript()
	if err != nil {
		return nil, err
	}

	feeAmount := int64(0)

	// search for the fee output in intent proof
	for _, output := range proof.UnsignedTx.TxOut {
		if bytes.Equal(output.PkScript, delegatorAddrScript) {
			feeAmount = output.Value
			break
		}
	}

	if message.ValidAt == 0 {
		return nil, fmt.Errorf("invalid valid at")
	}

	ScheduledAt := time.Unix(message.ValidAt, 0)

	return &domain.DelegateTask{
		ID: id,
		Intent: domain.Intent{
			Message: encodedMessage,
			Proof: encodedProof,
		},
		ForfeitTx: encodedForfeit,
		Fee: uint64(feeAmount),
		DelegatorPublicKey: hex.EncodeToString(s.svc.publicKey.SerializeCompressed()),
		Inputs: proof.GetOutpoints(),
		ScheduledAt: ScheduledAt,
		Status: domain.DelegateTaskStatusPending,
	}, nil
}

func (s *DelegatorService) getDelegatorAddress(ctx context.Context) (*arklib.Address, error) {
	if s.cachedDelegatorAddress != nil {
		return s.cachedDelegatorAddress, nil
	}

	if err := s.svc.isInitializedAndUnlocked(ctx); err != nil {
		return nil, err
	}

	_, addr, _, _, _, err := s.svc.GetAddress(ctx, 0)
	if err != nil {
		return nil, err
	}

	decodedAddr, err := arklib.DecodeAddressV0(addr)
	if err != nil {
		return nil, err
	}

	s.cachedDelegatorAddress = decodedAddr

	return decodedAddr, nil
}

func (s *DelegatorService) restorePendingTasks() error {
	ctx := context.Background()
	pendingTasks, err := s.svc.dbSvc.Delegate().GetAllPending(ctx)
	if err != nil {
		return err
	}

	for _, pendingTask := range pendingTasks {
		if err = s.svc.schedulerSvc.ScheduleTaskAtTime(pendingTask.ScheduledAt, func() {
			if err := s.executeDelegate(pendingTask.ID); err != nil {
				log.WithError(err).Warnf("failed to execute delegate task %s", pendingTask.ID)
			}
		}); err != nil {
			log.WithError(err).Warnf("failed to schedule delegate task %s", pendingTask.ID)
			continue
		}
	}

	return nil
}

func (s *DelegatorService) executeDelegate(id string) error {
	ctx := context.Background()
	_, err := s.svc.dbSvc.Delegate().GetByID(ctx, id)
	if err != nil {
		return err
	}

	// TODO: join batch

	return nil
}