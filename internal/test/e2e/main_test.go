package e2e_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	pb "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	//if err := refillArkd(ctx); err != nil {
	//log.Fatalf("❌ failed to refill Arkade server: %s", err)
	//}

	if err := refillFulmineBoltz(ctx); err != nil {
		log.Fatalf("❌ failed to refill Fulmine used by Boltz: %s", err)
	}

	if err := refillFulmineClient(ctx); err != nil {
		log.Fatalf("❌ failed to refill Fulmine used by Client: %s", err)
	}

	os.Exit(m.Run())
}

func refillArkd(ctx context.Context) error {
	arkdExec := "docker exec arkd arkd"
	balanceThreshold := 5.0

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

func refillFulmineBoltz(ctx context.Context) error {
	return refillFulmine(ctx, "localhost:7000")
}

func refillFulmineClient(ctx context.Context) error {
	return refillFulmine(ctx, "localhost:7002")
}

func refillFulmine(ctx context.Context, url string) error {
	balanceThreshold := 100000

	f, err := newFulmineClient(url)
	if err != nil {
		return err
	}

	balance, err := f.GetBalance(ctx, &pb.GetBalanceRequest{})
	if err != nil {
		return err
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

	time.Sleep(5 * time.Second)
	_, err = f.Settle(ctx, &pb.SettleRequest{})
	return err
}
