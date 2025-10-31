package lightning

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/creack/pty"
)

const (
	boltzLndContainer = "boltz-lnd"
	lncliBinary       = "lncli"
	nigiriBinary      = "nigiri"
	lndPeerHost       = "lnd:9735"
	requiredChannel   = 100_000 // sats
	minWalletBalance  = 200_000 // sats, gives buffer for fees
	faucetRounds      = 3
	faucetAmountBTC   = "1"
)

type getInfoResponse struct {
	IdentityPubKey string `json:"identity_pubkey"`
}

type listPeersResponse struct {
	Peers []struct {
		PubKey string `json:"pub_key"`
	} `json:"peers"`
}

type listChannelsResponse struct {
	Channels []struct {
		RemotePubKey string `json:"remote_pubkey"`
		Active       bool   `json:"active"`
		Capacity     string `json:"capacity"`
	} `json:"channels"`
}

type walletBalanceResponse struct {
	ConfirmedBalance json.Number `json:"confirmed_balance"`
}

type newAddressResponse struct {
	Address string `json:"address"`
}

type openChannelResponse struct {
	FundingTxid string `json:"funding_txid"`
}

// EnsureConnectivity connects boltz-lnd to the nigiri LND node, funds the
// wallet if necessary, opens a channel, and confirms it on-chain. It is
// idempotent and safe to call multiple times.
func EnsureConnectivity(ctx context.Context) error {
	pubKey, err := nigiriIdentity(ctx)
	if err != nil {
		return fmt.Errorf("fetch nigiri lnd identity: %w", err)
	}

	if err := ensurePeer(ctx, pubKey); err != nil {
		return fmt.Errorf("ensure boltz peer: %w", err)
	}

	if err := ensureChannel(ctx, pubKey); err != nil {
		return fmt.Errorf("ensure boltz channel: %w", err)
	}

	return nil
}

func nigiriIdentity(ctx context.Context) (string, error) {
	out, err := run(ctx, nigiriBinary, "lnd", "getinfo")
	if err != nil {
		return "", err
	}
	var info getInfoResponse
	if err := json.Unmarshal(out, &info); err != nil {
		return "", fmt.Errorf("parse getinfo: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	if info.IdentityPubKey == "" {
		return "", fmt.Errorf("empty identity pub key")
	}
	return info.IdentityPubKey, nil
}

func ensurePeer(ctx context.Context, pubKey string) error {
	peers, err := listPeers(ctx)
	if err != nil {
		return err
	}
	for _, peer := range peers.Peers {
		if peer.PubKey == pubKey {
			return nil
		}
	}

	connectAddr := fmt.Sprintf("%s@%s", pubKey, lndPeerHost)
	_, err = run(ctx, "docker", "exec", boltzLndContainer, lncliBinary, "--network=regtest", "connect", connectAddr)
	if err != nil {
		return fmt.Errorf("connect peers: %w", err)
	}
	// give the nodes a moment to register
	time.Sleep(2 * time.Second)
	return nil
}

func ensureChannel(ctx context.Context, pubKey string) error {
	channels, err := listChannels(ctx)
	if err != nil {
		return err
	}
	for _, channel := range channels.Channels {
		if channel.RemotePubKey == pubKey {
			return nil
		}
	}

	if err := ensureFunds(ctx); err != nil {
		return fmt.Errorf("fund boltz lnd: %w", err)
	}

	_, err = run(
		ctx, "docker", "exec", boltzLndContainer, lncliBinary, "--network=regtest",
		"openchannel", "--node_key", pubKey, "--local_amt", strconv.Itoa(requiredChannel),
	)
	if err != nil {
		if strings.Contains(err.Error(), "channel already exists") {
			return nil
		}
		return fmt.Errorf("open channel: %w", err)
	}

	// mine blocks to confirm the channel
	if _, err := run(ctx, nigiriBinary, "rpc", "--generate", "10"); err != nil {
		return fmt.Errorf("confirm blocks: %w", err)
	}

	// give the channel a moment to activate
	time.Sleep(5 * time.Second)

	// createa an invoice
	invoice, _, err := NigiriAddInvoice(ctx, 30000)
	if err != nil {
		return fmt.Errorf("create invoice: %w", err)
	}

	// pay the invoice to activate the channel
	_, err = run(
		ctx, "docker", "exec", boltzLndContainer, lncliBinary, "--network=regtest",
		"payinvoice", "--force", invoice,
	)
	if err != nil {
		return fmt.Errorf("activate channel: %w", err)
	}

	return nil
}

func ensureFunds(ctx context.Context) error {
	balance, err := boltzWalletBalance(ctx)
	if err != nil {
		return err
	}

	if balance >= minWalletBalance {
		return nil
	}

	addrResp, err := newAddress(ctx)
	if err != nil {
		return err
	}

	for i := 0; i < faucetRounds; i++ {
		if _, err := run(ctx, nigiriBinary, "faucet", addrResp.Address, faucetAmountBTC); err != nil {
			return fmt.Errorf("fund boltz-lnd address: %w", err)
		}
	}

	if _, err := run(ctx, nigiriBinary, "rpc", "--generate", "1"); err != nil {
		return fmt.Errorf("mine confirmation block: %w", err)
	}

	// Wait for wallet to register funds
	time.Sleep(5 * time.Second)
	return nil
}

func listPeers(ctx context.Context) (listPeersResponse, error) {
	var peers listPeersResponse
	out, err := run(ctx, "docker", "exec", "-t", boltzLndContainer, lncliBinary, "--network=regtest", "listpeers")
	if err != nil {
		return peers, err
	}
	if err := json.Unmarshal(out, &peers); err != nil {
		return peers, fmt.Errorf("parse listpeers: %w", err)
	}
	return peers, nil
}

func listChannels(ctx context.Context) (listChannelsResponse, error) {
	var channels listChannelsResponse
	out, err := run(ctx, "docker", "exec", boltzLndContainer, lncliBinary, "--network=regtest", "listchannels")
	if err != nil {
		return channels, err
	}
	if err := json.Unmarshal(out, &channels); err != nil {
		return channels, fmt.Errorf("parse listchannels: %w", err)
	}
	return channels, nil
}

func boltzWalletBalance(ctx context.Context) (int64, error) {
	out, err := run(ctx, "docker", "exec", boltzLndContainer, lncliBinary, "--network=regtest", "walletbalance")
	if err != nil {
		return 0, err
	}
	var balance walletBalanceResponse
	if err := json.Unmarshal(out, &balance); err != nil {
		return 0, fmt.Errorf("parse walletbalance: %w", err)
	}
	value, err := balance.ConfirmedBalance.Int64()
	if err != nil {
		parsed, perr := strconv.ParseInt(balance.ConfirmedBalance.String(), 10, 64)
		if perr != nil {
			return 0, fmt.Errorf("convert walletbalance: %w", err)
		}
		return parsed, nil
	}
	return value, nil
}

func newAddress(ctx context.Context) (newAddressResponse, error) {
	var resp newAddressResponse
	out, err := run(ctx, "docker", "exec", boltzLndContainer, lncliBinary, "--network=regtest", "newaddress", "p2wkh")
	if err != nil {
		return resp, err
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return resp, fmt.Errorf("parse newaddress: %w", err)
	}
	if resp.Address == "" {
		return resp, fmt.Errorf("received empty address")
	}
	return resp, nil
}

func run(ctx context.Context, command string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = ptmx.Close()
		_ = cmd.Wait()
	}()

	var out bytes.Buffer
	done := make(chan struct{})
	go func() {
		io.Copy(&out, ptmx)
		close(done)
	}()

	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-done
		return out.Bytes(), ctx.Err()
	case <-done:
		return out.Bytes(), cmd.Wait()
	}
}

func NigiriAddInvoice(ctx context.Context, sats int) (string, string, error) {
	out, err := run(ctx, "nigiri", "lnd", "addinvoice", "--amt", strconv.Itoa(sats))
	if err != nil {
		return "", "", err
	}

	var resp struct {
		PaymentRequest string `json:"payment_request"`
		RHash          string `json:"r_hash"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", "", fmt.Errorf("parse addinvoice response: %w", err)
	}
	if resp.PaymentRequest == "" || resp.RHash == "" {
		return "", "", fmt.Errorf("incomplete invoice response: %s", strings.TrimSpace(string(out)))
	}
	return resp.PaymentRequest, resp.RHash, nil
}

func NigiriLookupInvoice(ctx context.Context, rHash string) (bool, error) {
	output, err := run(ctx, "nigiri", "lnd", "lookupinvoice", rHash)
	if err != nil {
		return false, fmt.Errorf("lookupinvoice: %w (output: %s)", err, strings.TrimSpace(string(output)))
	}
	var resp struct {
		Settled bool `json:"settled"`
	}
	if err := json.Unmarshal(output, &resp); err != nil {
		return false, fmt.Errorf("parse lookupinvoice response: %w", err)
	}
	return resp.Settled, nil
}

func NigiriPayInvoice(ctx context.Context, invoice string) error {
	// run the payment with --force so lncli does not wait for interactive confirmation
	output, err := run(ctx, "nigiri", "lnd", "payinvoice", "--force", invoice)
	if err != nil {
		return fmt.Errorf("payinvoice: %w (output: %s)", err, strings.TrimSpace(string(output)))
	}

	return nil
}
