package e2e_test

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// regtestScript is the path to the arkade-regtest CLI relative to this package's
// working directory (the e2e package dir is <repo>/internal/test/e2e).
const regtestScript = "../../../regtest/regtest.mjs"

// regtestCmd runs `node <regtest.mjs> <args...>` and returns trimmed stdout/stderr.
func regtestCmd(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "node", append([]string{regtestScript}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("regtest %v: %w: %s", args, err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// faucet sends regtest coins to address and mines a block to confirm them.
func faucet(ctx context.Context, address string, amount float64) error {
	_, err := regtestCmd(ctx, "faucet", address, strconv.FormatFloat(amount, 'f', -1, 64), "--confirm")
	return err
}
