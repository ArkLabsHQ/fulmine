package application

import (
	"context"
	"fmt"
	"sync"

	"github.com/arkade-os/arkd/pkg/client-lib/indexer"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	log "github.com/sirupsen/logrus"
)

const logPrefix = "subscription handler:"

type scriptsStore interface {
	Get(ctx context.Context) ([]string, error)
	Add(ctx context.Context, subscribedScripts []string) (count int, err error)
	Delete(ctx context.Context, subscribedScripts []string) (count int, err error)
}

type subscriptionHandler struct {
	indexerClient indexer.Indexer
	scripts       scriptsStore
	onEvent       func(event indexer.ScriptEvent)

	mu          sync.Mutex
	closeFn     func()
	cancelRetry func()
	id          string
}

func newSubscriptionHandler(
	ctx context.Context, indexerClient indexer.Indexer,
	store scriptsStore, onEvent func(event indexer.ScriptEvent),
) (*subscriptionHandler, error) {
	scripts, err := store.Get(ctx)
	if err != nil {
		return nil, err
	}
	svc := &subscriptionHandler{
		indexerClient: indexerClient,
		scripts:       store,
		onEvent:       onEvent,
		mu:            sync.Mutex{},
		closeFn:       nil,
		id:            "",
	}
	if len(scripts) > 0 {
		if err := svc.start(scripts); err != nil {
			return nil, err
		}
	}
	return svc, nil
}

func (h *subscriptionHandler) subscribe(ctx context.Context, scripts []string) error {
	count, err := h.scripts.Add(ctx, scripts)
	if err != nil {
		return fmt.Errorf("failed to add scripts: %w", err)
	}

	if count == 0 {
		return nil
	}

	log.Debugf("%s added %d scripts to subscription", logPrefix, count)

	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.id) == 0 {
		return h.start(scripts)
	}

	return h.indexerClient.UpdateSubscription(ctx, h.id, scripts, nil)
}

func (h *subscriptionHandler) unsubscribe(ctx context.Context, scripts []string) error {
	count, err := h.scripts.Delete(ctx, scripts)
	if err != nil {
		return fmt.Errorf("failed to remove scripts: %w", err)
	}

	if count == 0 {
		return nil
	}

	log.Debugf("%s removed %d scripts from subscription", logPrefix, count)

	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.id) == 0 {
		return nil
	}

	return h.indexerClient.UpdateSubscription(ctx, h.id, nil, scripts)
}

func (h *subscriptionHandler) stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.cancelRetry != nil {
		h.cancelRetry()
		h.cancelRetry = nil
	}

	if h.closeFn != nil {
		h.closeFn()
		h.closeFn = nil
	}

	if len(h.id) != 0 {
		id := h.id
		h.id = ""
		log.Debugf("removed subscription %s", id)
	}
}

func (h *subscriptionHandler) start(scripts []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	h.cancelRetry = cancel

	log.Debugf("%s creating subscription...", logPrefix)

	id, subscriptionChannel, closeFn, err := h.indexerClient.NewSubscription(ctx, scripts)
	if err != nil {
		return err
	}

	h.closeFn = closeFn
	h.id = id
	log.Debugf("%s created subscription %s", logPrefix, h.id)

	go func() {
		waitForReconnection := false
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-subscriptionChannel:
				if !ok {
					return
				}

				if event.Connection != nil {
					if event.Connection.State == clientTypes.StreamConnectionStateDisconnected {
						waitForReconnection = true
						log.Debug("%s connection lost, waiting for reconnection...", logPrefix)
					}
					if waitForReconnection &&
						event.Connection.State == clientTypes.StreamConnectionStateReconnected {
						log.Debug("%s connection restored, resubscribing...", logPrefix)
					}
					if waitForReconnection &&
						event.Connection.State == clientTypes.StreamConnectionStateReady {
						log.Debug("%s restored subscriptions", logPrefix)
						waitForReconnection = false
					}
					continue
				}
				go h.onEvent(event)
			}
		}
	}()

	return nil
}
