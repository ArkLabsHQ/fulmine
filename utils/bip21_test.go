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

// BIP21 denominates amount in decimal BTC. Service.NewAddress emits it as
// fmt.Sprintf("%.8f", sats/1e8), so "0.00001000" and "1.00000000" below are the
// literal strings that end up in our own receive QR codes — parsing those as
// integers returned 0 for every one, which made validateBip21Api report
// fulmine's own links as "invalid invoice".
func TestSatsFromBip21(t *testing.T) {
	tests := []struct {
		name   string
		amount string
		want   uint64
	}{
		{name: "padded decimal", amount: "0.00001000", want: 1000},
		{name: "padded int", amount: "1.00000000", want: 100000000},
		{name: "unpadded decimal", amount: "0.0001", want: 10000},
		{
			// 0.29 * 1e8 is 28999999.9999999963 in binary floating point, so a
			// truncating conversion would return 28999999 and lose a satoshi.
			name: "no loss of precision", amount: "0.29", want: 29000000,
		},
		{name: "unpadded int", amount: "1", want: 100000000},
		{name: "max supply", amount: "21000000.00000000", want: 2100000000000000},
		{name: "zero", amount: "0.00000000", want: 0},
		{name: "negative", amount: "-1.0", want: 0},
		{name: "non-numeric", amount: "abc", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bip21 := "bitcoin:" + validBtcAddr + "?ark=" + validArkAddr + "&amount=" + tt.amount
			require.Equal(t, tt.want, utils.SatsFromBip21(bip21))
		})
	}

	t.Run("no amount parameter", func(t *testing.T) {
		require.Zero(t, utils.SatsFromBip21("bitcoin:"+validBtcAddr+"?ark="+validArkAddr))
	})
	t.Run("valueless amount parameter", func(t *testing.T) {
		require.Zero(t, utils.SatsFromBip21("bitcoin:"+validBtcAddr+"?amount"))
	})
	t.Run("not a bip21", func(t *testing.T) {
		require.Zero(t, utils.SatsFromBip21("definitely-not-a-bip21"))
	})
}

func TestIsBip21(t *testing.T) {
	tests := []struct {
		name  string
		bip21 string
		want  bool
	}{
		{
			name:  "onchain address only",
			bip21: "bitcoin:" + validBtcAddr,
			want:  true,
		},
		{
			name:  "onchain address with ark parameter",
			bip21: "bitcoin:" + validBtcAddr + "?ark=" + validArkAddr,
			want:  true,
		},
		{
			// An ark address with no on-chain part is still a usable bip21:
			// IsBip21 only requires one of the two to be present.
			name:  "ark parameter only",
			bip21: "bitcoin:?ark=" + validArkAddr,
			want:  true,
		},
		{
			name:  "empty string",
			bip21: "",
			want:  false,
		},
		{
			name:  "wrong scheme",
			bip21: "lightning:" + validBtcAddr,
			want:  false,
		},
		{
			name:  "invalid address",
			bip21: "bitcoin:nonsense",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, utils.IsBip21(tt.bip21))
		})
	}
}
