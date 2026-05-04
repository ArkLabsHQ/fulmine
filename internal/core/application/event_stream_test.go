package application

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"testing"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/core/ports"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/txutils"
	"github.com/arkade-os/arkd/pkg/client-lib/indexer"
	clienttypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/input"
	"github.com/stretchr/testify/require"
)

func TestHandleVhtlcScriptEventEmitsEventsAndUntracksTerminalSpend(t *testing.T) {
	record, preimage := newVhtlc(t)
	lockingScript, err := vhtlc.LockingScriptHexFromOpts(record.Opts)
	require.NoError(t, err)

	repo := newFakeVHTLCRepository(record)
	svc := &Service{
		dbSvc:  fakeRepoManager{vhtlcRepo: repo},
		events: make(chan VhtlcEvent, 4),
		vhtlcSubscription: &subscriptionHandler{
			scripts: fakeScriptsStore{},
			mu:      sync.Mutex{},
		},
	}

	svc.handleVhtlcScriptEvent(indexer.ScriptEvent{
		Data: &indexer.ScriptEventData{
			Txid: "funding-txid",
			NewVtxos: []clienttypes.Vtxo{
				{Script: lockingScript},
			},
		},
	})

	funded := <-svc.events
	require.Equal(t, EventTypeVhtlcFunded, funded.Type)
	require.Equal(t, record.Id, funded.ID)
	require.Equal(t, "funding-txid", funded.Txid)
	require.True(t, repo.records[record.Id].Tracked)

	vhtlcScript, err := vhtlc.NewVHTLCScriptFromOpts(record.Opts)
	require.NoError(t, err)
	claimScript, err := vhtlcScript.ClaimClosure.Script()
	require.NoError(t, err)

	spentVtxo := clienttypes.Vtxo{
		Outpoint: clienttypes.Outpoint{
			Txid: randomEventStreamTxID(t),
			VOut: 1,
		},
		Script: lockingScript,
	}
	spendTx := buildEventStreamSpendPSBT(t, spentVtxo, claimScript, preimage)

	svc.handleVhtlcScriptEvent(indexer.ScriptEvent{
		Data: &indexer.ScriptEventData{
			Txid:       "claim-txid",
			Tx:         spendTx,
			SpentVtxos: []clienttypes.Vtxo{spentVtxo},
		},
	})

	claimed := <-svc.events
	require.Equal(t, EventTypeVhtlcClaimed, claimed.Type)
	require.Equal(t, record.Id, claimed.ID)
	require.Equal(t, "claim-txid", claimed.Txid)
	require.Equal(t, hex.EncodeToString(preimage), claimed.Preimage)
	require.False(t, repo.records[record.Id].Tracked)
	require.Equal(t, [][]string{{lockingScript}}, repo.untrackedScripts)
}

func TestClassifyVhtlcSpendMatchesArkSpentByPrevout(t *testing.T) {
	record, preimage := newVhtlc(t)
	lockingScript, err := vhtlc.LockingScriptHexFromOpts(record.Opts)
	require.NoError(t, err)

	vhtlcScript, err := vhtlc.NewVHTLCScriptFromOpts(record.Opts)
	require.NoError(t, err)
	claimScript, err := vhtlcScript.ClaimClosure.Script()
	require.NoError(t, err)

	spentVtxo := clienttypes.Vtxo{
		Outpoint: clienttypes.Outpoint{
			Txid: randomEventStreamTxID(t),
			VOut: 0,
		},
		Script:  lockingScript,
		Spent:   true,
		SpentBy: randomEventStreamTxID(t),
	}

	details, err := classifyVhtlcSpend(
		buildEventStreamSpendPSBT(t, spentVtxo, claimScript, preimage),
		[]clienttypes.Vtxo{spentVtxo},
		record,
	)
	require.NoError(t, err)
	require.Equal(t, EventTypeVhtlcClaimed, details.eventType)
	require.Equal(t, hex.EncodeToString(preimage), details.preimage)
}

func TestClassifyVhtlcSpendFromForfeitTxs(t *testing.T) {
	record, preimage := newVhtlc(t)
	lockingScript, err := vhtlc.LockingScriptHexFromOpts(record.Opts)
	require.NoError(t, err)

	vhtlcScript, err := vhtlc.NewVHTLCScriptFromOpts(record.Opts)
	require.NoError(t, err)
	claimScript, err := vhtlcScript.ClaimClosure.Script()
	require.NoError(t, err)

	spentVtxo := clienttypes.Vtxo{
		Outpoint: clienttypes.Outpoint{
			Txid: randomEventStreamTxID(t),
			VOut: 0,
		},
		Script:    lockingScript,
		Spent:     true,
		SettledBy: randomEventStreamTxID(t),
	}

	details, err := classifyVhtlcSpendFromForfeitTxs(
		[]string{buildEventStreamSpendPSBT(t, spentVtxo, claimScript, preimage)},
		[]clienttypes.Vtxo{spentVtxo},
		record,
	)
	require.NoError(t, err)
	require.Equal(t, EventTypeVhtlcClaimed, details.eventType)
	require.Equal(t, hex.EncodeToString(preimage), details.preimage)
}

func newVhtlc(t *testing.T) (domain.Vhtlc, []byte) {
	t.Helper()

	senderKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	receiverKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	serverKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	preimage := make([]byte, 32)
	_, err = rand.Read(preimage)
	require.NoError(t, err)

	hash := sha256.Sum256(preimage)
	opts := vhtlc.Opts{
		Sender:         senderKey.PubKey(),
		Receiver:       receiverKey.PubKey(),
		Server:         serverKey.PubKey(),
		PreimageHash:   input.Ripemd160H(hash[:]),
		RefundLocktime: arklib.AbsoluteLocktime(100),
		UnilateralClaimDelay: arklib.RelativeLocktime{
			Type:  arklib.LocktimeTypeSecond,
			Value: 512,
		},
		UnilateralRefundDelay: arklib.RelativeLocktime{
			Type:  arklib.LocktimeTypeSecond,
			Value: 512,
		},
		UnilateralRefundWithoutReceiverDelay: arklib.RelativeLocktime{
			Type:  arklib.LocktimeTypeSecond,
			Value: 1024,
		},
	}

	return domain.NewVhtlc(opts), preimage
}

func buildEventStreamSpendPSBT(
	t *testing.T, spentVtxo clienttypes.Vtxo, leafScript []byte, preimage []byte,
) string {
	t.Helper()

	prevTxid := spentVtxo.Txid
	prevVout := spentVtxo.VOut
	if spentVtxo.SpentBy != "" {
		prevTxid = spentVtxo.SpentBy
		prevVout = 0
	}

	prevHash, err := chainhash.NewHashFromStr(prevTxid)
	require.NoError(t, err)

	unsignedTx := wire.NewMsgTx(2)
	unsignedTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  *prevHash,
			Index: prevVout,
		},
	})
	unsignedTx.AddTxOut(&wire.TxOut{Value: 1})

	packet, err := psbt.NewFromUnsignedTx(unsignedTx)
	require.NoError(t, err)

	witness := wire.TxWitness{[]byte("sig"), leafScript, []byte("control")}
	if len(preimage) > 0 {
		witness = wire.TxWitness{[]byte("sig"), preimage, leafScript, []byte("control")}
	}
	var witnessBuf bytes.Buffer
	err = psbt.WriteTxWitness(&witnessBuf, witness)
	require.NoError(t, err)
	packet.Inputs[0].FinalScriptWitness = witnessBuf.Bytes()

	if len(preimage) > 0 {
		err = txutils.SetArkPsbtField(packet, 0, txutils.ConditionWitnessField, wire.TxWitness{preimage})
		require.NoError(t, err)
	}

	tx, err := packet.B64Encode()
	require.NoError(t, err)
	return tx
}

func buildEventStreamRawSpendTx(t *testing.T) string {
	t.Helper()

	prevHash, err := chainhash.NewHashFromStr(randomEventStreamTxID(t))
	require.NoError(t, err)

	tx := wire.NewMsgTx(2)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  *prevHash,
			Index: 1,
		},
	})
	tx.AddTxOut(&wire.TxOut{Value: 1})

	var buf bytes.Buffer
	err = tx.Serialize(&buf)
	require.NoError(t, err)
	return hex.EncodeToString(buf.Bytes())
}

func randomEventStreamTxID(t *testing.T) string {
	t.Helper()

	buf := make([]byte, 32)
	_, err := rand.Read(buf)
	require.NoError(t, err)
	return hex.EncodeToString(buf)
}

type fakeRepoManager struct {
	ports.RepoManager
	vhtlcRepo domain.VHTLCRepository
}

func (m fakeRepoManager) VHTLC() domain.VHTLCRepository {
	return m.vhtlcRepo
}

type fakeVHTLCRepository struct {
	records          map[string]domain.Vhtlc
	untrackedScripts [][]string
}

func newFakeVHTLCRepository(vhtlcs ...domain.Vhtlc) *fakeVHTLCRepository {
	records := make(map[string]domain.Vhtlc, len(vhtlcs))
	for _, v := range vhtlcs {
		records[v.Id] = v
	}
	return &fakeVHTLCRepository{records: records}
}

func (r *fakeVHTLCRepository) GetAll(ctx context.Context) ([]domain.Vhtlc, error) {
	out := make([]domain.Vhtlc, 0, len(r.records))
	for _, v := range r.records {
		out = append(out, v)
	}
	return out, nil
}

func (r *fakeVHTLCRepository) Get(ctx context.Context, id string) (*domain.Vhtlc, error) {
	v := r.records[id]
	return &v, nil
}

func (r *fakeVHTLCRepository) GetByIds(ctx context.Context, ids []string) ([]domain.Vhtlc, error) {
	out := make([]domain.Vhtlc, 0, len(ids))
	for _, id := range ids {
		if v, ok := r.records[id]; ok {
			out = append(out, v)
		}
	}
	return out, nil
}

func (r *fakeVHTLCRepository) GetByScripts(ctx context.Context, scripts []string) ([]domain.Vhtlc, error) {
	scriptSet := make(map[string]struct{}, len(scripts))
	for _, script := range scripts {
		scriptSet[script] = struct{}{}
	}

	out := make([]domain.Vhtlc, 0, len(scripts))
	for _, v := range r.records {
		script, err := vhtlc.LockingScriptHexFromOpts(v.Opts)
		if err != nil {
			return nil, err
		}
		if _, ok := scriptSet[script]; ok {
			out = append(out, v)
		}
	}
	return out, nil
}

func (r *fakeVHTLCRepository) GetScripts(ctx context.Context) ([]string, error) {
	out := make([]string, 0, len(r.records))
	for _, v := range r.records {
		if !v.Tracked {
			continue
		}
		script, err := vhtlc.LockingScriptHexFromOpts(v.Opts)
		if err != nil {
			return nil, err
		}
		out = append(out, script)
	}
	return out, nil
}

func (r *fakeVHTLCRepository) UntrackByScripts(ctx context.Context, scripts []string) error {
	r.untrackedScripts = append(r.untrackedScripts, append([]string(nil), scripts...))
	vhtlcs, err := r.GetByScripts(ctx, scripts)
	if err != nil {
		return err
	}
	for _, v := range vhtlcs {
		v.Tracked = false
		r.records[v.Id] = v
	}
	return nil
}

func (r *fakeVHTLCRepository) Add(ctx context.Context, v domain.Vhtlc) error {
	r.records[v.Id] = v
	return nil
}

func (r *fakeVHTLCRepository) Close() {}

type fakeScriptsStore struct{}

func (fakeScriptsStore) Get(ctx context.Context) ([]string, error) { return nil, nil }
func (fakeScriptsStore) Add(ctx context.Context, scripts []string) (int, error) {
	return len(scripts), nil
}
func (fakeScriptsStore) Delete(ctx context.Context, scripts []string) (int, error) {
	return len(scripts), nil
}
