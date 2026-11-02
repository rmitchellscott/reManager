// Package cloudsync relies on every store merging deterministically, so two hosts that exchanged payloads both ways settle on the same value.
package cloudsync

import (
	"context"
	"sync"

	"reManager/internal/debug"
)

type Domain interface {
	SyncKey() string
	ExportSync() (string, error)
	MergeSync(remote string) (changed bool, err error)
}

type KeyValueStore interface {
	Get(key string) (string, bool)
	Set(key, value string)
}

type Syncer struct {
	store    KeyValueStore
	domains  map[string]Domain
	onMerged func(syncKey string)

	mu      sync.Mutex
	pending map[string]bool
	wake    chan struct{}
}

func NewSyncer(onMerged func(syncKey string), domains ...Domain) *Syncer {
	s := &Syncer{
		domains:  map[string]Domain{},
		onMerged: onMerged,
		pending:  map[string]bool{},
		wake:     make(chan struct{}, 1),
	}
	for _, domain := range domains {
		s.domains[domain.SyncKey()] = domain
	}
	return s
}

// Changed may run before Start, because the store's change callback is registered before the store opens.
func (s *Syncer) Start(ctx context.Context, store KeyValueStore) {
	s.store = store
	for key := range s.domains {
		s.Changed(key)
	}
	go s.run(ctx)
}

// Changed never blocks, so a store can call it while holding its own lock.
func (s *Syncer) Changed(syncKey string) {
	s.mu.Lock()
	s.pending[syncKey] = true
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Syncer) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		}
		s.mu.Lock()
		keys := s.pending
		s.pending = map[string]bool{}
		s.mu.Unlock()
		for key := range keys {
			s.reconcile(key)
		}
	}
}

// Publishing only a changed value matters: every publish wakes the other hosts, so republishing unconditionally would never go quiet.
func (s *Syncer) reconcile(syncKey string) {
	domain, ok := s.domains[syncKey]
	if !ok {
		return
	}

	remote, published := s.store.Get(syncKey)
	if published {
		changed, err := domain.MergeSync(remote)
		if err != nil {
			debug.Printf("[DEBUG] cloudsync: merge %s failed: %v\n", syncKey, err)
		} else if changed && s.onMerged != nil {
			s.onMerged(syncKey)
		}
	}

	local, err := domain.ExportSync()
	if err != nil {
		debug.Printf("[DEBUG] cloudsync: export %s failed: %v\n", syncKey, err)
		return
	}
	if !published || local != remote {
		s.store.Set(syncKey, local)
	}
}
