package application

import (
	"encoding/hex"
	"fmt"

	arklib "github.com/arkade-os/arkd/pkg/ark-lib"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
)

func offchainAddressesPkScripts(addresses []string) ([]string, error) {
	scripts := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		decodedAddress, err := arklib.DecodeAddressV0(addr)
		if err != nil {
			return nil, fmt.Errorf("failed to decode address %s: %w", addr, err)
		}

		p2trScript, err := txscript.PayToTaprootScript(decodedAddress.VtxoTapKey)
		if err != nil {
			return nil, fmt.Errorf("failed to parse address to p2tr script: %w", err)
		}

		scripts = append(scripts, hex.EncodeToString(p2trScript))
	}
	return scripts, nil
}

func onchainAddressesPkScripts(addresses []string, network arklib.Network) ([]string, error) {
	scripts := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		btcAddress, err := btcutil.DecodeAddress(addr, toBitcoinNetwork(network))
		if err != nil {
			return nil, fmt.Errorf("failed to decode address %s: %w", addr, err)
		}

		script, err := txscript.PayToAddrScript(btcAddress)
		if err != nil {
			return nil, fmt.Errorf("failed to parse address to p2tr script: %w", err)
		}
		scripts = append(scripts, hex.EncodeToString(script))
	}
	return scripts, nil
}

func toBitcoinNetwork(net arklib.Network) *chaincfg.Params {
	switch net.Name {
	case arklib.Bitcoin.Name:
		return &chaincfg.MainNetParams
	case arklib.BitcoinTestNet.Name:
		return &chaincfg.TestNet3Params
	//case arklib.BitcoinTestNet4.Name: //TODO uncomment once supported
	//	return chaincfg.TestNet4Params
	case arklib.BitcoinSigNet.Name:
		return &chaincfg.SigNetParams
	case arklib.BitcoinMutinyNet.Name:
		return &arklib.MutinyNetSigNetParams
	case arklib.BitcoinRegTest.Name:
		return &chaincfg.RegressionNetParams
	default:
		return &chaincfg.MainNetParams
	}
}
