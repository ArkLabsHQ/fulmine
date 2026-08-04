package sqlitedb

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChunkSlice(t *testing.T) {
	t.Run("empty input yields no chunks", func(t *testing.T) {
		require.Empty(t, chunkSlice([]string(nil), 3))
		require.Empty(t, chunkSlice([]string{}, 3))
	})

	t.Run("non-positive size yields no chunks", func(t *testing.T) {
		require.Empty(t, chunkSlice([]string{"a"}, 0))
		require.Empty(t, chunkSlice([]string{"a"}, -1))
	})

	t.Run("splits without losing or reordering elements", func(t *testing.T) {
		for _, total := range []int{1, 2, 3, 4, 5, 9, 10, 11} {
			items := make([]int, total)
			for i := range items {
				items[i] = i
			}

			for _, size := range []int{1, 2, 3, 10} {
				chunks := chunkSlice(items, size)

				flat := make([]int, 0, total)
				for i, chunk := range chunks {
					require.LessOrEqual(t, len(chunk), size)
					if i < len(chunks)-1 {
						require.Len(t, chunk, size, "only the final chunk may be short")
					}
					require.NotEmpty(t, chunk, "chunks must never be empty")
					flat = append(flat, chunk...)
				}

				require.Equal(t, items, flat,
					fmt.Sprintf("total=%d size=%d must round-trip exactly", total, size),
				)
			}
		}
	})

	t.Run("a size larger than the input yields one chunk", func(t *testing.T) {
		// math.MaxInt is the boundary for the capacity calculation: computing it
		// as len(items)+size-1 wraps negative for any input of two or more.
		require.Equal(t, [][]int{{0, 1}}, chunkSlice([]int{0, 1}, math.MaxInt))
		require.Equal(t, [][]int{{0}}, chunkSlice([]int{0}, math.MaxInt))
		require.Equal(t, [][]int{{0, 1, 2}}, chunkSlice([]int{0, 1, 2}, math.MaxInt))
	})

	t.Run("stays within the sqlite bind variable limit", func(t *testing.T) {
		// The limit this bound exists to respect. Guards against someone raising
		// maxBindVariablesPerStatement past what SQLite will prepare.
		const sqliteMaxVariableNumber = 32766
		require.Less(t, maxBindVariablesPerStatement, sqliteMaxVariableNumber)

		items := make([]string, 100_000)
		for _, chunk := range chunkSlice(items, maxBindVariablesPerStatement) {
			require.LessOrEqual(t, len(chunk), maxBindVariablesPerStatement)
		}
	})
}

func TestDedupeStrings(t *testing.T) {
	t.Run("preserves first-seen order", func(t *testing.T) {
		require.Equal(
			t,
			[]string{"c", "a", "b"},
			dedupeStrings([]string{"c", "a", "c", "b", "a", "c"}),
		)
	})

	t.Run("passes through already-unique input", func(t *testing.T) {
		require.Equal(t, []string{"a", "b"}, dedupeStrings([]string{"a", "b"}))
	})

	t.Run("returns an empty slice for empty input", func(t *testing.T) {
		require.Empty(t, dedupeStrings(nil))
	})
}
