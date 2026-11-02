package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// FollowedAt is the feed baseline: only packages released after it are new, so following never produces a backlog. Seen is scoped per follow, so unfollowing drops it.
type Follow struct {
	Name       string   `json:"name"`
	FollowedAt string   `json:"followedAt"`
	UpdatedAt  string   `json:"updatedAt,omitempty"`
	Seen       []string `json:"seen,omitempty"`
}

// In its own file (like Favorites) so the sync container carries the follow graph without host-local settings.
type Follows struct {
	Version    int      `json:"version"`
	Developers []Follow `json:"developers"`
}

const followsVersion = 1

type FollowsStore struct {
	syncHooks
	configPath string
	mu         sync.RWMutex
}

func NewFollowsStore() (*FollowsStore, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, err
	}

	return &FollowsStore{
		configPath: filepath.Join(configDir, "follows.json"),
	}, nil
}

func (s *FollowsStore) Load() (*Follows, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.load()
}

func (s *FollowsStore) load() (*Follows, error) {
	data, err := os.ReadFile(s.configPath)
	if os.IsNotExist(err) {
		return &Follows{Version: followsVersion}, nil
	}
	if err != nil {
		return nil, err
	}

	var follows Follows
	if err := json.Unmarshal(data, &follows); err != nil {
		if setAsideErr := setAsideUnreadable(s.configPath, err); setAsideErr != nil {
			return nil, setAsideErr
		}
		return &Follows{Version: followsVersion}, nil
	}
	return &follows, nil
}

func (s *FollowsStore) save(follows *Follows) error {
	follows.Version = followsVersion
	sort.Slice(follows.Developers, func(i, j int) bool {
		return follows.Developers[i].Name < follows.Developers[j].Name
	})
	for i := range follows.Developers {
		sort.Strings(follows.Developers[i].Seen)
	}

	data, err := json.MarshalIndent(follows, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.configPath, data, 0600)
}

func (s *FollowsStore) Toggle(name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	follows, err := s.load()
	if err != nil {
		return false, err
	}

	for i, follow := range follows.Developers {
		if follow.Name == name {
			follows.Developers = append(follows.Developers[:i], follows.Developers[i+1:]...)
			if err := s.save(follows); err != nil {
				return false, err
			}
			s.recordRemoval(FollowsSyncKey, name, followEntry(follow).Stamp)
			s.changed()
			return false, nil
		}
	}

	follows.Developers = append(follows.Developers, Follow{
		Name:       name,
		FollowedAt: millisRFC3339(s.readdStamp(FollowsSyncKey, name)),
	})
	if err := s.save(follows); err != nil {
		return false, err
	}
	s.changed()
	return true, nil
}

func (s *FollowsStore) saveChanged(follows *Follows) error {
	if err := s.save(follows); err != nil {
		return err
	}
	s.changed()
	return nil
}

func touched(follow Follow) string { return millisRFC3339(nextStamp(followEntry(follow).Stamp)) }

// Following is a prerequisite: a seen entry with no follow to hang off would never be read back or pruned.
func (s *FollowsStore) SetSeen(developer, pkg string, seen bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	follows, err := s.load()
	if err != nil {
		return err
	}

	for i, follow := range follows.Developers {
		if follow.Name != developer {
			continue
		}
		for j, name := range follow.Seen {
			if name != pkg {
				continue
			}
			if seen {
				return nil
			}
			follows.Developers[i].Seen = append(follow.Seen[:j], follow.Seen[j+1:]...)
			follows.Developers[i].UpdatedAt = touched(follow)
			return s.saveChanged(follows)
		}
		if !seen {
			return nil
		}
		follows.Developers[i].Seen = append(follow.Seen, pkg)
		follows.Developers[i].UpdatedAt = touched(follow)
		return s.saveChanged(follows)
	}
	return nil
}

// An empty index is treated as a failed load and skipped, otherwise one bad fetch would wipe every seen entry.
func (s *FollowsStore) PruneSeen(indexed map[string]bool) error {
	if len(indexed) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	follows, err := s.load()
	if err != nil {
		return err
	}

	changed := false
	for i, follow := range follows.Developers {
		kept := follow.Seen[:0]
		for _, name := range follow.Seen {
			if indexed[name] {
				kept = append(kept, name)
			}
		}
		if len(kept) != len(follow.Seen) {
			changed = true
			follows.Developers[i].UpdatedAt = touched(follow)
		}
		follows.Developers[i].Seen = kept
	}
	if !changed {
		return nil
	}
	return s.saveChanged(follows)
}

func followEntry(follow Follow) syncedEntry[Follow] {
	stamp := max(rfc3339Millis(follow.FollowedAt), rfc3339Millis(follow.UpdatedAt))
	return syncedEntry[Follow]{ID: follow.Name, Stamp: stamp, Value: follow}
}

func (s *FollowsStore) SyncKey() string { return string(FollowsSyncKey) }

func (s *FollowsStore) ExportSync() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	follows, err := s.load()
	if err != nil {
		return "", err
	}
	var entries []syncedEntry[Follow]
	for _, follow := range follows.Developers {
		entries = append(entries, followEntry(follow))
	}
	return encodeCollection(entries, s.removed(FollowsSyncKey))
}

func (s *FollowsStore) MergeSync(remote string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	follows, err := s.load()
	if err != nil {
		return false, err
	}
	var local []syncedEntry[Follow]
	for _, follow := range follows.Developers {
		local = append(local, followEntry(follow))
	}

	merge, err := mergeInto(&s.syncHooks, FollowsSyncKey, local, remote, followEntry)
	if err != nil || !merge.changed {
		return false, err
	}
	follows.Developers = follows.Developers[:0]
	for _, entry := range merge.entries {
		follows.Developers = append(follows.Developers, entry.Value)
	}
	if err := s.save(follows); err != nil {
		return false, err
	}
	return true, s.replaceRemoved(FollowsSyncKey, merge.removed)
}
