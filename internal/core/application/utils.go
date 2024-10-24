package application

import "sync"

type listener[T any] struct {
	id string
	ch chan T
}

type listenerHandler[T any] struct {
	lock      *sync.Mutex
	listeners []*listener[T]
}

func newListenerHandler[T any]() *listenerHandler[T] {
	return &listenerHandler[T]{
		lock:      &sync.Mutex{},
		listeners: make([]*listener[T], 0),
	}
}

func (h *listenerHandler[T]) push(l *listener[T]) {
	h.lock.Lock()
	defer h.lock.Unlock()

	h.listeners = append(h.listeners, l)
}

func (h *listenerHandler[T]) remove(id string) {
	h.lock.Lock()
	defer h.lock.Unlock()

	for i, listener := range h.listeners {
		if listener.id == id {
			h.listeners = append(h.listeners[:i], h.listeners[i+1:]...)
			return
		}
	}
}
