package nigiri

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/creack/pty"
)

const (
	nigiriBinary = "nigiri"
)

func AddInvoice(ctx context.Context, sats int) (string, string, error) {
	out, err := run(ctx, nigiriBinary, "lnd", "addinvoice", "--amt", strconv.Itoa(sats))
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

func LookupInvoice(ctx context.Context, rHash string) (bool, error) {
	output, err := run(ctx, nigiriBinary, "lnd", "lookupinvoice", rHash)
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

func PayInvoice(ctx context.Context, invoice string) error {
	// run the payment with --force so lncli does not wait for interactive confirmation
	output, err := run(ctx, nigiriBinary, "lnd", "payinvoice", "--force", invoice)
	if err != nil {
		return fmt.Errorf("payinvoice: %w (output: %s)", err, strings.TrimSpace(string(output)))
	}

	return nil
}

func MineBlocks(ctx context.Context, blocks int) error {
	if _, err := run(ctx, nigiriBinary, "rpc", "--generate", strconv.Itoa(blocks)); err != nil {
		return fmt.Errorf("confirm blocks: %w", err)
	}

	return nil

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
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(&out, ptmx)
		done <- err
	}()

	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		copyErr := <-done
		if copyErr != nil && !errors.Is(copyErr, syscall.EIO) {
			return out.Bytes(), fmt.Errorf("read command output: %w", copyErr)
		}
		return out.Bytes(), ctx.Err()
	case copyErr := <-done:
		if copyErr != nil && !errors.Is(copyErr, syscall.EIO) {
			return out.Bytes(), fmt.Errorf("read command output: %w", copyErr)
		}
		return out.Bytes(), cmd.Wait()
	}
}
