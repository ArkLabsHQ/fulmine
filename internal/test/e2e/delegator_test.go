package e2e_test

import (
	"encoding/hex"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/arkade-os/arkd/pkg/ark-lib/intent"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/arkade-os/arkd/pkg/ark-lib/tree"
	"github.com/arkade-os/arkd/pkg/ark-lib/txutils"
	"github.com/arkade-os/go-sdk/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// TestDelegate delegate the renewal of a single vtxo
func TestDelegate(t *testing.T) {
	ctx := t.Context()
	alice, _, alicePubKey, grpcClient := setupArkSDKwithPublicKey(t)
	defer alice.Stop()
	defer grpcClient.Close()

	delegatorClient, err := newDelegatorClient("localhost:7004")
	require.NoError(t, err)
	require.NotNil(t, delegatorClient)

	delegateInfo, err := delegatorClient.GetDelegatorInfo(ctx, &pb.GetDelegatorInfoRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, delegateInfo.GetPubkey())
	require.NotEmpty(t, delegateInfo.GetFee())

	delegatorPubKeyBytes, err := hex.DecodeString(delegateInfo.GetPubkey())
	require.NoError(t, err)
	delegatorPubKey, err := btcec.ParsePubKey(delegatorPubKeyBytes)
	require.NoError(t, err)
	require.NotNil(t, delegatorPubKey)

	_, aliceAddr, _, _, err := alice.GetAddresses(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, aliceAddr)

	aliceArkAddr, err := arklib.DecodeAddressV0(aliceAddr[0])
	require.NoError(t, err)
	require.NotNil(t, aliceArkAddr)

	aliceConfig, err := alice.GetConfigData(ctx)
	require.NoError(t, err)

	signerPubKey := aliceConfig.SignerPubKey

	collaborativeAliceDelegatorClosure := &script.MultisigClosure{
		PubKeys: []*btcec.PublicKey{alicePubKey, delegatorPubKey, signerPubKey},
	}

	exitLocktime := arklib.RelativeLocktime{
		Type:  arklib.LocktimeTypeSecond,
		Value: 1024,
	}

	delegationVtxoScript := script.TapscriptsVtxoScript{
		Closures: []script.Closure{
			collaborativeAliceDelegatorClosure,
			&script.MultisigClosure{
				PubKeys: []*btcec.PublicKey{alicePubKey, signerPubKey},
			},
			&script.CSVMultisigClosure{
				Locktime: exitLocktime,
				MultisigClosure: script.MultisigClosure{
					PubKeys: []*btcec.PublicKey{alicePubKey},
				},
			},
		},
	}

	vtxoTapKey, vtxoTapTree, err := delegationVtxoScript.TapTree()
	require.NoError(t, err)

	arkAddress := arklib.Address{
		HRP:        "tark",
		VtxoTapKey: vtxoTapKey,
		Signer:     signerPubKey,
	}

	arkAddressStr, err := arkAddress.EncodeV0()
	require.NoError(t, err)

	faucetOffchain(t, alice, 0.00021)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	var incomingFunds []types.Vtxo
	var incomingErr error
	go func() {
		incomingFunds, incomingErr = alice.NotifyIncomingFunds(ctx, arkAddressStr)
		wg.Done()
	}()
	_, err = alice.SendOffChain(ctx, []types.Receiver{{
		To:     arkAddressStr,
		Amount: 21000,
	}})
	require.NoError(t, err)

	wg.Wait()
	require.NoError(t, incomingErr)
	require.NotEmpty(t, incomingFunds)

	aliceVtxo := incomingFunds[0]

	intentMessage := intent.RegisterMessage{
		BaseMessage: intent.BaseMessage{
			Type: intent.IntentMessageTypeRegister,
		},
		CosignersPublicKeys: []string{delegateInfo.GetPubkey()},
		ValidAt:             time.Now().Add(3 * time.Second).Unix(),
		ExpireAt:            0,
	}

	encodedIntentMessage, err := intentMessage.Encode()
	require.NoError(t, err)

	vtxoHash, err := chainhash.NewHashFromStr(aliceVtxo.Txid)
	require.NoError(t, err)

	exitScript, err := delegationVtxoScript.ExitClosures()[0].Script()
	require.NoError(t, err)

	exitScriptMerkleProof, err := vtxoTapTree.GetTaprootMerkleProof(
		txscript.NewBaseTapLeaf(exitScript).TapHash(),
	)
	require.NoError(t, err)

	sequence, err := arklib.BIP68Sequence(exitLocktime)
	require.NoError(t, err)

	delegatePkScript, err := arkAddress.GetPkScript()
	require.NoError(t, err)

	alicePkScript, err := aliceArkAddr.GetPkScript()
	require.NoError(t, err)

	intentProof, err := intent.New(
		encodedIntentMessage,
		[]intent.Input{
			{
				OutPoint: &wire.OutPoint{
					Hash:  *vtxoHash,
					Index: aliceVtxo.VOut,
				},
				Sequence: sequence,
				WitnessUtxo: &wire.TxOut{
					Value:    int64(aliceVtxo.Amount),
					PkScript: delegatePkScript,
				},
			},
		},
		[]*wire.TxOut{
			{
				Value:    int64(aliceVtxo.Amount),
				PkScript: alicePkScript,
			},
		},
	)
	require.NoError(t, err)

	tapLeafScript := &psbt.TaprootTapLeafScript{
		ControlBlock: exitScriptMerkleProof.ControlBlock,
		Script:       exitScriptMerkleProof.Script,
		LeafVersion:  txscript.BaseLeafVersion,
	}

	intentProof.Inputs[0].TaprootLeafScript = []*psbt.TaprootTapLeafScript{tapLeafScript}
	intentProof.Inputs[1].TaprootLeafScript = []*psbt.TaprootTapLeafScript{tapLeafScript}

	scripts, err := delegationVtxoScript.Encode()
	require.NoError(t, err)

	tapTree := txutils.TapTree(scripts)

	err = txutils.SetArkPsbtField(&intentProof.Packet, 1, txutils.VtxoTaprootTreeField, tapTree)
	require.NoError(t, err)

	unsignedIntentProof, err := intentProof.B64Encode()
	require.NoError(t, err)

	signedIntentProof, err := alice.SignTransaction(ctx, unsignedIntentProof)
	require.NoError(t, err)

	signedIntentProofPsbt, err := psbt.NewFromRawBytes(strings.NewReader(signedIntentProof), true)
	require.NoError(t, err)

	encodedIntentProof, err := signedIntentProofPsbt.B64Encode()
	require.NoError(t, err)

	forfeitOutputAddr, err := btcutil.DecodeAddress(aliceConfig.ForfeitAddress, nil)
	require.NoError(t, err)

	forfeitOutputScript, err := txscript.PayToAddrScript(forfeitOutputAddr)
	require.NoError(t, err)

	connectorAmount := aliceConfig.Dust

	partialForfeitTx, err := tree.BuildForfeitTxWithOutput(
		[]*wire.OutPoint{{
			Hash:  *vtxoHash,
			Index: aliceVtxo.VOut,
		}},
		[]uint32{wire.MaxTxInSequenceNum},
		[]*wire.TxOut{{
			Value:    int64(aliceVtxo.Amount),
			PkScript: delegatePkScript,
		}},
		&wire.TxOut{
			Value:    int64(aliceVtxo.Amount + connectorAmount),
			PkScript: forfeitOutputScript,
		},
		0,
	)
	require.NoError(t, err)

	updater, err := psbt.NewUpdater(partialForfeitTx)
	require.NoError(t, err)
	require.NotNil(t, updater)

	err = updater.AddInSighashType(txscript.SigHashAnyOneCanPay|txscript.SigHashAll, 0)
	require.NoError(t, err)

	aliceDelegatorScript, err := collaborativeAliceDelegatorClosure.Script()
	require.NoError(t, err)

	aliceDelegatorMerkleProof, err := vtxoTapTree.GetTaprootMerkleProof(
		txscript.NewBaseTapLeaf(aliceDelegatorScript).TapHash(),
	)
	require.NoError(t, err)

	aliceDelegatorTapLeafScript := &psbt.TaprootTapLeafScript{
		ControlBlock: aliceDelegatorMerkleProof.ControlBlock,
		Script:       aliceDelegatorMerkleProof.Script,
		LeafVersion:  txscript.BaseLeafVersion,
	}

	updater.Upsbt.Inputs[0].TaprootLeafScript = []*psbt.TaprootTapLeafScript{aliceDelegatorTapLeafScript}

	b64partialForfeitTx, err := updater.Upsbt.B64Encode()
	require.NoError(t, err)

	signedPartialForfeitTx, err := alice.SignTransaction(ctx, b64partialForfeitTx)
	require.NoError(t, err)

	_, err = delegatorClient.Delegate(ctx, &pb.DelegateRequest{
		Intent: &pb.Intent{
			Message: encodedIntentMessage,
			Proof:   encodedIntentProof,
		},
		ForfeitTxs: []string{signedPartialForfeitTx},
	})
	require.NoError(t, err)

	time.Sleep(30 * time.Second)

	spendable, _, err := alice.ListVtxos(ctx)
	require.NoError(t, err)
	require.Len(t, spendable, 1)
	require.Equal(t, int(aliceVtxo.Amount), int(spendable[0].Amount))
	require.False(t, spendable[0].Preconfirmed)
}

func TestDelegateCollaborativeExit(t *testing.T) {
	ctx := t.Context()
	alice, _, alicePubKey, grpcClient := setupArkSDKwithPublicKey(t)
	defer alice.Stop()
	defer grpcClient.Close()

	delegatorClient, err := newDelegatorClient("localhost:7004")
	require.NoError(t, err)
	require.NotNil(t, delegatorClient)

	delegateInfo, err := delegatorClient.GetDelegatorInfo(ctx, &pb.GetDelegatorInfoRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, delegateInfo.GetPubkey())
	require.NotEmpty(t, delegateInfo.GetFee())

	delegatorPubKeyBytes, err := hex.DecodeString(delegateInfo.GetPubkey())
	require.NoError(t, err)
	delegatorPubKey, err := btcec.ParsePubKey(delegatorPubKeyBytes)
	require.NoError(t, err)
	require.NotNil(t, delegatorPubKey)

	_, aliceAddr, boardingAddr, _, err := alice.GetAddresses(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, aliceAddr)

	aliceArkAddr, err := arklib.DecodeAddressV0(aliceAddr[0])
	require.NoError(t, err)
	require.NotNil(t, aliceArkAddr)

	aliceConfig, err := alice.GetConfigData(ctx)
	require.NoError(t, err)

	signerPubKey := aliceConfig.SignerPubKey

	collaborativeAliceDelegatorClosure := &script.MultisigClosure{
		PubKeys: []*btcec.PublicKey{alicePubKey, delegatorPubKey, signerPubKey},
	}

	exitLocktime := arklib.RelativeLocktime{
		Type:  arklib.LocktimeTypeSecond,
		Value: 1024,
	}

	delegationVtxoScript := script.TapscriptsVtxoScript{
		Closures: []script.Closure{
			collaborativeAliceDelegatorClosure,
			&script.MultisigClosure{
				PubKeys: []*btcec.PublicKey{alicePubKey, signerPubKey},
			},
			&script.CSVMultisigClosure{
				Locktime: exitLocktime,
				MultisigClosure: script.MultisigClosure{
					PubKeys: []*btcec.PublicKey{alicePubKey},
				},
			},
		},
	}

	vtxoTapKey, vtxoTapTree, err := delegationVtxoScript.TapTree()
	require.NoError(t, err)

	arkAddress := arklib.Address{
		HRP:        "tark",
		VtxoTapKey: vtxoTapKey,
		Signer:     signerPubKey,
	}

	arkAddressStr, err := arkAddress.EncodeV0()
	require.NoError(t, err)

	faucetOffchain(t, alice, 0.00021)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	var incomingFunds []types.Vtxo
	var incomingErr error
	go func() {
		incomingFunds, incomingErr = alice.NotifyIncomingFunds(ctx, arkAddressStr)
		wg.Done()
	}()
	_, err = alice.SendOffChain(ctx, []types.Receiver{{
		To:     arkAddressStr,
		Amount: 21000,
	}})
	require.NoError(t, err)

	wg.Wait()
	require.NoError(t, incomingErr)
	require.NotEmpty(t, incomingFunds)

	aliceVtxo := incomingFunds[0]

	intentMessage := intent.RegisterMessage{
		BaseMessage: intent.BaseMessage{
			Type: intent.IntentMessageTypeRegister,
		},
		CosignersPublicKeys:  []string{},
		OnchainOutputIndexes: []int{0},
		ValidAt:              time.Now().Add(3 * time.Second).Unix(),
		ExpireAt:             0,
	}

	encodedIntentMessage, err := intentMessage.Encode()
	require.NoError(t, err)

	vtxoHash, err := chainhash.NewHashFromStr(aliceVtxo.Txid)
	require.NoError(t, err)

	exitScript, err := delegationVtxoScript.ExitClosures()[0].Script()
	require.NoError(t, err)

	exitScriptMerkleProof, err := vtxoTapTree.GetTaprootMerkleProof(
		txscript.NewBaseTapLeaf(exitScript).TapHash(),
	)
	require.NoError(t, err)

	sequence, err := arklib.BIP68Sequence(exitLocktime)
	require.NoError(t, err)

	delegatePkScript, err := arkAddress.GetPkScript()
	require.NoError(t, err)

	boardingAddress, err := btcutil.DecodeAddress(boardingAddr[0], &chaincfg.RegressionNetParams)
	require.NoError(t, err)
	require.NotNil(t, boardingAddress)

	exitPkScript, err := txscript.PayToAddrScript(boardingAddress)
	require.NoError(t, err)

	intentProof, err := intent.New(
		encodedIntentMessage,
		[]intent.Input{
			{
				OutPoint: &wire.OutPoint{
					Hash:  *vtxoHash,
					Index: aliceVtxo.VOut,
				},
				Sequence: sequence,
				WitnessUtxo: &wire.TxOut{
					Value:    int64(aliceVtxo.Amount),
					PkScript: delegatePkScript,
				},
			},
		},
		[]*wire.TxOut{
			{
				Value:    int64(aliceVtxo.Amount),
				PkScript: exitPkScript,
			},
		},
	)
	require.NoError(t, err)

	tapLeafScript := &psbt.TaprootTapLeafScript{
		ControlBlock: exitScriptMerkleProof.ControlBlock,
		Script:       exitScriptMerkleProof.Script,
		LeafVersion:  txscript.BaseLeafVersion,
	}

	intentProof.Inputs[0].TaprootLeafScript = []*psbt.TaprootTapLeafScript{tapLeafScript}
	intentProof.Inputs[1].TaprootLeafScript = []*psbt.TaprootTapLeafScript{tapLeafScript}

	scripts, err := delegationVtxoScript.Encode()
	require.NoError(t, err)

	tapTree := txutils.TapTree(scripts)

	err = txutils.SetArkPsbtField(&intentProof.Packet, 1, txutils.VtxoTaprootTreeField, tapTree)
	require.NoError(t, err)

	unsignedIntentProof, err := intentProof.B64Encode()
	require.NoError(t, err)

	signedIntentProof, err := alice.SignTransaction(ctx, unsignedIntentProof)
	require.NoError(t, err)

	signedIntentProofPsbt, err := psbt.NewFromRawBytes(strings.NewReader(signedIntentProof), true)
	require.NoError(t, err)

	encodedIntentProof, err := signedIntentProofPsbt.B64Encode()
	require.NoError(t, err)

	forfeitOutputAddr, err := btcutil.DecodeAddress(aliceConfig.ForfeitAddress, nil)
	require.NoError(t, err)

	forfeitOutputScript, err := txscript.PayToAddrScript(forfeitOutputAddr)
	require.NoError(t, err)

	connectorAmount := aliceConfig.Dust

	partialForfeitTx, err := tree.BuildForfeitTxWithOutput(
		[]*wire.OutPoint{{
			Hash:  *vtxoHash,
			Index: aliceVtxo.VOut,
		}},
		[]uint32{wire.MaxTxInSequenceNum},
		[]*wire.TxOut{{
			Value:    int64(aliceVtxo.Amount),
			PkScript: delegatePkScript,
		}},
		&wire.TxOut{
			Value:    int64(aliceVtxo.Amount + connectorAmount),
			PkScript: forfeitOutputScript,
		},
		0,
	)
	require.NoError(t, err)

	updater, err := psbt.NewUpdater(partialForfeitTx)
	require.NoError(t, err)
	require.NotNil(t, updater)

	err = updater.AddInSighashType(txscript.SigHashAnyOneCanPay|txscript.SigHashAll, 0)
	require.NoError(t, err)

	aliceDelegatorScript, err := collaborativeAliceDelegatorClosure.Script()
	require.NoError(t, err)

	aliceDelegatorMerkleProof, err := vtxoTapTree.GetTaprootMerkleProof(
		txscript.NewBaseTapLeaf(aliceDelegatorScript).TapHash(),
	)
	require.NoError(t, err)

	aliceDelegatorTapLeafScript := &psbt.TaprootTapLeafScript{
		ControlBlock: aliceDelegatorMerkleProof.ControlBlock,
		Script:       aliceDelegatorMerkleProof.Script,
		LeafVersion:  txscript.BaseLeafVersion,
	}

	updater.Upsbt.Inputs[0].TaprootLeafScript = []*psbt.TaprootTapLeafScript{aliceDelegatorTapLeafScript}

	b64partialForfeitTx, err := updater.Upsbt.B64Encode()
	require.NoError(t, err)

	signedPartialForfeitTx, err := alice.SignTransaction(ctx, b64partialForfeitTx)
	require.NoError(t, err)

	_, err = delegatorClient.Delegate(ctx, &pb.DelegateRequest{
		Intent: &pb.Intent{
			Message: encodedIntentMessage,
			Proof:   encodedIntentProof,
		},
		ForfeitTxs: []string{signedPartialForfeitTx},
	})
	require.NoError(t, err)

	time.Sleep(30 * time.Second)

	balance, err := alice.Balance(t.Context())
	require.NoError(t, err)
	require.Len(t, balance.OnchainBalance.LockedAmount, 1)
	require.Equal(t, int(aliceVtxo.Amount), int(balance.OnchainBalance.LockedAmount[0].Amount))
}

// TestMultipleDelegate delegate the renewal of multiple vtxos at once using different intents.
func TestMultipleDelegate(t *testing.T) {
	ctx := t.Context()
	alice, _, alicePubKey, grpcClient := setupArkSDKwithPublicKey(t)
	defer alice.Stop()
	defer grpcClient.Close()

	delegatorClient, err := newDelegatorClient("localhost:7004")
	require.NoError(t, err)
	require.NotNil(t, delegatorClient)

	delegateInfo, err := delegatorClient.GetDelegatorInfo(ctx, &pb.GetDelegatorInfoRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, delegateInfo.GetPubkey())
	require.NotEmpty(t, delegateInfo.GetFee())

	delegatorPubKeyBytes, err := hex.DecodeString(delegateInfo.GetPubkey())
	require.NoError(t, err)
	delegatorPubKey, err := btcec.ParsePubKey(delegatorPubKeyBytes)
	require.NoError(t, err)
	require.NotNil(t, delegatorPubKey)

	_, aliceAddr, _, _, err := alice.GetAddresses(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, aliceAddr)

	aliceArkAddr, err := arklib.DecodeAddressV0(aliceAddr[0])
	require.NoError(t, err)
	require.NotNil(t, aliceArkAddr)

	aliceConfig, err := alice.GetConfigData(ctx)
	require.NoError(t, err)

	signerPubKey := aliceConfig.SignerPubKey

	collaborativeAliceDelegatorClosure := &script.MultisigClosure{
		PubKeys: []*btcec.PublicKey{alicePubKey, delegatorPubKey, signerPubKey},
	}

	exitLocktime := arklib.RelativeLocktime{
		Type:  arklib.LocktimeTypeSecond,
		Value: 1024,
	}

	delegationVtxoScript := script.TapscriptsVtxoScript{
		Closures: []script.Closure{
			collaborativeAliceDelegatorClosure,
			&script.MultisigClosure{
				PubKeys: []*btcec.PublicKey{alicePubKey, signerPubKey},
			},
			&script.CSVMultisigClosure{
				Locktime: exitLocktime,
				MultisigClosure: script.MultisigClosure{
					PubKeys: []*btcec.PublicKey{alicePubKey},
				},
			},
		},
	}

	vtxoTapKey, vtxoTapTree, err := delegationVtxoScript.TapTree()
	require.NoError(t, err)

	arkAddress := arklib.Address{
		HRP:        "tark",
		VtxoTapKey: vtxoTapKey,
		Signer:     signerPubKey,
	}

	arkAddressStr, err := arkAddress.EncodeV0()
	require.NoError(t, err)

	faucetOffchain(t, alice, 0.0021) // 10 * 0.00021

	delegatePkScript, err := arkAddress.GetPkScript()
	require.NoError(t, err)

	alicePkScript, err := aliceArkAddr.GetPkScript()
	require.NoError(t, err)

	exitScript, err := delegationVtxoScript.ExitClosures()[0].Script()
	require.NoError(t, err)

	exitScriptMerkleProof, err := vtxoTapTree.GetTaprootMerkleProof(
		txscript.NewBaseTapLeaf(exitScript).TapHash(),
	)
	require.NoError(t, err)

	sequence, err := arklib.BIP68Sequence(exitLocktime)
	require.NoError(t, err)

	scripts, err := delegationVtxoScript.Encode()
	require.NoError(t, err)

	tapTree := txutils.TapTree(scripts)

	aliceDelegatorScript, err := collaborativeAliceDelegatorClosure.Script()
	require.NoError(t, err)

	aliceDelegatorMerkleProof, err := vtxoTapTree.GetTaprootMerkleProof(
		txscript.NewBaseTapLeaf(aliceDelegatorScript).TapHash(),
	)
	require.NoError(t, err)

	aliceDelegatorTapLeafScript := &psbt.TaprootTapLeafScript{
		ControlBlock: aliceDelegatorMerkleProof.ControlBlock,
		Script:       aliceDelegatorMerkleProof.Script,
		LeafVersion:  txscript.BaseLeafVersion,
	}

	forfeitOutputAddr, err := btcutil.DecodeAddress(aliceConfig.ForfeitAddress, nil)
	require.NoError(t, err)

	forfeitOutputScript, err := txscript.PayToAddrScript(forfeitOutputAddr)
	require.NoError(t, err)

	connectorAmount := aliceConfig.Dust

	const numVtxos = 10
	vtxos := make([]types.Vtxo, 0, numVtxos)

	for range numVtxos {
		wg := &sync.WaitGroup{}
		wg.Add(1)
		var incomingFunds []types.Vtxo
		var incomingErr error
		go func() {
			incomingFunds, incomingErr = alice.NotifyIncomingFunds(ctx, arkAddressStr)
			wg.Done()
		}()
		_, err = alice.SendOffChain(ctx, []types.Receiver{{
			To:     arkAddressStr,
			Amount: 21000,
		}})
		require.NoError(t, err)

		wg.Wait()
		require.NoError(t, incomingErr)
		require.NotEmpty(t, incomingFunds)
		vtxos = append(vtxos, incomingFunds[0])
	}

	delegateRequests := make([]*pb.DelegateRequest, 0, numVtxos)

	for _, aliceVtxo := range vtxos {
		vtxoHash, err := chainhash.NewHashFromStr(aliceVtxo.Txid)
		require.NoError(t, err)

		intentMessage := intent.RegisterMessage{
			BaseMessage: intent.BaseMessage{
				Type: intent.IntentMessageTypeRegister,
			},
			CosignersPublicKeys: []string{delegateInfo.GetPubkey()},
			ValidAt:             time.Now().Add(3 * time.Second).Unix(),
			ExpireAt:            0,
		}

		encodedIntentMessage, err := intentMessage.Encode()
		require.NoError(t, err)

		intentProof, err := intent.New(
			encodedIntentMessage,
			[]intent.Input{
				{
					OutPoint: &wire.OutPoint{
						Hash:  *vtxoHash,
						Index: aliceVtxo.VOut,
					},
					Sequence: sequence,
					WitnessUtxo: &wire.TxOut{
						Value:    int64(aliceVtxo.Amount),
						PkScript: delegatePkScript,
					},
				},
			},
			[]*wire.TxOut{
				{
					Value:    int64(aliceVtxo.Amount),
					PkScript: alicePkScript,
				},
			},
		)
		require.NoError(t, err)

		tapLeafScript := &psbt.TaprootTapLeafScript{
			ControlBlock: exitScriptMerkleProof.ControlBlock,
			Script:       exitScriptMerkleProof.Script,
			LeafVersion:  txscript.BaseLeafVersion,
		}

		intentProof.Inputs[0].TaprootLeafScript = []*psbt.TaprootTapLeafScript{tapLeafScript}
		intentProof.Inputs[1].TaprootLeafScript = []*psbt.TaprootTapLeafScript{tapLeafScript}

		err = txutils.SetArkPsbtField(&intentProof.Packet, 1, txutils.VtxoTaprootTreeField, tapTree)
		require.NoError(t, err)

		unsignedIntentProof, err := intentProof.B64Encode()
		require.NoError(t, err)

		signedIntentProof, err := alice.SignTransaction(ctx, unsignedIntentProof)
		require.NoError(t, err)

		signedIntentProofPsbt, err := psbt.NewFromRawBytes(strings.NewReader(signedIntentProof), true)
		require.NoError(t, err)

		encodedIntentProof, err := signedIntentProofPsbt.B64Encode()
		require.NoError(t, err)

		partialForfeitTx, err := tree.BuildForfeitTxWithOutput(
			[]*wire.OutPoint{{
				Hash:  *vtxoHash,
				Index: aliceVtxo.VOut,
			}},
			[]uint32{wire.MaxTxInSequenceNum},
			[]*wire.TxOut{{
				Value:    int64(aliceVtxo.Amount),
				PkScript: delegatePkScript,
			}},
			&wire.TxOut{
				Value:    int64(aliceVtxo.Amount + connectorAmount),
				PkScript: forfeitOutputScript,
			},
			0,
		)
		require.NoError(t, err)

		updater, err := psbt.NewUpdater(partialForfeitTx)
		require.NoError(t, err)
		require.NotNil(t, updater)

		err = updater.AddInSighashType(txscript.SigHashAnyOneCanPay|txscript.SigHashAll, 0)
		require.NoError(t, err)

		updater.Upsbt.Inputs[0].TaprootLeafScript = []*psbt.TaprootTapLeafScript{aliceDelegatorTapLeafScript}

		b64partialForfeitTx, err := updater.Upsbt.B64Encode()
		require.NoError(t, err)

		signedPartialForfeitTx, err := alice.SignTransaction(ctx, b64partialForfeitTx)
		require.NoError(t, err)

		delegateRequests = append(delegateRequests, &pb.DelegateRequest{
			Intent: &pb.Intent{
				Message: encodedIntentMessage,
				Proof:   encodedIntentProof,
			},
			ForfeitTxs: []string{signedPartialForfeitTx},
		})
	}

	for i, req := range delegateRequests {
		_, err = delegatorClient.Delegate(ctx, req)
		require.NoError(t, err, "failed to delegate vtxo %d", i+1)
	}

	time.Sleep(30 * time.Second)

	spendable, _, err := alice.ListVtxos(ctx)
	require.NoError(t, err)
	require.Len(t, spendable, numVtxos, "expected %d refreshed vtxos", numVtxos)

	for i, vtxo := range spendable {
		require.False(t, vtxo.Preconfirmed, "vtxo %d should not be preconfirmed", i+1)
		require.Equal(t, 21000, int(vtxo.Amount), "vtxo %d should have amount 21000", i+1)
	}
}

// TestDelegateSameInput tests the case where a delegate task with the same input is already pending.
// The delegator should cancel the old task and process the new one instead of returning an error.
func TestDelegateSameInput(t *testing.T) {
	ctx := t.Context()
	alice, _, alicePubKey, grpcClient := setupArkSDKwithPublicKey(t)
	defer alice.Stop()
	defer grpcClient.Close()

	delegatorClient, err := newDelegatorClient("localhost:7004")
	require.NoError(t, err)
	require.NotNil(t, delegatorClient)

	delegateInfo, err := delegatorClient.GetDelegatorInfo(ctx, &pb.GetDelegatorInfoRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, delegateInfo.GetPubkey())
	require.NotEmpty(t, delegateInfo.GetFee())

	delegatorPubKeyBytes, err := hex.DecodeString(delegateInfo.GetPubkey())
	require.NoError(t, err)
	delegatorPubKey, err := btcec.ParsePubKey(delegatorPubKeyBytes)
	require.NoError(t, err)
	require.NotNil(t, delegatorPubKey)

	_, aliceAddr, _, _, err := alice.GetAddresses(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, aliceAddr)

	aliceArkAddr, err := arklib.DecodeAddressV0(aliceAddr[0])
	require.NoError(t, err)
	require.NotNil(t, aliceArkAddr)

	aliceConfig, err := alice.GetConfigData(ctx)
	require.NoError(t, err)

	signerPubKey := aliceConfig.SignerPubKey

	collaborativeAliceDelegatorClosure := &script.MultisigClosure{
		PubKeys: []*btcec.PublicKey{alicePubKey, delegatorPubKey, signerPubKey},
	}

	exitLocktime := arklib.RelativeLocktime{
		Type:  arklib.LocktimeTypeSecond,
		Value: 1024,
	}

	delegationVtxoScript := script.TapscriptsVtxoScript{
		Closures: []script.Closure{
			collaborativeAliceDelegatorClosure,
			&script.MultisigClosure{
				PubKeys: []*btcec.PublicKey{alicePubKey, signerPubKey},
			},
			&script.CSVMultisigClosure{
				Locktime: exitLocktime,
				MultisigClosure: script.MultisigClosure{
					PubKeys: []*btcec.PublicKey{alicePubKey},
				},
			},
		},
	}

	vtxoTapKey, vtxoTapTree, err := delegationVtxoScript.TapTree()
	require.NoError(t, err)

	arkAddress := arklib.Address{
		HRP:        "tark",
		VtxoTapKey: vtxoTapKey,
		Signer:     signerPubKey,
	}

	arkAddressStr, err := arkAddress.EncodeV0()
	require.NoError(t, err)

	faucetOffchain(t, alice, 0.00021)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	var incomingFunds []types.Vtxo
	var incomingErr error
	go func() {
		incomingFunds, incomingErr = alice.NotifyIncomingFunds(ctx, arkAddressStr)
		wg.Done()
	}()
	_, err = alice.SendOffChain(ctx, []types.Receiver{{
		To:     arkAddressStr,
		Amount: 21000,
	}})
	require.NoError(t, err)

	wg.Wait()
	require.NoError(t, incomingErr)
	require.NotEmpty(t, incomingFunds)

	aliceVtxo := incomingFunds[0]

	vtxoHash, err := chainhash.NewHashFromStr(aliceVtxo.Txid)
	require.NoError(t, err)

	exitScript, err := delegationVtxoScript.ExitClosures()[0].Script()
	require.NoError(t, err)

	exitScriptMerkleProof, err := vtxoTapTree.GetTaprootMerkleProof(
		txscript.NewBaseTapLeaf(exitScript).TapHash(),
	)
	require.NoError(t, err)

	sequence, err := arklib.BIP68Sequence(exitLocktime)
	require.NoError(t, err)

	delegatePkScript, err := arkAddress.GetPkScript()
	require.NoError(t, err)

	alicePkScript, err := aliceArkAddr.GetPkScript()
	require.NoError(t, err)

	createDelegateRequest := func() (*pb.DelegateRequest, error) {
		intentMessage := intent.RegisterMessage{
			BaseMessage: intent.BaseMessage{
				Type: intent.IntentMessageTypeRegister,
			},
			CosignersPublicKeys: []string{delegateInfo.GetPubkey()},
			ValidAt:             time.Now().Add(3 * time.Second).Unix(),
			ExpireAt:            0,
		}

		encodedIntentMessage, err := intentMessage.Encode()
		if err != nil {
			return nil, err
		}

		intentProof, err := intent.New(
			encodedIntentMessage,
			[]intent.Input{
				{
					OutPoint: &wire.OutPoint{
						Hash:  *vtxoHash,
						Index: aliceVtxo.VOut,
					},
					Sequence: sequence,
					WitnessUtxo: &wire.TxOut{
						Value:    int64(aliceVtxo.Amount),
						PkScript: delegatePkScript,
					},
				},
			},
			[]*wire.TxOut{
				{
					Value:    int64(aliceVtxo.Amount),
					PkScript: alicePkScript,
				},
			},
		)
		if err != nil {
			return nil, err
		}

		tapLeafScript := &psbt.TaprootTapLeafScript{
			ControlBlock: exitScriptMerkleProof.ControlBlock,
			Script:       exitScriptMerkleProof.Script,
			LeafVersion:  txscript.BaseLeafVersion,
		}

		intentProof.Inputs[0].TaprootLeafScript = []*psbt.TaprootTapLeafScript{tapLeafScript}
		intentProof.Inputs[1].TaprootLeafScript = []*psbt.TaprootTapLeafScript{tapLeafScript}

		scripts, err := delegationVtxoScript.Encode()
		if err != nil {
			return nil, err
		}

		tapTree := txutils.TapTree(scripts)

		err = txutils.SetArkPsbtField(&intentProof.Packet, 1, txutils.VtxoTaprootTreeField, tapTree)
		if err != nil {
			return nil, err
		}

		unsignedIntentProof, err := intentProof.B64Encode()
		if err != nil {
			return nil, err
		}

		signedIntentProof, err := alice.SignTransaction(ctx, unsignedIntentProof)
		if err != nil {
			return nil, err
		}

		signedIntentProofPsbt, err := psbt.NewFromRawBytes(strings.NewReader(signedIntentProof), true)
		if err != nil {
			return nil, err
		}

		encodedIntentProof, err := signedIntentProofPsbt.B64Encode()
		if err != nil {
			return nil, err
		}

		forfeitOutputAddr, err := btcutil.DecodeAddress(aliceConfig.ForfeitAddress, nil)
		if err != nil {
			return nil, err
		}

		forfeitOutputScript, err := txscript.PayToAddrScript(forfeitOutputAddr)
		if err != nil {
			return nil, err
		}

		connectorAmount := aliceConfig.Dust

		partialForfeitTx, err := tree.BuildForfeitTxWithOutput(
			[]*wire.OutPoint{{
				Hash:  *vtxoHash,
				Index: aliceVtxo.VOut,
			}},
			[]uint32{wire.MaxTxInSequenceNum},
			[]*wire.TxOut{{
				Value:    int64(aliceVtxo.Amount),
				PkScript: delegatePkScript,
			}},
			&wire.TxOut{
				Value:    int64(aliceVtxo.Amount + connectorAmount),
				PkScript: forfeitOutputScript,
			},
			0,
		)
		if err != nil {
			return nil, err
		}

		updater, err := psbt.NewUpdater(partialForfeitTx)
		if err != nil {
			return nil, err
		}

		err = updater.AddInSighashType(txscript.SigHashAnyOneCanPay|txscript.SigHashAll, 0)
		if err != nil {
			return nil, err
		}

		aliceDelegatorScript, err := collaborativeAliceDelegatorClosure.Script()
		if err != nil {
			return nil, err
		}

		aliceDelegatorMerkleProof, err := vtxoTapTree.GetTaprootMerkleProof(
			txscript.NewBaseTapLeaf(aliceDelegatorScript).TapHash(),
		)
		if err != nil {
			return nil, err
		}

		aliceDelegatorTapLeafScript := &psbt.TaprootTapLeafScript{
			ControlBlock: aliceDelegatorMerkleProof.ControlBlock,
			Script:       aliceDelegatorMerkleProof.Script,
			LeafVersion:  txscript.BaseLeafVersion,
		}

		updater.Upsbt.Inputs[0].TaprootLeafScript = []*psbt.TaprootTapLeafScript{aliceDelegatorTapLeafScript}

		b64partialForfeitTx, err := updater.Upsbt.B64Encode()
		if err != nil {
			return nil, err
		}

		signedPartialForfeitTx, err := alice.SignTransaction(ctx, b64partialForfeitTx)
		if err != nil {
			return nil, err
		}

		return &pb.DelegateRequest{
			Intent: &pb.Intent{
				Message: encodedIntentMessage,
				Proof:   encodedIntentProof,
			},
			ForfeitTxs: []string{signedPartialForfeitTx},
		}, nil
	}

	firstRequest, err := createDelegateRequest()
	require.NoError(t, err)

	_, err = delegatorClient.Delegate(ctx, firstRequest)
	require.NoError(t, err)

	secondRequest, err := createDelegateRequest()
	require.NoError(t, err)

	// the second delegation should succeed - it will cancel the old task and process the new one
	_, err = delegatorClient.Delegate(ctx, secondRequest)
	require.NoError(t, err)

	time.Sleep(30 * time.Second)

	spendable, _, err := alice.ListVtxos(ctx)
	require.NoError(t, err)
	require.Len(t, spendable, 1)
	require.Equal(t, int(aliceVtxo.Amount), int(spendable[0].Amount))
	require.False(t, spendable[0].Preconfirmed)
}

// TestDelegateSeveralInputs tests delegating multiple inputs in a single intent.
// including a subdust (recoverable) coin.
func TestDelegateSeveralInputs(t *testing.T) {
	ctx := t.Context()
	alice, _, alicePubKey, grpcClient := setupArkSDKwithPublicKey(t)
	defer alice.Stop()
	defer grpcClient.Close()

	delegatorClient, err := newDelegatorClient("localhost:7004")
	require.NoError(t, err)
	require.NotNil(t, delegatorClient)

	delegateInfo, err := delegatorClient.GetDelegatorInfo(ctx, &pb.GetDelegatorInfoRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, delegateInfo.GetPubkey())
	require.NotEmpty(t, delegateInfo.GetFee())

	delegatorPubKeyBytes, err := hex.DecodeString(delegateInfo.GetPubkey())
	require.NoError(t, err)
	delegatorPubKey, err := btcec.ParsePubKey(delegatorPubKeyBytes)
	require.NoError(t, err)
	require.NotNil(t, delegatorPubKey)

	_, aliceAddr, _, _, err := alice.GetAddresses(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, aliceAddr)

	aliceArkAddr, err := arklib.DecodeAddressV0(aliceAddr[0])
	require.NoError(t, err)
	require.NotNil(t, aliceArkAddr)

	aliceConfig, err := alice.GetConfigData(ctx)
	require.NoError(t, err)

	signerPubKey := aliceConfig.SignerPubKey

	collaborativeAliceDelegatorClosure := &script.MultisigClosure{
		PubKeys: []*btcec.PublicKey{alicePubKey, delegatorPubKey, signerPubKey},
	}

	exitLocktime := arklib.RelativeLocktime{
		Type:  arklib.LocktimeTypeSecond,
		Value: 1024,
	}

	delegationVtxoScript := script.TapscriptsVtxoScript{
		Closures: []script.Closure{
			collaborativeAliceDelegatorClosure,
			&script.MultisigClosure{
				PubKeys: []*btcec.PublicKey{alicePubKey, signerPubKey},
			},
			&script.CSVMultisigClosure{
				Locktime: exitLocktime,
				MultisigClosure: script.MultisigClosure{
					PubKeys: []*btcec.PublicKey{alicePubKey},
				},
			},
		},
	}

	vtxoTapKey, vtxoTapTree, err := delegationVtxoScript.TapTree()
	require.NoError(t, err)

	arkAddress := arklib.Address{
		HRP:        "tark",
		VtxoTapKey: vtxoTapKey,
		Signer:     signerPubKey,
	}

	arkAddressStr, err := arkAddress.EncodeV0()
	require.NoError(t, err)

	const numVtxos = 5
	faucetOffchain(t, alice, 0.00105) // 5 * 0.00021

	delegatePkScript, err := arkAddress.GetPkScript()
	require.NoError(t, err)

	alicePkScript, err := aliceArkAddr.GetPkScript()
	require.NoError(t, err)

	exitScript, err := delegationVtxoScript.ExitClosures()[0].Script()
	require.NoError(t, err)

	exitScriptMerkleProof, err := vtxoTapTree.GetTaprootMerkleProof(
		txscript.NewBaseTapLeaf(exitScript).TapHash(),
	)
	require.NoError(t, err)

	sequence, err := arklib.BIP68Sequence(exitLocktime)
	require.NoError(t, err)

	scripts, err := delegationVtxoScript.Encode()
	require.NoError(t, err)

	tapTree := txutils.TapTree(scripts)

	aliceDelegatorScript, err := collaborativeAliceDelegatorClosure.Script()
	require.NoError(t, err)

	aliceDelegatorMerkleProof, err := vtxoTapTree.GetTaprootMerkleProof(
		txscript.NewBaseTapLeaf(aliceDelegatorScript).TapHash(),
	)
	require.NoError(t, err)

	aliceDelegatorTapLeafScript := &psbt.TaprootTapLeafScript{
		ControlBlock: aliceDelegatorMerkleProof.ControlBlock,
		Script:       aliceDelegatorMerkleProof.Script,
		LeafVersion:  txscript.BaseLeafVersion,
	}

	forfeitOutputAddr, err := btcutil.DecodeAddress(aliceConfig.ForfeitAddress, nil)
	require.NoError(t, err)

	forfeitOutputScript, err := txscript.PayToAddrScript(forfeitOutputAddr)
	require.NoError(t, err)

	connectorAmount := aliceConfig.Dust

	vtxos := make([]types.Vtxo, 0, numVtxos)

	for i := 0; i < numVtxos; i++ {
		wg := &sync.WaitGroup{}
		wg.Add(1)
		var incomingFunds []types.Vtxo
		var incomingErr error
		go func() {
			incomingFunds, incomingErr = alice.NotifyIncomingFunds(ctx, arkAddressStr)
			wg.Done()
		}()
		amount := 21000
		if i == numVtxos-1 {
			amount = 50 // subdust vtxo
		}
		_, err = alice.SendOffChain(ctx, []types.Receiver{{
			To:     arkAddressStr,
			Amount: uint64(amount),
		}})
		require.NoError(t, err)

		wg.Wait()
		require.NoError(t, incomingErr)
		require.NotEmpty(t, incomingFunds)
		vtxos = append(vtxos, incomingFunds[0])
	}

	intentInputs := make([]intent.Input, 0, numVtxos)
	totalAmount := int64(0)

	for _, aliceVtxo := range vtxos {
		vtxoHash, err := chainhash.NewHashFromStr(aliceVtxo.Txid)
		require.NoError(t, err)

		intentInputs = append(intentInputs, intent.Input{
			OutPoint: &wire.OutPoint{
				Hash:  *vtxoHash,
				Index: aliceVtxo.VOut,
			},
			Sequence: sequence,
			WitnessUtxo: &wire.TxOut{
				Value:    int64(aliceVtxo.Amount),
				PkScript: delegatePkScript,
			},
		})
		totalAmount += int64(aliceVtxo.Amount)
	}

	intentMessage := intent.RegisterMessage{
		BaseMessage: intent.BaseMessage{
			Type: intent.IntentMessageTypeRegister,
		},
		CosignersPublicKeys: []string{delegateInfo.GetPubkey()},
		ValidAt:             time.Now().Add(3 * time.Second).Unix(),
		ExpireAt:            0,
	}

	encodedIntentMessage, err := intentMessage.Encode()
	require.NoError(t, err)

	delegatorAddr, err := arklib.DecodeAddressV0(delegateInfo.GetDelegatorAddress())
	require.NoError(t, err)
	delegatorPkScript, err := delegatorAddr.GetPkScript()
	require.NoError(t, err)

	requiredFee, err := strconv.ParseUint(delegateInfo.GetFee(), 10, 64)
	require.NoError(t, err)

	aliceOutputAmount := totalAmount - int64(requiredFee)
	require.Greater(t, aliceOutputAmount, int64(0), "alice output amount must be positive after fee")

	intentOutputs := []*wire.TxOut{
		{
			Value:    aliceOutputAmount,
			PkScript: alicePkScript,
		},
	}

	if requiredFee > 0 {
		intentOutputs = append(intentOutputs, &wire.TxOut{
			Value:    int64(requiredFee),
			PkScript: delegatorPkScript,
		})
	}

	intentProof, err := intent.New(
		encodedIntentMessage,
		intentInputs,
		intentOutputs,
	)
	require.NoError(t, err)

	tapLeafScript := &psbt.TaprootTapLeafScript{
		ControlBlock: exitScriptMerkleProof.ControlBlock,
		Script:       exitScriptMerkleProof.Script,
		LeafVersion:  txscript.BaseLeafVersion,
	}

	for i := range intentProof.Inputs {
		intentProof.Inputs[i].TaprootLeafScript = []*psbt.TaprootTapLeafScript{tapLeafScript}
		err = txutils.SetArkPsbtField(&intentProof.Packet, i, txutils.VtxoTaprootTreeField, tapTree)
		require.NoError(t, err)
	}

	unsignedIntentProof, err := intentProof.B64Encode()
	require.NoError(t, err)

	signedIntentProof, err := alice.SignTransaction(ctx, unsignedIntentProof)
	require.NoError(t, err)

	signedIntentProofPsbt, err := psbt.NewFromRawBytes(strings.NewReader(signedIntentProof), true)
	require.NoError(t, err)

	encodedIntentProof, err := signedIntentProofPsbt.B64Encode()
	require.NoError(t, err)

	forfeits := make([]string, 0, numVtxos)
	for _, aliceVtxo := range vtxos {
		if aliceVtxo.Amount < 330 {
			continue // no need to forfeit subdust vtxo
		}
		vtxoHash, err := chainhash.NewHashFromStr(aliceVtxo.Txid)
		require.NoError(t, err)

		partialForfeitTx, err := tree.BuildForfeitTxWithOutput(
			[]*wire.OutPoint{{
				Hash:  *vtxoHash,
				Index: aliceVtxo.VOut,
			}},
			[]uint32{wire.MaxTxInSequenceNum},
			[]*wire.TxOut{{
				Value:    int64(aliceVtxo.Amount),
				PkScript: delegatePkScript,
			}},
			&wire.TxOut{
				Value:    int64(aliceVtxo.Amount + connectorAmount),
				PkScript: forfeitOutputScript,
			},
			0,
		)
		require.NoError(t, err)

		updater, err := psbt.NewUpdater(partialForfeitTx)
		require.NoError(t, err)
		require.NotNil(t, updater)

		err = updater.AddInSighashType(txscript.SigHashAnyOneCanPay|txscript.SigHashAll, 0)
		require.NoError(t, err)

		updater.Upsbt.Inputs[0].TaprootLeafScript = []*psbt.TaprootTapLeafScript{aliceDelegatorTapLeafScript}

		b64partialForfeitTx, err := updater.Upsbt.B64Encode()
		require.NoError(t, err)

		signedPartialForfeitTx, err := alice.SignTransaction(ctx, b64partialForfeitTx)
		require.NoError(t, err)

		forfeits = append(forfeits, signedPartialForfeitTx)
	}

	_, err = delegatorClient.Delegate(ctx, &pb.DelegateRequest{
		Intent: &pb.Intent{
			Message: encodedIntentMessage,
			Proof:   encodedIntentProof,
		},
		ForfeitTxs: forfeits,
	})
	require.NoError(t, err)

	time.Sleep(30 * time.Second)

	spendable, _, err := alice.ListVtxos(ctx)
	require.NoError(t, err)
	require.Len(t, spendable, 2)

	var leafVtxo *types.Vtxo
	for _, vtxo := range spendable {
		if !vtxo.Preconfirmed {
			leafVtxo = &vtxo
			break
		}
	}
	require.NotNil(t, leafVtxo)
	require.Equal(t, int(aliceOutputAmount), int(leafVtxo.Amount))
}
