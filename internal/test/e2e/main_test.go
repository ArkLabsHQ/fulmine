package e2e_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	log "github.com/sirupsen/logrus"
)

const (
	// The swap client is a dedicated "user" Fulmine (regtest-user.compose.yml,
	// host gRPC 7020) that runs THIS repo's image but is separate from
	// boltz-fulmine, which is Boltz's own Ark wallet. fulmine-delegator's gRPC is
	// on host 7010 and stands in for the old in-repo "mock" Fulmine counterparty.
	delegatorFulmineURL = "localhost:7010"
)

// fulmineTarget is one user-Fulmine instance the client tests run against.
type fulmineTarget struct {
	name string
	url  string
}

// The same suite runs against both identity types. "hd" (host gRPC 7020) is the
// default for any wallet created fresh; "singlekey" (host gRPC 7030) boots from a
// datadir seeded by internal/test/tools/seed-singlekey and covers the legacy
// identity path that wallets upgraded from v0.3 still use.
var clientTargets = []fulmineTarget{
	{name: "hd", url: "localhost:7020"},
	{name: "singlekey", url: "localhost:7030"},
}

func TestMain(m *testing.M) {
	ctx := context.Background()

	if err := refillArkd(ctx); err != nil {
		log.Fatalf("❌ failed to refill Arkade server: %s", err)
	}

	for _, target := range clientTargets {
		if err := refillFulmine(ctx, target.url); err != nil {
			log.Fatalf("❌ failed to refill Fulmine used by Client (%s): %s", target.name, err)
		}
	}

	if err := refillFulmine(ctx, delegatorFulmineURL); err != nil {
		log.Fatalf("❌ failed to refill Fulmine delegator: %s", err)
	}

	os.Exit(m.Run())
}

func refillArkd(ctx context.Context) error {
	arkdExec := "docker exec arkd arkd"
	balanceThreshold := 10.0

	command := fmt.Sprintf("%s wallet balance", arkdExec)
	out, err := runCommand(ctx, command)
	if err != nil {
		return err
	}

	re := regexp.MustCompile(`available:\s*([0-9]+\.[0-9]+)`)
	balance, err := strconv.ParseFloat(re.FindStringSubmatch(out)[1], 64)
	if err != nil {
		return err
	}

	if delta := balanceThreshold - balance; delta >= 1 {
		command := fmt.Sprintf("%s wallet address", arkdExec)
		address, err := runCommand(ctx, command)
		if err != nil {
			return err
		}

		for range int(delta) {
			if err := faucet(ctx, strings.TrimSpace(address), 1); err != nil {
				return err
			}
		}
	}

	time.Sleep(5 * time.Second)
	return nil
}

func refillFulmine(ctx context.Context, url string) error {
	balanceThreshold := 100000

	f, err := newFulmineClient(url)
	if err != nil {
		return err
	}

	// The user Fulmine is created + funded by the harness immediately before the
	// suite; on a slow/contended CI start it can still be initialising, so wait
	// for it to start answering rather than hard-failing the whole run on the
	// first call ("service not initialized").
	var balance *pb.GetBalanceResponse
	deadline := time.Now().Add(90 * time.Second)
	for {
		balance, err = f.GetBalance(ctx, &pb.GetBalanceRequest{})
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("fulmine %s not ready: %w", url, err)
		}
		time.Sleep(2 * time.Second)
	}
	if int(balance.GetAmount()) >= balanceThreshold {
		return nil
	}

	if delta := balanceThreshold - int(balance.GetAmount()); delta > 0 {
		address, err := f.GetOnboardAddress(ctx, &pb.GetOnboardAddressRequest{})
		if err != nil {
			return err
		}
		amountInBtc := float64(delta) / 100000000
		if err := faucet(ctx, address.GetAddress(), amountInBtc); err != nil {
			return err
		}
	}

	return waitForSettle(ctx, func(ctx context.Context) error {
		_, err := f.Settle(ctx, &pb.SettleRequest{})
		if err != nil {
			return err
		}
		return nil
	})
}
