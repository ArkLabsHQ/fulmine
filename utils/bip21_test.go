package utils_test

import (
	"testing"

	"github.com/ArkLabsHQ/fulmine/utils"
	"github.com/stretchr/testify/require"
)

const (
	validBtcAddr = "bc1qar0srrr7xfkvy5l643lydnw9re59gtzzwf5mdq"
	validArkAddr = "tark1qr340xg400jtxat9hdd0ungyu6s05zjtdf85uj9smyzxshf98ndah5wxuw00tcf2f46cky4a2c845xdkvpdh2r68ffkh508vaht8wlkw87ttcm"
)

// These helpers run on whatever the user types into the send form — the page
// posts every keystroke to /helpers/bip21/validate — so malformed input must
// return empty rather than panic.
func TestBip21HelpersDoNotPanicOnMalformedInput(t *testing.T) {
	inputs := []string{
		"",
		"bitcoin:",
		"bitcoin:" + validBtcAddr,
		// Valueless parameters: these used to panic on kv[1].
		"bitcoin:" + validBtcAddr + "?ark",
		"bitcoin:" + validBtcAddr + "?amount",
		"bitcoin:" + validBtcAddr + "?ark&amount",
		"bitcoin:" + validBtcAddr + "?=",
		"bitcoin:" + validBtcAddr + "?&&&",
		"bitcoin:" + validBtcAddr + "?ark=",
		"not-a-bip21",
		"?ark",
	}

	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			require.NotPanics(t, func() {
				utils.IsBip21(in)
				utils.GetOffchainAddress(in)
				utils.GetBtcAddress(in)
				utils.SatsFromBip21(in)
			})
		})
	}
}

func TestGetOffchainAddress(t *testing.T) {
	tests := []struct {
		name  string
		bip21 string
		want  string
	}{
		{
			name:  "ark parameter present",
			bip21: "bitcoin:" + validBtcAddr + "?ark=" + validArkAddr,
			want:  validArkAddr,
		},
		{
			name:  "ark alongside other parameters",
			bip21: "bitcoin:" + validBtcAddr + "?amount=1&ark=" + validArkAddr,
			want:  validArkAddr,
		},
		{
			name:  "valueless ark parameter",
			bip21: "bitcoin:" + validBtcAddr + "?ark",
			want:  "",
		},
		{
			name:  "ark parameter with an invalid address",
			bip21: "bitcoin:" + validBtcAddr + "?ark=nonsense",
			want:  "",
		},
		{
			name:  "no query string",
			bip21: "bitcoin:" + validBtcAddr,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, utils.GetOffchainAddress(tt.bip21))
		})
	}
}

func TestIsBip21(t *testing.T) {
	require.True(t, utils.IsBip21("bitcoin:"+validBtcAddr))
	require.True(t, utils.IsBip21("bitcoin:"+validBtcAddr+"?ark="+validArkAddr))
	// Valid ark address but no on-chain part is still a usable bip21.
	require.True(t, utils.IsBip21("bitcoin:?ark="+validArkAddr))

	require.False(t, utils.IsBip21(""))
	require.False(t, utils.IsBip21("lightning:"+validBtcAddr))
	require.False(t, utils.IsBip21("bitcoin:nonsense"))
}
