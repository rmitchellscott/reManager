package main

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"

	"reManager/internal/cloudsync"
	"reManager/internal/debug"
	"reManager/internal/storage"
)

const eventSyncChanged = "sync:changed"

func init() {
	application.RegisterEvent[storage.SyncKey](eventSyncChanged)
}

type syncedStore interface {
	cloudsync.Domain
	EnableSync(tombstones *storage.Tombstones, onChange func())
}

// startSync joins this host to the user's iCloud key-value store; other platforms, and Apple builds signed without the entitlement, keep every store local.
func (c *core) startSync(ctx context.Context) {
	var stores []syncedStore
	if c.settingsStore != nil {
		stores = append(stores, c.settingsStore)
	}
	if c.favoritesStore != nil {
		stores = append(stores, c.favoritesStore)
	}
	if c.followsStore != nil {
		stores = append(stores, c.followsStore)
	}
	if c.deviceStore != nil {
		stores = append(stores, c.deviceStore, c.deviceStore.Keys())
	}
	if c.deviceInfoCache != nil {
		stores = append(stores, c.deviceInfoCache)
	}
	if len(stores) == 0 {
		return
	}

	domains := make([]cloudsync.Domain, len(stores))
	for i, store := range stores {
		domains[i] = store
	}
	syncer := cloudsync.NewSyncer(func(syncKey string) {
		c.wails.Event.Emit(eventSyncChanged, storage.SyncKey(syncKey))
	}, domains...)

	keyValueStore, ok := cloudsync.Open(func(keys []string) {
		for _, key := range keys {
			syncer.Changed(key)
		}
	})
	if !ok {
		debug.Println("[DEBUG] cloudsync: no iCloud key-value store, sync is off")
		return
	}

	tombstones, err := storage.NewTombstones()
	if err != nil {
		debug.Printf("[DEBUG] cloudsync: tombstones unavailable, sync is off: %v\n", err)
		return
	}
	for _, store := range stores {
		syncKey := store.SyncKey()
		store.EnableSync(tombstones, func() { syncer.Changed(syncKey) })
	}
	syncer.Start(ctx, keyValueStore)
	debug.Println("[DEBUG] cloudsync: iCloud sync started")
}
