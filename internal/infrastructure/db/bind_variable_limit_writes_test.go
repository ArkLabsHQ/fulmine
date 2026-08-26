package db_test

import (
	"fmt"
	"testing"

	"github.com/ArkLabsHQ/fulmine/internal/core/domain"
	"github.com/ArkLabsHQ/fulmine/internal/infrastructure/db"
	"github.com/stretchr/testify/require"
)

// TestBindVariableLimitStatusUpdates covers the delegate task status transitions,
// which expand an id slice into bind variables the same way the reads do.
//
// CancelTasks binds one variable per id. CompleteTasks and FailTasks bind an
// extra leading variable (commitment txid / fail reason) before the id list, so
// they overflow one id *earlier* than CancelTasks does — a fix that chunked at
// exactly the raw limit would still have been broken for those two.
func TestBindVariableLimitStatusUpdates(t *testing.T) {
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

			repo := svc.Delegate()

			updates := []struct {
				name   string
				update func(ids []string) error
				status domain.DelegateTaskStatus
			}{
				{
					name:   "cancel",
					update: func(ids []string) error { return repo.CancelTasks(ctx, ids...) },
					status: domain.DelegateTaskStatusCancelled,
				},
				{
					name: "complete",
					update: func(ids []string) error {
						return repo.CompleteTasks(ctx, "boundary_commitment_txid", ids...)
					},
					status: domain.DelegateTaskStatusCompleted,
				},
				{
					name: "fail",
					update: func(ids []string) error {
						return repo.FailTasks(ctx, "boundary failure reason", ids...)
					},
					status: domain.DelegateTaskStatusFailed,
				},
			}

			for ui, u := range updates {
				t.Run(u.name+" tasks over the bind variable limit", func(t *testing.T) {
					// A task that is never named in any batch, so it pins the
					// negative invariant at every size: a chunked update must not
					// transition anything outside its id set.
					bystander := fmt.Sprintf("boundary_%s_bystander", u.name)
					require.NoError(t, repo.Add(ctx, makeDelegateTaskWithInputs(
						bystander, outpointAt(900_000+ui),
					)))

					for _, size := range boundarySizes() {
						t.Run(fmt.Sprintf("size=%d", size), func(t *testing.T) {
							// Two real pending tasks: one first, one last. The
							// update must reach the final chunk to transition the
							// second, so a first-chunk-only implementation fails.
							first := fmt.Sprintf("boundary_%s_%d_first", u.name, size)
							last := fmt.Sprintf("boundary_%s_%d_last", u.name, size)
							for i, id := range []string{first, last} {
								require.NoError(t, repo.Add(ctx, makeDelegateTaskWithInputs(
									id, outpointAt(500_000+size*2+i),
								)))
							}

							ids := interleaveIDs(first, last, size, "task-absent-"+u.name)
							require.NoError(t, u.update(ids))

							// Only assert on ids the batch could actually hold;
							// a single-slot batch carries first alone.
							updated := []string{first}
							if size >= 2 {
								updated = append(updated, last)
							}
							for _, id := range updated {
								got, err := repo.GetByID(ctx, id)
								require.NoError(t, err)
								require.Equal(t, u.status, got.Status,
									"status update must be applied across every chunk")
							}

							// The single-slot batch cannot carry last, so it doubles
							// as a bystander for that size.
							if size < 2 {
								untouched, err := repo.GetByID(ctx, last)
								require.NoError(t, err)
								require.Equal(t, domain.DelegateTaskStatusPending, untouched.Status,
									"tasks outside the id set must not be transitioned")
							}

							// Checked at every size, including the ones where
							// chunking actually activates.
							untouched, err := repo.GetByID(ctx, bystander)
							require.NoError(t, err)
							require.Equal(t, domain.DelegateTaskStatusPending, untouched.Status,
								"a task outside the id set must not be transitioned")
						})
					}
				})
			}
		})
	}
}

// interleaveIDs builds a slice of exactly size entries with first at the head and
// last at the tail, padded in between with ids that do not exist in the store.
func interleaveIDs(first, last string, size int, prefix string) []string {
	if size <= 1 {
		return []string{first}
	}

	ids := make([]string, 0, size)
	ids = append(ids, first)
	for i := 0; i < size-2; i++ {
		ids = append(ids, fmt.Sprintf("%s-%d", prefix, i))
	}
	return append(ids, last)
}
