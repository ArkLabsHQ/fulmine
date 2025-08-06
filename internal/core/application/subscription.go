package application

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/arkade-os/go-sdk/indexer"
	indexergrpc "github.com/arkade-os/go-sdk/indexer/grpc"
	log "github.com/sirupsen/logrus"
)

type scriptsStore interface {
	Get(ctx context.Context) ([]string, error)
	Add(ctx context.Context, subscribedScripts []string) (count int, err error)
	Delete(ctx context.Context, subscribedScripts []string) (count int, err error)
}

type subscriptionHandler struct {
	indexerBaseURL string
	scripts        scriptsStore
	onEvent        func(event *indexer.ScriptEvent)

	mu          sync.Mutex
	closeFn     func()
	cancelRetry func()
	id          string
}

func newSubscriptionHandler(indexerBaseURL string, scripts scriptsStore, onEvent func(event *indexer.ScriptEvent)) *subscriptionHandler {
	return &subscriptionHandler{
		indexerBaseURL: indexerBaseURL,
		scripts:        scripts,
		onEvent:        onEvent,
		mu:             sync.Mutex{},
		closeFn:        nil,
		id:             "",
	}
}

func (h *subscriptionHandler) createIndexerClient() (indexer.Indexer, error) {
	return indexergrpc.NewClient(h.indexerBaseURL)
}

func (h *subscriptionHandler) subscribe(ctx context.Context, scripts []string) error {
	count, err := h.scripts.Add(ctx, scripts)
	if err != nil {
		return fmt.Errorf("failed to add scripts: %w", err)
	}

	if count == 0 {
		return nil
	}

	log.Debugf("added %d scripts to subscription", count)

	h.mu.Lock()
	id := h.id
	h.mu.Unlock()

	if len(id) == 0 {
		if err := h.start(); err != nil {
			return err
		}
		return nil
	}

	indexerClient, err := h.createIndexerClient()
	if err != nil {
		return fmt.Errorf("failed to create indexer client: %w", err)
	}

	_, err = indexerClient.SubscribeForScripts(ctx, id, scripts)
	if err != nil {
		log.WithError(err).Warn("failed to update subscription, retrying...")
		h.stop()
		if err := h.start(); err != nil {
			return err
		}
		return nil
	}

	return nil
}

func (h *subscriptionHandler) unsubscribe(ctx context.Context, scripts []string) error {
	count, err := h.scripts.Delete(ctx, scripts)
	if err != nil {
		return fmt.Errorf("failed to remove scripts: %w", err)
	}

	if count == 0 {
		return nil
	}

	log.Debugf("removed %d scripts from subscription", count)

	h.mu.Lock()
	id := h.id
	h.mu.Unlock()

	if len(id) == 0 {
		scripts, err := h.scripts.Get(ctx)
		if err != nil {
			return fmt.Errorf("failed to get scripts: %w", err)
		}
		if len(scripts) == 0 {
			return nil
		}

		if err := h.start(); err != nil {
			return err
		}
		return nil
	}

	indexerClient, err := h.createIndexerClient()
	if err != nil {
		return fmt.Errorf("failed to create indexer client: %w", err)
	}

	err = indexerClient.UnsubscribeForScripts(ctx, id, scripts)
	if err != nil {
		log.WithError(err).Warn("failed to unsubscribe, retrying...")
		h.stop()
		if err := h.start(); err != nil {
			return err
		}
		return nil
	}

	scripts, err = h.scripts.Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to get scripts: %w", err)
	}

	if len(scripts) == 0 {
		h.stop()
		return nil
	}

	return nil
}

func (h *subscriptionHandler) stop() {
	if h.cancelRetry != nil {
		h.cancelRetry()
		h.cancelRetry = nil
	}

	h.close()
}

func (h *subscriptionHandler) close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closeFn != nil {
		h.closeFn()
		h.closeFn = nil
	}

	if len(h.id) != 0 {
		log.Debugf("stopped subscription %s", h.id)
		h.id = ""
	}
}

func (h *subscriptionHandler) start() error {
	ctx, cancel := context.WithCancel(context.Background())

	h.mu.Lock()
	h.cancelRetry = cancel
	h.mu.Unlock()

	scripts, err := h.scripts.Get(ctx)
	if err != nil {
		return err
	}

	if len(scripts) == 0 {
		h.stop()
		log.Debugf("no scripts to subscribe to, skip starting subscription")
		return nil
	}

	go func() {
		onError := func(err error) {
			h.close()
			log.WithError(err).Warn("retrying in 2 seconds")
			time.Sleep(2 * time.Second)
		}

		for {
			select {
			case <-ctx.Done():
				log.Debugf("context done, stop retrying to subscribe")
				return
			default:
			}

			log.Debugf("creating subscription...")

			if err := h.create(ctx); err != nil {
				onError(err)
				continue
			}

			log.Debugf("created subscription %s", h.id)

			indexerClient, err := h.createIndexerClient()
			if err != nil {
				onError(err)
				continue
			}

			subscriptionChannel, closeFn, err := indexerClient.GetSubscription(ctx, h.id)
			if err != nil {
				onError(err)
				continue
			}

			h.mu.Lock()
			h.closeFn = closeFn
			h.mu.Unlock()

			stopped := false
			for !stopped {
				select {
				case <-ctx.Done():
					return
				case event := <-subscriptionChannel:
					if event.Err != nil {
						onError(event.Err)
						stopped = true
						continue
					}

					log.Debugf("received transaction event: %s for subscription %s", event.Txid, h.id)
					go h.onEvent(event)
				}
			}
		}
	}()

	return nil
}

func (h *subscriptionHandler) create(ctx context.Context) error {
	var err error

	scripts, err := h.scripts.Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to get scripts: %w", err)
	}

	if len(scripts) == 0 {
		return fmt.Errorf("no scripts to subscribe to")
	}

	indexerClient, err := h.createIndexerClient()
	if err != nil {
		return fmt.Errorf("failed to create indexer client: %w", err)
	}

	subscriptionId, err := indexerClient.SubscribeForScripts(ctx, "", scripts)
	if err != nil {
		return fmt.Errorf("failed to subscribe for scripts: %w", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.id = subscriptionId

	return nil
}
