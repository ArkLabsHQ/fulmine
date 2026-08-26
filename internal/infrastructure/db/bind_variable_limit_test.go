package db_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/db"
	"github.com/ArkLabsHQ/fulmine/pkg/boltz"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// sqliteMaxVariableNumber mirrors SQLITE_MAX_VARIABLE_NUMBER as compiled into
// modernc.org/sqlite. A single prepared statement cannot carry more bind
// variables than this, so any repository method that expands a caller-supplied
// slice into one variable per element must chunk below it.
const sqliteMaxVariableNumber = 32766

func TestBindVariableLimit(t *testing.T) {
	dbDir := t.TempDir()
	tests := []struct {
		name   string
		config db.ServiceConfig
	}{
		{
			name: "badger",
			config: db.ServiceConfig{
				DbType:   "badger",
				DbConfig: []any{"", nil},
			},
		},
		{
			name: "sqlite",
			config: db.ServiceConfig{
				DbType:   "sqlite",
				DbConfig: []any{dbDir},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := db.NewService(tt.config)
			require.NoError(t, err)
			defer svc.Close()

			testVHTLCGetByIdsBoundary(t, svc.VHTLC())
			testDelegateInputsBoundary(t, svc.Delegate())

			// Chain swaps are sqlite-only; the badger backend leaves the repo nil.
			if tt.name == "sqlite" {
				testChainSwapGetByIDsBoundary(t, svc.ChainSwaps(), filepath.Join(dbDir, "fulmine.db"))
			}
		})
	}
}

func testChainSwapGetByIDsBoundary(
	t *testing.T, repo domain.ChainSwapRepository, dbPath string,
) {
	t.Run("get chain swaps by ids over the bind variable limit", func(t *testing.T) {
		// ListChainSwapsByIDs orders by created_at DESC. Seed the rows so their
		// insertion order differs from their created_at order, otherwise a
		// concatenation of per-chunk results would pass by accident.
		seeds := []struct {
			id        string
			createdAt int64
		}{
			{"chainswap-boundary-oldest", 1_000},
			{"chainswap-boundary-newest", 3_000},
			{"chainswap-boundary-middle", 2_000},
		}
		for _, s := range seeds {
			require.NoError(t, repo.Add(ctx, domain.ChainSwap{
				Id:     s.id,
				From:   boltz.CurrencyBtc,
				To:     boltz.CurrencyArk,
				Amount: 1000,
				Status: domain.ChainSwapPending,
			}))
			// created_at defaults to a second-resolution clock, so rows inserted
			// in one test would otherwise be indistinguishable. Set it directly.
			setChainSwapCreatedAt(t, dbPath, s.id, s.createdAt)
		}

		wantIDs := []string{
			"chainswap-boundary-oldest", "chainswap-boundary-newest", "chainswap-boundary-middle",
		}
		wantOrder := []string{
			"chainswap-boundary-newest", "chainswap-boundary-middle", "chainswap-boundary-oldest",
		}

		for _, size := range boundarySizes() {
			t.Run(fmt.Sprintf("size=%d", size), func(t *testing.T) {
				ids := padIDs(wantIDs, size, "chainswap-absent")

				got, err := repo.GetByIDs(ctx, ids)
				require.NoError(t, err)

				gotIDs := make([]string, 0, len(got))
				for _, s := range got {
					gotIDs = append(gotIDs, s.Id)
				}

				require.ElementsMatch(t, expectedSubset(wantIDs, ids), gotIDs)

				if len(gotIDs) == len(wantOrder) {
					require.Equal(t, wantOrder, gotIDs,
						"created_at DESC must hold across the merged chunks")
				}
			})
		}
	})
}

// setChainSwapCreatedAt overwrites the DB-assigned created_at so ordering is
// deterministic. The repository deliberately has no setter for it.
func setChainSwapCreatedAt(t *testing.T, dbPath, id string, createdAt int64) {
	t.Helper()

	conn, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.ExecContext(
		ctx, "UPDATE chain_swap SET created_at = ? WHERE id = ?", createdAt, id,
	)
	require.NoError(t, err)
}

// boundarySizes exercises the batch sizes either side of the old failure point.
// The largest entry is deliberately several multiples of the limit so a
// single-chunk regression cannot slip through.
func boundarySizes() []int {
	return []int{
		1,
		sqliteMaxVariableNumber - 1,
		sqliteMaxVariableNumber,
		sqliteMaxVariableNumber + 1,
		sqliteMaxVariableNumber * 3,
	}
}

func testVHTLCGetByIdsBoundary(t *testing.T, repo domain.VHTLCRepository) {
	t.Run("get vHTLCs by ids over the bind variable limit", func(t *testing.T) {
		// Three real rows, so we can assert the chunked read returns exactly the
		// rows that exist and nothing else.
		wantIDs := make([]string, 0, 3)
		for i := 0; i < 3; i++ {
			v := makeVHTLC()
			require.NoError(t, repo.Add(ctx, v))
			wantIDs = append(wantIDs, v.Id)
		}

		for _, size := range boundarySizes() {
			t.Run(fmt.Sprintf("size=%d", size), func(t *testing.T) {
				ids := padIDs(wantIDs, size, "vhtlc-absent")

				got, err := repo.GetByIds(ctx, ids)
				require.NoError(t, err)

				gotIDs := make([]string, 0, len(got))
				for _, v := range got {
					gotIDs = append(gotIDs, v.Id)
				}

				// Exact ownership: every requested-and-existing row, each once.
				require.ElementsMatch(t, expectedSubset(wantIDs, ids), gotIDs)
				require.Len(t, gotIDs, len(dedupe(gotIDs)), "chunking must not duplicate rows")
			})
		}
	})
}

func testDelegateInputsBoundary(t *testing.T, repo domain.DelegateRepository) {
	t.Run("get pending task ids by inputs over the bind variable limit", func(t *testing.T) {
		// A pending task whose single input sits in the *last* chunk, proving
		// every chunk is queried rather than just the first.
		markerInput := outpointAt(999_001)
		task := makeDelegateTaskWithInputs("boundary_inputs_task", markerInput)
		require.NoError(t, repo.Add(ctx, task))

		for _, size := range boundarySizes() {
			t.Run(fmt.Sprintf("size=%d", size), func(t *testing.T) {
				inputs := make([]wire.OutPoint, 0, size)
				for i := 0; i < size-1; i++ {
					inputs = append(inputs, outpointAt(i))
				}
				inputs = append(inputs, markerInput)

				got, err := repo.GetPendingTaskIDsByInputs(ctx, inputs)
				require.NoError(t, err)
				require.Equal(t, []string{task.ID}, got,
					"the task must be reported exactly once across all chunks")
			})
		}
	})
}

// padIDs returns a slice of exactly size entries containing every id in want,
// padded with ids that do not exist in the store. The real ids are placed at the
// end so a partial (first-chunk-only) implementation cannot pass by accident.
func padIDs(want []string, size int, prefix string) []string {
	if size <= len(want) {
		return append([]string(nil), want[:size]...)
	}

	ids := make([]string, 0, size)
	for i := 0; i < size-len(want); i++ {
		ids = append(ids, fmt.Sprintf("%s-%d", prefix, i))
	}
	return append(ids, want...)
}

// expectedSubset returns the entries of want that actually appear in requested.
func expectedSubset(want, requested []string) []string {
	present := make(map[string]struct{}, len(requested))
	for _, id := range requested {
		present[id] = struct{}{}
	}

	out := make([]string, 0, len(want))
	for _, id := range want {
		if _, ok := present[id]; ok {
			out = append(out, id)
		}
	}
	return out
}

func dedupe(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func outpointAt(i int) wire.OutPoint {
	hash, _ := chainhash.NewHashFromStr(
		fmt.Sprintf("%064x", 1),
	)
	return wire.OutPoint{Hash: *hash, Index: uint32(i)}
}

func makeDelegateTaskWithInputs(id string, inputs ...wire.OutPoint) domain.DelegateTask {
	forfeits := make(map[wire.OutPoint]string, len(inputs))
	for _, in := range inputs {
		forfeits[in] = "forfeit_tx_hex"
	}

	return domain.DelegateTask{
		ID: id,
		Intent: domain.Intent{
			Message: "boundary_message",
			Proof:   "boundary_proof",
			Txid:    "txid_" + id,
			Inputs:  inputs,
		},
		ForfeitTxs:        forfeits,
		Fee:               1000,
		DelegatePublicKey: "delegate_pubkey",
		ScheduledAt:       time.Now(),
		Status:            domain.DelegateTaskStatusPending,
	}
}
