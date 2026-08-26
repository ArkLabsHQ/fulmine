package sqlitedb

// maxBindVariablesPerStatement bounds how many caller-supplied values a single
// statement may carry.
//
// Queries built with sqlc.slice() expand into one bind variable per element, and
// SQLite refuses to prepare a statement with more variables than its compiled-in
// SQLITE_MAX_VARIABLE_NUMBER (32766 in modernc.org/sqlite) — it fails with
// "too many SQL variables". Some of those statements also bind fixed variables of
// their own before the slice, so the usable headroom is not the raw limit.
//
// Rather than tracking per-query headroom we stay far below the limit. It is also
// under the 999 that SQLite used before 3.32, so the bound survives a driver or
// build-flag change. Keeping it a fixed round number means every full chunk
// produces an identical statement string, which the driver's statement cache can
// reuse; only the trailing partial chunk differs.
//
// The statements that bind a fixed variable of their own before the slice
// (SuccessDelegateTasks, FailDelegateTasks) sit one higher at 901, still far
// under either ceiling.
const maxBindVariablesPerStatement = 900

// chunkSlice splits items into consecutive batches of at most size elements.
// An empty input yields no batches, so callers iterating the result issue no
// query at all for an empty slice.
//
// The batches alias items rather than copying it, so a caller that mutates a
// batch mutates the input. Every caller here passes a slice of ids straight to a
// query and never writes to it.
func chunkSlice[T any](items []T, size int) [][]T {
	if size <= 0 || len(items) == 0 {
		return nil
	}

	// Round up as 1+(len-1)/size rather than (len+size-1)/size, which wraps
	// negative for a size near math.MaxInt. The guard above keeps len(items)-1
	// non-negative.
	chunks := make([][]T, 0, 1+(len(items)-1)/size)
	for start := 0; start < len(items); start += size {
		end := start + size
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, items[start:end])
	}

	return chunks
}

// dedupeStrings returns values without repeats, preserving first-seen order.
//
// Callers use it for two distinct reasons: to keep a duplicated input from
// straddling a chunk boundary and yielding the same row twice, and to restore a
// SELECT DISTINCT that only held within a single statement.
func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}

	return out
}
