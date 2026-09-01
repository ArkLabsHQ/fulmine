package grpc_interface

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ArkLabsHQ/fulmine/utils"
	log "github.com/sirupsen/logrus"
)

const (
	autoInitMaxAttempts  = 6
	autoInitInitialDelay = 2 * time.Second
	autoInitMaxDelay     = 30 * time.Second
)

// autoInit creates the wallet on first boot so a fresh instance becomes
// operational without any manual genseed/create/unlock calls. Whatever lock
// state Setup leaves behind is handled by the autoUnlock step that follows in
// Start(): UnlockNode guards on IsLocked, so it is safe either way.
func (s *service) autoInit() error {
	ctx := context.Background()

	if s.appSvc.IsInitialized() {
		log.Debug("wallet already initialized, skipping auto-init")
		return nil
	}

	// An unlocker is guaranteed by config validation.
	password, err := s.unlockerSvc.GetPassword(ctx)
	if err != nil {
		return fmt.Errorf("failed to get password from unlocker: %w", err)
	}
	if len(password) <= 0 {
		return fmt.Errorf("unlocker returned an empty password")
	}

	mnemonic, generated, err := resolveSetupMnemonic(s.cfg.AutoInitMnemonic)
	if err != nil {
		return err
	}

	// The ark server may still be booting (e.g. compose startup race): retry
	// with backoff before giving up. The mnemonic is fixed before the loop so
	// every retry re-creates the same identity.
	delay := autoInitInitialDelay
	for attempt := 1; ; attempt++ {
		err = s.appSvc.Setup(ctx, s.arkServer, password, mnemonic)
		if err == nil {
			break
		}
		if attempt >= autoInitMaxAttempts {
			// Name the ark server: an unreachable or misconfigured
			// FULMINE_ARK_SERVER is the likeliest cause, and this error is the
			// last thing an operator sees before the process exits.
			return fmt.Errorf(
				"failed to initialize wallet with ark server %s after %d attempts: %w",
				s.arkServer, autoInitMaxAttempts, err,
			)
		}
		log.WithError(err).Warnf(
			"auto-init attempt %d/%d failed, retrying in %s", attempt, autoInitMaxAttempts, delay,
		)
		time.Sleep(delay)
		delay *= 2
		if delay > autoInitMaxDelay {
			delay = autoInitMaxDelay
		}
	}

	log.Info("wallet auto-initialized")
	if generated {
		// The banner goes straight to stdout, never through the logger: it must
		// not be suppressed by the log level nor shipped by any log hook.
		fmt.Fprint(os.Stdout, formatMnemonicBanner(mnemonic))
	}
	return nil
}

// verifyConfiguredMnemonic ensures the configured restore mnemonic matches the
// identity of the wallet found in the datadir, so an operator restoring a
// backup can't run against the wrong wallet without noticing. The wallet must
// be unlocked.
func (s *service) verifyConfiguredMnemonic() error {
	mnemonic, err := s.appSvc.Dump(context.Background())
	if err != nil {
		return fmt.Errorf("cannot verify FULMINE_MNEMONIC against the wallet identity: %w", err)
	}
	if normalizeMnemonic(mnemonic) != normalizeMnemonic(s.cfg.AutoInitMnemonic) {
		return fmt.Errorf(
			"FULMINE_MNEMONIC does not match the wallet found in the datadir: " +
				"remove the mnemonic variable or point FULMINE_DATADIR to the matching wallet",
		)
	}
	return nil
}

// resolveSetupMnemonic returns the configured mnemonic, or a freshly generated
// one when none is configured.
func resolveSetupMnemonic(configured string) (mnemonic string, generated bool, err error) {
	if len(configured) > 0 {
		return configured, false, nil
	}
	mnemonic, err = utils.GetNewMnemonic()
	if err != nil {
		return "", false, fmt.Errorf("failed to generate mnemonic: %w", err)
	}
	return mnemonic, true, nil
}

func normalizeMnemonic(mnemonic string) string {
	return strings.ToLower(strings.Join(strings.Fields(mnemonic), " "))
}

func formatMnemonicBanner(mnemonic string) string {
	return fmt.Sprintf(`
==========================================================================

  FULMINE WALLET CREATED - BACK UP YOUR MNEMONIC NOW

      %s

  These 12 words are shown ONCE and never again. They are the wallet
  identity and the delegate signing key: store them offline before
  onboarding users. To restore on a new machine, run the same command
  with FULMINE_MNEMONIC (or FULMINE_MNEMONIC_FILE_PATH) set.

  Once backed up, scrub these words from wherever this output was
  captured (container logs, log aggregators, terminal scrollback).

==========================================================================
`, mnemonic)
}
