package application

import (
	"testing"

	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// The delegate's fee address, and an unrelated one a confused (or stale) client
// might pay instead.
var (
	delegateScript = []byte{0x51, 0x20, 0xaa, 0xbb, 0xcc}
	strangerScript = []byte{0x51, 0x20, 0xdd, 0xee, 0xff}
	delegateAddr   = "tark1qdelegateaddress"
)

func TestFindDelegateFeeOutput(t *testing.T) {
	tests := []struct {
		name      string
		outputs   []*wire.TxOut
		wantValue int64
		wantFound bool
	}{
		{
			name:      "no outputs at all",
			outputs:   nil,
			wantValue: 0,
			wantFound: false,
		},
		{
			// The shape produced by a client holding a stale delegate address:
			// the proof is well formed, it just pays someone else.
			name: "outputs present but none pays the delegate",
			outputs: []*wire.TxOut{
				{Value: 5000, PkScript: strangerScript},
			},
			wantValue: 0,
			wantFound: false,
		},
		{
			name: "output paying the delegate",
			outputs: []*wire.TxOut{
				{Value: 21000, PkScript: strangerScript},
				{Value: 1000, PkScript: delegateScript},
			},
			wantValue: 1000,
			wantFound: true,
		},
		{
			// A zero-value output to us is still an output to us. It must be
			// reported as found so the caller rejects it as an underpayment
			// rather than as a wrong address.
			name: "zero-value output paying the delegate is still found",
			outputs: []*wire.TxOut{
				{Value: 0, PkScript: delegateScript},
			},
			wantValue: 0,
			wantFound: true,
		},
		{
			name: "first matching output wins",
			outputs: []*wire.TxOut{
				{Value: 1000, PkScript: delegateScript},
				{Value: 9999, PkScript: delegateScript},
			},
			wantValue: 1000,
			wantFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, found := findDelegateFeeOutput(tt.outputs, delegateScript)
			require.Equal(t, tt.wantFound, found)
			require.Equal(t, tt.wantValue, value)
		})
	}
}

func TestValidateDelegateFee(t *testing.T) {
	tests := []struct {
		name        string
		requiredFee uint64
		paidAmount  int64
		feePaidToUs bool
		wantErr     string // empty means the delegation must be accepted
	}{
		// requiredFee == 0 disables the check completely: GetInfo advertised a
		// zero fee, so a client owes us nothing and need not pay us at all.
		{
			name:        "no fee required and nothing paid",
			requiredFee: 0,
			feePaidToUs: false,
		},
		{
			name:        "no fee required and the client paid a stranger",
			requiredFee: 0,
			paidAmount:  5000,
			feePaidToUs: false,
		},
		{
			name:        "no fee required but the client paid anyway",
			requiredFee: 0,
			paidAmount:  1000,
			feePaidToUs: true,
		},

		{
			name:        "exact fee paid",
			requiredFee: 1000,
			paidAmount:  1000,
			feePaidToUs: true,
		},
		{
			name:        "more than the required fee paid",
			requiredFee: 1000,
			paidAmount:  5000,
			feePaidToUs: true,
		},
		{
			name:        "underpaid",
			requiredFee: 1000,
			paidAmount:  999,
			feePaidToUs: true,
			wantErr:     "delegate fee is less than the required fee",
		},
		{
			// The regression this guard exists for. Before it, a client paying a
			// delegate address we had rotated away from produced an
			// "underpayment" error blaming the client for our own address change.
			name:        "fee required but no output pays the delegate",
			requiredFee: 1000,
			feePaidToUs: false,
			wantErr:     "has no output paying the delegate address",
		},
		{
			// A negative int64 converted to uint64 unchecked wraps to a huge
			// number and would sail past the comparison.
			name:        "negative output amount is rejected, not wrapped",
			requiredFee: 1000,
			paidAmount:  -1,
			feePaidToUs: true,
			wantErr:     "invalid delegate fee output amount",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDelegateFee(tt.requiredFee, tt.paidAmount, tt.feePaidToUs, delegateAddr)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// The wrong-address error must name the address we expect, so an operator can
// compare it against what the client used instead of guessing.
func TestValidateDelegateFeeErrorNamesTheAddress(t *testing.T) {
	err := validateDelegateFee(1000, 0, false, delegateAddr)
	require.ErrorContains(t, err, delegateAddr)
}
