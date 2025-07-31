package swap_test

import (
	"encoding/hex"
	"testing"

	"github.com/ArkLabsHQ/fulmine/pkg/swap"
	"github.com/stretchr/testify/require"
)

var (
	offer           = "lno1qgsqvgnwgcg35z6ee2h3yczraddm72xrfua9uve2rlrm9deu7xyfzrcgqdxyksq2qdn82msjp94x76rwgp4kjer0zcss8mpn9563dpekmu6gurjr820tw98szknwpp0r57354a998cxvk0d5"
	invalid_offer   = "lno1qazwsxedcrfvtgbyhnujmikolp1234567890asdfghjklzxcvbnmqwertyuiopasdfghjklzxcvbnmqwertyuiopq"
	invoice         = "lni1qqgp3y505jcajpjek5clxrj6f3rasq3qqc3xu3s3rg94nj40zfsy866mhu5vxne6tcej5878k2mneuvgjy8ssq6vfdqq5qmxw4hpyzt2da5xusrtd9jx793pq0krxtf4z6rndhe53c8yxw57ku20q9dxuzz78farft6220svev7mg5pqqc3xu3s3rg94nj40zfsy866mhu5vxne6tcej5878k2mneuvgjy84yq6vfdq9gqzcyypc6725sflgs96cq5wazj0ny2y6ezps4ydxuu4e6zutqfzkujtg5k2eqdn82m4qnqp7cvedx5tgwdklxj8quse6n6m3fuq45msgtca85d90fff7pn9nmdqrn72vewu6uc5rnpm9fsmsq0eecxqy4n5v7rw97v563sz2vn2vq2lqzqlzt9jmk4klhlwr740pfx3yge0xldqeuu0htt8qjnqrt0049jd6yyqrym4zc5u7stcxlrlg26t9jraupd2apknpdkc0r9quz8803kdtnaw6yy4ymjnw6hnyg0cekuslcj3j7ctmdgsuqqqqqqqqqqqqqqq2qqqqqqqqqqqqq8fykt06c5sqqqqqpfqydzruzc4gyz8plcs02xsyyycgnpasya9cmuhm2ncs74rg0ce84uv3c76u3q4292srf395pvppq0krxtf4z6rndhe53c8yxw57ku20q9dxuzz78farft6220svev7mfuzq480w0sphr4kydjausdeysrwy95ll07cszzr06zq68pyg8ldprzhg4u5j82tl3gfm3afecxeu600v48dj9s64hxrvnke6gs44hzzrerg"
	invalid_invoice = "lni1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"
)

func TestBolt12(t *testing.T) {
	testBolt12Offer(t)
	testBolt12Invoice(t)
}

func testBolt12Offer(t *testing.T) {
	_, err := swap.DecodeBolt12Offer(invalid_offer)
	require.Error(t, err)

	decoded_offer, err := swap.DecodeBolt12Offer(offer)
	require.NoError(t, err)
	require.Equal(t, decoded_offer.AmountInSats, uint64(5000))
	require.Equal(t, decoded_offer.DescriptionStr, "fun")
	require.Equal(t, decoded_offer.ID, "4435a4b44691f3e2164dae39814ba0ed13b5557ed1e11bc72f7f2e80e336d4c3")
}

func testBolt12Invoice(t *testing.T) {
	_, err := swap.DecodeBolt12Invoice(invalid_invoice)
	require.Error(t, err)

	decoded_invoice, err := swap.DecodeBolt12Invoice(invoice)
	require.NoError(t, err)
	require.Equal(t, decoded_invoice.AmountInSats, uint64(5000))
	require.Equal(t, hex.EncodeToString(decoded_invoice.PaymentHash160), "c4f9f68a9a92d115970386c1e4bd7308941278fc")
}
