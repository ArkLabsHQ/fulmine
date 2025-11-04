package main

import (
	"encoding/hex"

	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

func main() {
	outputScripts, _ := hex.DecodeString("3c9fcacc24fc39fcbf7c80c62d97efb4e1465e1b767836bae10f1e2f99b9413f")
	signerPubkey, _ := hex.DecodeString("03abd7ac8dcfcc6e4ceed284288a568ea03d7403850e8dcf2a88cb9ec7d2718bd7")

	signerParsedPubkey, _ := btcec.ParsePubKey(signerPubkey)
	outputPubkey, _ := schnorr.ParsePubKey(outputScripts)

	addr := arklib.Address{
		Version:    0,
		HRP:        "tark",
		Signer:     signerParsedPubkey,
		VtxoTapKey: outputPubkey,
	}

	encodedAddr, _ := addr.EncodeV0()

	println(encodedAddr)

}
