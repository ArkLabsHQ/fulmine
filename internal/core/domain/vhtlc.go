package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/arkade-os/go-sdk/vhtlc"
	"github.com/btcsuite/btcd/btcec/v2"
)

type Vhtlc struct {
	vhtlc.Opts
	Id string
}

// VHTLCRepository stores the VHTLC options owned by the wallet
type VHTLCRepository interface {
	GetAll(ctx context.Context) ([]Vhtlc, error)
	Get(ctx context.Context, id string) (*Vhtlc, error)
	GetByIds(ctx context.Context, ids []string) ([]Vhtlc, error)
	Add(ctx context.Context, vhtlc Vhtlc) error
	Close()
}

func NewVhtlc(opts vhtlc.Opts) Vhtlc {
	preimageHash := opts.PreimageHash
	sender := opts.Sender.SerializeCompressed()
	receiver := opts.Receiver.SerializeCompressed()
	return Vhtlc{
		Opts: opts,
		Id:   GetVhtlcId(preimageHash, sender, receiver),
	}
}

func ParseNonInteractiveClaim(
	pkScriptHex, emulatorPubKeyHex string,
) (*vhtlc.NonInteractiveClaimOpts, error) {
	if (pkScriptHex == "") != (emulatorPubKeyHex == "") {
		return nil, fmt.Errorf(
			"inconsistent non-interactive data: both receiver pkScript and emulator pubkey must be set together",
		)
	}
	if pkScriptHex == "" {
		return nil, nil
	}
	pkScript, err := hex.DecodeString(pkScriptHex)
	if err != nil {
		return nil, fmt.Errorf("decode non-interactive receiver pkScript: %w", err)
	}
	pubBytes, err := hex.DecodeString(emulatorPubKeyHex)
	if err != nil {
		return nil, fmt.Errorf("decode non-interactive emulator pubkey: %w", err)
	}
	pub, err := btcec.ParsePubKey(pubBytes)
	if err != nil {
		return nil, fmt.Errorf("parse non-interactive emulator pubkey: %w", err)
	}
	return &vhtlc.NonInteractiveClaimOpts{
		ReceiverPkScript: pkScript,
		EmulatorPubKey:   pub,
	}, nil
}

func GetVhtlcId(preimageHash, sender, receiver []byte) string {
	id := make([]byte, 0, len(preimageHash)+len(sender)+len(receiver))
	id = append(id, preimageHash...)
	id = append(id, sender...)
	id = append(id, receiver...)
	id_hash := sha256.Sum256(id)
	return hex.EncodeToString(id_hash[:])
}
