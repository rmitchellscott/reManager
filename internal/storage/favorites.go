package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// AddedAt is carried per entry so two copies merge by taking the newer side of each name; removals are tombstoned under FavoritesSyncKey.
type Favorite struct {
	Name    string `json:"name"`
	AddedAt string `json:"addedAt"`
}

// In its own file rather than settings.json, so the sync container carries the starred set without host-local settings.
type Favorites struct {
	Version  int        `json:"version"`
	Packages []Favorite `json:"packages"`
}

const favoritesVersion = 1

type FavoritesStore struct {
	syncHooks
	configPath string
	mu         sync.RWMutex
}

func NewFavoritesStore() (*FavoritesStore, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, err
	}

	return &FavoritesStore{
		configPath: filepath.Join(configDir, "favorites.json"),
	}, nil
}

func (s *FavoritesStore) Load() (*Favorites, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.load()
}

func (s *FavoritesStore) load() (*Favorites, error) {
	data, err := os.ReadFile(s.configPath)
	if os.IsNotExist(err) {
		return &Favorites{Version: favoritesVersion}, nil
	}
	if err != nil {
		return nil, err
	}

	var favorites Favorites
	if err := json.Unmarshal(data, &favorites); err != nil {
		if setAsideErr := setAsideUnreadable(s.configPath, err); setAsideErr != nil {
			return nil, setAsideErr
		}
		return &Favorites{Version: favoritesVersion}, nil
	}
	return &favorites, nil
}

func (s *FavoritesStore) save(favorites *Favorites) error {
	favorites.Version = favoritesVersion
	sort.Slice(favorites.Packages, func(i, j int) bool {
		return favorites.Packages[i].Name < favorites.Packages[j].Name
	})

	data, err := json.MarshalIndent(favorites, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.configPath, data, 0600)
}

func (s *FavoritesStore) Toggle(name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	favorites, err := s.load()
	if err != nil {
		return false, err
	}

	for i, fav := range favorites.Packages {
		if fav.Name == name {
			favorites.Packages = append(favorites.Packages[:i], favorites.Packages[i+1:]...)
			if err := s.save(favorites); err != nil {
				return false, err
			}
			s.recordRemoval(FavoritesSyncKey, name, rfc3339Millis(fav.AddedAt))
			s.changed()
			return false, nil
		}
	}

	favorites.Packages = append(favorites.Packages, Favorite{
		Name:    name,
		AddedAt: millisRFC3339(s.readdStamp(FavoritesSyncKey, name)),
	})
	if err := s.save(favorites); err != nil {
		return false, err
	}
	s.changed()
	return true, nil
}

func favoriteEntry(fav Favorite) syncedEntry[Favorite] {
	return syncedEntry[Favorite]{ID: fav.Name, Stamp: rfc3339Millis(fav.AddedAt), Value: fav}
}

func (s *FavoritesStore) SyncKey() string { return string(FavoritesSyncKey) }

func (s *FavoritesStore) ExportSync() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	favorites, err := s.load()
	if err != nil {
		return "", err
	}
	var entries []syncedEntry[Favorite]
	for _, fav := range favorites.Packages {
		entries = append(entries, favoriteEntry(fav))
	}
	return encodeCollection(entries, s.removed(FavoritesSyncKey))
}

func (s *FavoritesStore) MergeSync(remote string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	favorites, err := s.load()
	if err != nil {
		return false, err
	}
	var local []syncedEntry[Favorite]
	for _, fav := range favorites.Packages {
		local = append(local, favoriteEntry(fav))
	}

	merge, err := mergeInto(&s.syncHooks, FavoritesSyncKey, local, remote, favoriteEntry)
	if err != nil || !merge.changed {
		return false, err
	}
	favorites.Packages = favorites.Packages[:0]
	for _, entry := range merge.entries {
		favorites.Packages = append(favorites.Packages, entry.Value)
	}
	if err := s.save(favorites); err != nil {
		return false, err
	}
	return true, s.replaceRemoved(FavoritesSyncKey, merge.removed)
}
