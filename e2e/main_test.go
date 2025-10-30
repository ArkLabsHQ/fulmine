package e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	arksetup "github.com/ArkLabsHQ/fulmine/e2e/setup/arkd"
	fulminesetup "github.com/ArkLabsHQ/fulmine/e2e/setup/fulmine"
	lightningsetup "github.com/ArkLabsHQ/fulmine/e2e/setup/lightning"
)

const (
	defaultComposeFile = "test.docker-compose.yml"
	boltzComposeFile   = "boltz.docker-compose.yml"
	defaultTimeout     = 5 * time.Minute
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer func() {
		// Clean up any existing e2e stack before starting new one
		_ = composeDown(ctx)
		cancel()
	}()
	// Clean up any existing e2e stack before starting new one
	err := composeDown(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to clean up e2e stack: %v\n", err)
		os.Exit(1)
	}

	if err := composeUp(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start e2e stack: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel = context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	if err := provisionServices(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to provision services: %v\n", err)
		os.Exit(1)
	}

	exitCode := m.Run()

	ctx, cancel = context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	if err := composeDown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to stop e2e stack: %v\n", err)
		// keep the original exit code so we do not mask test failures
	}

	os.Exit(exitCode)
}

func composeUp(ctx context.Context) error {
	if err := runComposeCommand(ctx, defaultComposeFile, "up", "-d", "--wait"); err != nil {
		return err
	}

	go func() {
		boltzFulmine := fulminesetup.NewTestFulmine("http://localhost:7003/api/v1")

		for {
			err := boltzFulmine.EnsureReady(context.Background())
			if err == nil {
				break
			}
			fmt.Fprintf(os.Stderr, "waiting for boltz fulmine to be ready: %v\n", err)
			time.Sleep(5 * time.Second)

			// Wait or cancel
			select {
			case <-time.After(5 * time.Second):
				// retry
			case <-ctx.Done():
				fmt.Fprintf(os.Stderr, "context cancelled while waiting for boltz fulmine to be ready: %v\n", ctx.Err())
				return
			}
		}
	}()

	if err := runComposeCommand(ctx, boltzComposeFile, "up", "-d", "--wait"); err != nil {
		return err
	}

	return nil
}

func composeDown(ctx context.Context) error {
	var combined error
	if err := runComposeCommand(ctx, boltzComposeFile, "down", "-v"); err != nil {
		combined = errors.Join(combined, err)
	}
	if err := runComposeCommand(ctx, defaultComposeFile, "down"); err != nil {
		combined = errors.Join(combined, err)
	}
	return combined
}

func runCommand(ctx context.Context, command string, args ...string) error {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = projectRoot()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %v: %w", command, args, err)
	}

	return nil
}

func projectRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), ".."))
}

func provisionServices(ctx context.Context) error {
	if err := arksetup.EnsureReady(ctx); err != nil {
		return fmt.Errorf("arkd: %w", err)
	}

	// ensure client fulmine is ready
	clientFulmine := fulminesetup.NewTestFulmine("http://localhost:7001/api/v1")
	if err := clientFulmine.EnsureReady(ctx); err != nil {
		return fmt.Errorf("fulmine: %w", err)
	}
	if err := lightningsetup.EnsureConnectivity(ctx); err != nil {
		return fmt.Errorf("lightning: %w", err)
	}
	return nil
}

func runComposeCommand(ctx context.Context, composeFile string, args ...string) error {
	cmdArgs := append([]string{"compose", "-f", composeFile}, args...)
	err := runCommand(ctx, "docker", cmdArgs...)
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "unknown flag: --wait") {
		withoutWait := make([]string, 0, len(args)-1)
		for _, arg := range args {
			if arg != "--wait" {
				withoutWait = append(withoutWait, arg)
			}
		}
		cmdArgs = append([]string{"compose", "-f", composeFile}, withoutWait...)
		return runCommand(ctx, "docker", cmdArgs...)
	}
	return err
}
