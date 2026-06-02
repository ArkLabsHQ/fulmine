package application

import (
	"encoding/hex"
	"fmt"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/pkg/vhtlc"
	"github.com/arkade-os/arkd/pkg/ark-lib/extension"
	"github.com/arkade-os/arkd/pkg/ark-lib/script"
	"github.com/arkade-os/arkd/pkg/ark-lib/txutils"
)

// findExtraPacketsForReceivers returns the extension packets and per-output
// tap trees needed to fund VHTLCs at the given destination addresses. The
// tap tree map is keyed by hex-encoded P2TR pkScript so that client-lib's
// WithReceiverTaprootTapTree option can match outputs.
func findExtraPacketsForReceivers(
	receiverAddrs []string, vhtlcs []domain.Vhtlc, hrp string,
) ([]extension.Packet, map[string][]byte, error) {
	if len(receiverAddrs) == 0 || len(vhtlcs) == 0 {
		return nil, nil, nil
	}

	wanted := make(map[string]bool, len(receiverAddrs))
	for _, a := range receiverAddrs {
		wanted[a] = true
	}

	var pkts []extension.Packet
	tapTrees := make(map[string][]byte)
	for _, v := range vhtlcs {
		if len(v.ExtraPacket) == 0 {
			continue
		}
		s, err := vhtlc.NewVHTLCScriptFromOpts(v.Opts)
		if err != nil {
			return nil, nil, fmt.Errorf("rebuild vhtlc %s: %w", v.Id, err)
		}
		addr, err := s.Address(hrp)
		if err != nil {
			return nil, nil, fmt.Errorf("compute address for vhtlc %s: %w", v.Id, err)
		}
		if !wanted[addr] {
			continue
		}
		raw := v.ExtraPacket
		pkts = append(pkts, extension.UnknownPacket{PacketType: raw[0], Data: raw[1:]})

		tapKey, _, err := s.TapTree()
		if err != nil {
			return nil, nil, fmt.Errorf("derive tapkey for vhtlc %s: %w", v.Id, err)
		}
		pkScript, err := script.P2TRScript(tapKey)
		if err != nil {
			return nil, nil, fmt.Errorf("p2tr for vhtlc %s: %w", v.Id, err)
		}
		encoded, err := txutils.TapTree(s.GetRevealedTapscripts()).Encode()
		if err != nil {
			return nil, nil, fmt.Errorf("encode taptree for vhtlc %s: %w", v.Id, err)
		}
		tapTrees[hex.EncodeToString(pkScript)] = encoded
	}
	return pkts, tapTrees, nil
}
