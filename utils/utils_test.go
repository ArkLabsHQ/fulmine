package utils_test

import (
	"testing"

	"github.com/ArkLabsHQ/fulmine/utils"
	"github.com/stretchr/testify/require"
)

var (
	arkAddress   = "tark1qz9fhwclk24f9w240hgt8x597vwjqn6ckswx96s3944dzj9f3qfg2dk2u4fadt0jj54kf8s3y42gr4fzl4f8xc5hfgl5kazuvk5cwsj5zg4aet"
	arkNote      = "arknote8rFzGqZsG9RCLripA6ez8d2hQEzFKsqCeiSnXhQj56Ysw7ZQT"
	btcAddress   = "tb1pf422yvfxrh9ne0cunv0xalp3cv0pcys0fvpttv09lsh9dvt09zzqzcmphm"
	bip21Invoice = "bitcoin:" + btcAddress + "?ark=" + arkAddress
	mnemonic     = "reward liar quote property federal print outdoor attitude satoshi favorite special layer"
)

func TestUtils(t *testing.T) {
	testAddresses(t)
	testBip21(t)
	testNotes(t)
	testSecrets(t)
	testIsValidUrls(t)
	testValidateUrls(t)
}

func testAddresses(t *testing.T) {
	t.Run("addresses", func(t *testing.T) {
		addr := utils.GetArkAddress(bip21Invoice)
		require.Equal(t, arkAddress, addr)

		addr = utils.GetBtcAddress(bip21Invoice)
		require.Equal(t, btcAddress, addr)

		res := utils.IsValidArkAddress("")
		require.Equal(t, false, res)

		res = utils.IsValidArkAddress(arkAddress)
		require.Equal(t, true, res)

		res = utils.IsValidBtcAddress("")
		require.Equal(t, false, res)

		res = utils.IsValidBtcAddress(btcAddress)
		require.Equal(t, true, res)
	})
}

func testBip21(t *testing.T) {
	t.Run("bip21", func(t *testing.T) {
		res := utils.IsBip21("")
		require.Equal(t, false, res)

		res = utils.IsBip21("bitcoin:xxx")
		require.Equal(t, false, res)

		res = utils.IsBip21(bip21Invoice)
		require.Equal(t, true, res)
	})
}

func testNotes(t *testing.T) {
	t.Run("notes", func(t *testing.T) {
		res := utils.IsValidArkNote("")
		require.Equal(t, false, res)

		res = utils.IsValidArkNote("arknote")
		require.Equal(t, false, res)

		res = utils.IsValidArkNote(arkNote)
		require.Equal(t, true, res)
	})
}

func testSecrets(t *testing.T) {
	t.Run("secrets", func(t *testing.T) {
		err := utils.IsValidMnemonic("")
		require.Error(t, err)
		require.ErrorContains(t, err, "12 words")

		err = utils.IsValidMnemonic("mnemonic")
		require.Error(t, err)
		require.ErrorContains(t, err, "12 words")

		err = utils.IsValidMnemonic(mnemonic + "xxx")
		require.Error(t, err)
		require.ErrorContains(t, err, "invalid")

		err = utils.IsValidMnemonic(mnemonic)
		require.NoError(t, err)

		// TODO: enable when password validation is enabled
		// err = utils.IsValidPassword("abc")
		// require.Error(t, err)
		// require.ErrorContains(t, err, "too short")

		// err = utils.IsValidPassword("abcdefgh")
		// require.Error(t, err)
		// require.ErrorContains(t, err, "must have a number")

		// err = utils.IsValidPassword("12345678")
		// require.Error(t, err)
		// require.ErrorContains(t, err, "must have a special character")

		err = utils.IsValidPassword("12345678!")
		require.NoError(t, err)
	})
}

func testIsValidUrls(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "empty", input: "", want: false},
		{name: "no host", input: "acme", want: false},
		{name: "hostname only", input: "acme.com", want: false},
		{name: "host and port", input: "acme.com:7070", want: true},
		{name: "localhost port", input: "localhost:7070", want: true},
		{name: "http", input: "http://acme.com", want: true},
		{name: "https", input: "https://acme.com", want: true},
		{name: "http with port", input: "http://acme.com:7070", want: true},
		{name: "https with port", input: "https://acme.com:7070", want: true},
		{name: "ftp scheme", input: "ftp://acme.com", want: true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, utils.IsValidURL(tc.input))
		})
	}
}

func testValidateUrls(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expect      string
		errContains string
	}{
		{name: "with scheme and port", input: "http://boltz-lnd:10009", expect: "boltz-lnd:10009"},
		{name: "https with port", input: "https://acme.com:7070", expect: "acme.com:7070"},
		{name: "localhost with port", input: "localhost:7000", expect: "localhost:7000"},
		{name: "hostname with port", input: "acme.com:8080", expect: "acme.com:8080"},
		{name: "http without port", input: "http://acme.com", expect: "http://acme.com"},
		{name: "https without port", input: "https://example.com", expect: "https://example.com"},
		{name: "no scheme adds http", input: "acme.com", expect: "http://acme.com"},
		{name: "trims whitespace", input: "  https://trim.me  ", expect: "https://trim.me"},
		{name: "empty", input: "", errContains: "url is empty"},
		{name: "unsupported scheme", input: "ftp://acme.com", errContains: "unsupported scheme"},
		{name: "missing host", input: "http://", errContains: "missing host"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			validated, err := utils.ValidateURL(tc.input)
			if tc.errContains != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.errContains)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expect, validated)
		})
	}

}
