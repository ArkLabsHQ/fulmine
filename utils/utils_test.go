package utils_test

import (
	"testing"

	"github.com/ArkLabsHQ/ark-node/utils"
	"github.com/stretchr/testify/require"
)

var (
	arkAddress   = "tark1qd90wnfly2zd5749lse0mgyttytzaumy35wx8rnvug3sz30wkl805qat5mgdend24ay6pnq0hcm2wgtfkk0xdn2lt25dc90wyxlcx8hnpst8lpeh"
	arkNote      = "arknoteQrSSs5CFsLsMxsfNnhgudjqWeicxuSGSzBEntv7dyZ9CLS9zazjEyeyshmcwMRcKehCxJRouEwurnzPRbVbc3JHsfTZ3LT4mrBJDsVJj"
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

		err = utils.IsValidPassword("abc")
		require.Error(t, err)
		require.ErrorContains(t, err, "too short")

		err = utils.IsValidPassword("abcdefgh")
		require.Error(t, err)
		require.ErrorContains(t, err, "must have a number")

		err = utils.IsValidPassword("12345678")
		require.Error(t, err)
		require.ErrorContains(t, err, "must have a special character")

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
