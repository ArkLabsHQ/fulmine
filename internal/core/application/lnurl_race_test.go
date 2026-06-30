package application

import (
	"context"
	"sync"
	"testing"

	"github.com/ArkLabsHQ/fulmine/internal/lnurl"
)

// TestCurrentLnurlConcurrentLifecycle runs the web read path (CurrentLnurl)
// concurrently with writers that swap lnurlClient the way startLnurlReceiver and
// LockNode do. Before lnurlMu guarded the pair, CurrentLnurl's check-then-use
// (read the pointer, then re-read it to call .Lnurl()) could dereference a
// freshly nil'd client and panic, and the unsynchronized access tripped the race
// detector. This guards that fix and must stay clean under `go test -race`.
func TestCurrentLnurlConcurrentLifecycle(t *testing.T) {
	s := &Service{}
	client := lnurl.New("https://example.com", []byte{1, 2, 3}, func(context.Context, uint64) (string, error) {
		return "", nil
	})

	stop := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() { // the web handler read path
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = s.CurrentLnurl()
			}
		}
	}()

	for i := 0; i < 5000; i++ { // unlock sets a client, lock tears it down
		s.lnurlMu.Lock()
		s.lnurlClient = client
		s.lnurlMu.Unlock()
		s.lnurlMu.Lock()
		s.lnurlClient = nil
		s.lnurlMu.Unlock()
	}

	close(stop)
	wg.Wait()
}
