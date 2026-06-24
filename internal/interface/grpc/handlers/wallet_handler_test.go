package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeUnlocker is a minimal ports.Unlocker for exercising the auto-unlock
// password resolution without a real env/file unlocker.
type fakeUnlocker struct {
	password string
	err      error
}

func (f fakeUnlocker) GetPassword(_ context.Context) (string, error) {
	return f.password, f.err
}

// TestWalletHandlerAutoUnlockPassword pins the rule that an unlocker password is
// only used when it is actually present. In particular an empty password — which
// a file-backed unlocker returns for an empty/whitespace-only file — must be
// treated as unavailable so request-password validation still applies, rather
// than creating the wallet with an empty password.
func TestWalletHandlerAutoUnlockPassword(t *testing.T) {
	t.Run("no unlocker configured", func(t *testing.T) {
		h := &walletHandler{}

		pwd, ok := h.autoUnlockPassword(context.Background())

		require.False(t, ok)
		require.Empty(t, pwd)
	})

	t.Run("valid password is used", func(t *testing.T) {
		h := &walletHandler{unlocker: fakeUnlocker{password: "s3cret-pw"}}

		pwd, ok := h.autoUnlockPassword(context.Background())

		require.True(t, ok)
		require.Equal(t, "s3cret-pw", pwd)
	})

	t.Run("empty password is treated as unavailable", func(t *testing.T) {
		h := &walletHandler{unlocker: fakeUnlocker{password: ""}}

		pwd, ok := h.autoUnlockPassword(context.Background())

		require.False(t, ok)
		require.Empty(t, pwd)
	})

	t.Run("unlocker error is treated as unavailable", func(t *testing.T) {
		h := &walletHandler{unlocker: fakeUnlocker{err: errors.New("read failure")}}

		pwd, ok := h.autoUnlockPassword(context.Background())

		require.False(t, ok)
		require.Empty(t, pwd)
	})
}
