package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type SyncKey string

const (
	SettingsSyncKey   SyncKey = "settings.v1"
	FavoritesSyncKey  SyncKey = "favorites.v1"
	FollowsSyncKey    SyncKey = "follows.v1"
	DevicesSyncKey    SyncKey = "devices.v1"
	KeysSyncKey       SyncKey = "keys.v1"
	DeviceInfoSyncKey SyncKey = "deviceinfo.v1"
)

// Kept apart from the collections so their file formats stay unchanged.
type Tombstones struct {
	path string
	mu   sync.Mutex
}

func NewTombstones() (*Tombstones, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, err
	}
	return &Tombstones{path: filepath.Join(configDir, "sync-removed.json")}, nil
}

func (t *Tombstones) load() (map[SyncKey]map[string]int64, error) {
	all := map[SyncKey]map[string]int64{}
	data, err := os.ReadFile(t.path)
	if os.IsNotExist(err) {
		return all, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &all); err != nil {
		if setAsideErr := setAsideUnreadable(t.path, err); setAsideErr != nil {
			return nil, setAsideErr
		}
		return map[SyncKey]map[string]int64{}, nil
	}
	return all, nil
}

func (t *Tombstones) write(all map[SyncKey]map[string]int64) error {
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(t.path, data, 0600)
}

func (t *Tombstones) Get(syncKey SyncKey) map[string]int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	all, _ := t.load()
	removed := all[syncKey]
	if removed == nil {
		removed = map[string]int64{}
	}
	return removed
}

func (t *Tombstones) Record(syncKey SyncKey, id string, removedEntryStamp int64) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	all, err := t.load()
	if err != nil {
		return err
	}
	if all[syncKey] == nil {
		all[syncKey] = map[string]int64{}
	}
	all[syncKey][id] = nextStamp(removedEntryStamp)
	return t.write(all)
}

func (t *Tombstones) Replace(syncKey SyncKey, removed map[string]int64) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	all, err := t.load()
	if err != nil {
		return err
	}
	all[syncKey] = removed
	return t.write(all)
}

// Left zero on hosts with no sync backend.
type syncHooks struct {
	tombstones *Tombstones
	onChange   func()
}

// EnableSync must run before the store is used from more than one goroutine.
func (h *syncHooks) EnableSync(tombstones *Tombstones, onChange func()) {
	h.tombstones = tombstones
	h.onChange = onChange
}

func (h *syncHooks) changed() {
	if h.onChange != nil {
		h.onChange()
	}
}

func (h *syncHooks) recordRemoval(syncKey SyncKey, id string, removedEntryStamp int64) {
	if h.tombstones != nil {
		_ = h.tombstones.Record(syncKey, id, removedEntryStamp)
	}
}

func (h *syncHooks) readdStamp(syncKey SyncKey, id string) int64 {
	return nextStamp(h.removed(syncKey)[id])
}

func (h *syncHooks) removed(syncKey SyncKey) map[string]int64 {
	if h.tombstones == nil {
		return map[string]int64{}
	}
	return h.tombstones.Get(syncKey)
}

func (h *syncHooks) replaceRemoved(syncKey SyncKey, removed map[string]int64) error {
	if h.tombstones == nil {
		return nil
	}
	return h.tombstones.Replace(syncKey, removed)
}

type collectionMerge[T any] struct {
	entries []syncedEntry[T]
	removed map[string]int64
	changed bool
}

func mergeInto[T any](h *syncHooks, syncKey SyncKey, local []syncedEntry[T], remotePayload string, remoteEntry func(T) syncedEntry[T]) (collectionMerge[T], error) {
	remote, err := decodeCollection[T](remotePayload)
	if err != nil {
		return collectionMerge[T]{}, err
	}
	remoteEntries := make([]syncedEntry[T], 0, len(remote.Entries))
	for _, value := range remote.Entries {
		remoteEntries = append(remoteEntries, remoteEntry(value))
	}

	localRemoved := h.removed(syncKey)
	before, err := encodeCollection(local, localRemoved)
	if err != nil {
		return collectionMerge[T]{}, err
	}
	merged, removed := mergeCollections(local, remoteEntries, localRemoved, remote.Removed)
	after, err := encodeCollection(merged, removed)
	if err != nil {
		return collectionMerge[T]{}, err
	}
	return collectionMerge[T]{entries: merged, removed: removed, changed: before != after}, nil
}
