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
	testUrls(t)
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

func testUrls(t *testing.T) {
	t.Run("urls", func(t *testing.T) {
		res := utils.IsValidURL("acme")
		require.Equal(t, false, res)

		res = utils.IsValidURL("acme.com")
		require.Equal(t, false, res)

		res = utils.IsValidURL("acme.com:7070")
		require.Equal(t, true, res)

		res = utils.IsValidURL("localhost:7070")
		require.Equal(t, true, res)

		res = utils.IsValidURL("http://acme.com")
		require.Equal(t, true, res)

		res = utils.IsValidURL("https://acme.com")
		require.Equal(t, true, res)

		res = utils.IsValidURL("http://acme.com:7070")
		require.Equal(t, true, res)

		res = utils.IsValidURL("https://acme.com:7070")
		require.Equal(t, true, res)
	})
}
