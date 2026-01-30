package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/stretchr/testify/require"
)

func TestChainSwapArkToBTC(t *testing.T) {
	ctx := context.Background()

	url := "localhost:7000"
	client, err := newFulmineClient(url)
	require.NoError(t, err)

	err = refillFulmine(ctx, url)
	require.NoError(t, err)

	out, err := runCommand(ctx, "nigiri rpc getnewaddress")
	require.NoError(t, err)
	btcAddress := strings.TrimSpace(out)

	getBtcAddressBalance := func(addr string) (float64, error) {
		getBtcAddressBalanceCmd := fmt.Sprintf("nigiri rpc scantxoutset start '[\"addr(%s)\"]'", addr)
		o, er := runCommand(ctx, getBtcAddressBalanceCmd)
		if er != nil {
			return 0, er
		}

		var raw map[string]interface{}
		if er := json.Unmarshal([]byte(stripANSI(o)), &raw); er != nil {
			return 0, er
		}
		return raw["total_amount"].(float64), nil
	}

	addrBalance, err := getBtcAddressBalance(btcAddress)
	require.NoError(t, err)
	require.Equal(t, addrBalance, float64(0))

	// Step 1: Create Ark→BTC chain swap
	t.Log("Creating Ark→BTC chain swap...")
	createResp, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
		Direction:  pb.SwapDirection_SWAP_DIRECTION_ARK_TO_BTC,
		Amount:     3000,
		BtcAddress: btcAddress,
	})
	require.NoError(t, err, "CreateChainSwap should succeed")

	swapID := createResp.GetId()
	t.Logf("Created chain swap: %s", swapID)

	out, err = runCommand(ctx, "nigiri rpc getnewaddress")
	require.NoError(t, err)
	tmpAddr := strings.TrimSpace(out)
	genBlocksCmd := fmt.Sprintf("nigiri rpc generatetoaddress 20 %s", tmpAddr)
	_, err = runCommand(ctx, genBlocksCmd)
	require.NoError(t, err)

	time.Sleep(5 * time.Second)

	addrBalance, err = getBtcAddressBalance(btcAddress)
	require.NoError(t, err)
	require.Greater(t, addrBalance, float64(0))

	swaps, err := client.ListChainSwaps(ctx, &pb.ListChainSwapsRequest{
		SwapIds: []string{swapID},
	})
	require.NoError(t, err)
	require.Equal(t, "claimed", swaps.GetSwaps()[0].GetStatus())
}

func TestChainSwapBTCtoARK(t *testing.T) {
	ctx := context.Background()

	url := "localhost:7000"
	client, err := newFulmineClient(url)
	require.NoError(t, err)

	err = refillFulmine(ctx, url)
	require.NoError(t, err)

	startBalance, err := client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	t.Logf("Balance: %v", startBalance.GetAmount())

	t.Log("Creating BTC→ARK chain swap...")
	createResp, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
		Direction: pb.SwapDirection_SWAP_DIRECTION_BTC_TO_ARK,
		Amount:    3000,
	})
	require.NoError(t, err, "CreateChainSwap should succeed")

	swapID := createResp.GetId()
	t.Logf("Created chain swap: %s", swapID)

	err = faucet(ctx, createResp.LockupAddress, 0.00003000)
	require.NoError(t, err)

	time.Sleep(5 * time.Second)

	swaps, err := client.ListChainSwaps(ctx, &pb.ListChainSwapsRequest{
		SwapIds: []string{swapID},
	})
	require.NoError(t, err)
	require.Equal(t, "claimed", swaps.GetSwaps()[0].GetStatus())

	endBalance, err := client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	t.Logf("Balance: %d", endBalance.GetAmount())

	diff := int64(endBalance.GetAmount()) - int64(startBalance.GetAmount())
	t.Logf("Balance after swap: %d", diff)
}

func TestChainSwapBTCtoARKWithQuote(t *testing.T) {
	ctx := context.Background()

	url := "localhost:7000"
	client, err := newFulmineClient(url)
	require.NoError(t, err)

	err = refillFulmine(ctx, url)
	require.NoError(t, err)

	startBalance, err := client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	t.Logf("Balance: %v", startBalance.GetAmount())

	t.Log("Creating BTC→ARK chain swap...")
	createResp, err := client.CreateChainSwap(ctx, &pb.CreateChainSwapRequest{
		Direction: pb.SwapDirection_SWAP_DIRECTION_BTC_TO_ARK,
		Amount:    3000,
	})
	require.NoError(t, err, "CreateChainSwap should succeed")

	swapID := createResp.GetId()
	t.Logf("Created chain swap: %s", swapID)

	// faucet bigger amount so that we can test quote
	err = faucet(ctx, createResp.LockupAddress, 0.00015500)
	require.NoError(t, err)

	time.Sleep(5 * time.Second)

	swaps, err := client.ListChainSwaps(ctx, &pb.ListChainSwapsRequest{
		SwapIds: []string{swapID},
	})
	require.NoError(t, err)
	require.Equal(t, "claimed", swaps.GetSwaps()[0].GetStatus())

	endBalance, err := client.GetBalance(ctx, &pb.GetBalanceRequest{})
	require.NoError(t, err)
	t.Logf("Balance: %d", endBalance.GetAmount())

	diff := int64(endBalance.GetAmount()) - int64(startBalance.GetAmount())
	t.Logf("Balance after swap: %d", diff)
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}
