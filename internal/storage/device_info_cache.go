package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type CachedDeviceInfo struct {
	MachineType          string            `json:"machineType"`
	DeviceType           string            `json:"deviceType,omitempty"`
	Arch                 string            `json:"arch,omitempty"`
	FirmwareVersion      string            `json:"firmwareVersion"`
	OSVersion            string            `json:"osVersion,omitempty"`
	InstalledPackages    map[string]string `json:"installedPackages,omitempty"`
	UpdateServiceEnabled bool              `json:"updateServiceEnabled"`
	UpdateServiceRunning bool              `json:"updateServiceRunning"`
	HashtabInstalled     bool              `json:"hashtabInstalled"`
	HashtabVersion       string            `json:"hashtabVersion,omitempty"`
	HashtabNeedsRebuild  bool              `json:"hashtabNeedsRebuild"`
	LauncherctlInstalled bool              `json:"launcherctlInstalled"`
	CurrentLauncher      string            `json:"currentLauncher,omitempty"`
	ActiveLauncher       string            `json:"activeLauncher,omitempty"`
	PartitionInfo        any               `json:"partitionInfo,omitempty"`
	Timezone             string            `json:"timezone,omitempty"`
	CachedAt             int64             `json:"cachedAt"`
	// Orders snapshots between hosts in milliseconds; CachedAt's second resolution cannot place a snapshot taken right after its own removal, as a reconnect to a replaced tablet does.
	SyncedAt int64 `json:"syncedAt,omitempty"`
}

type DeviceInfoCacheStore struct {
	syncHooks
	configPath string
	mu         sync.RWMutex
}

func NewDeviceInfoCacheStore() (*DeviceInfoCacheStore, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}

	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	return &DeviceInfoCacheStore{
		configPath: filepath.Join(configDir, "device_info_cache.json"),
	}, nil
}

func (s *DeviceInfoCacheStore) Get(deviceID string) (*CachedDeviceInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cache, err := s.readAll()
	if err != nil {
		return nil, err
	}

	info, ok := cache[deviceID]
	if !ok {
		return nil, nil
	}
	return &info, nil
}

func (s *DeviceInfoCacheStore) Set(deviceID string, info CachedDeviceInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cache, err := s.readAll()
	if err != nil {
		return err
	}

	info.SyncedAt = s.readdStamp(DeviceInfoSyncKey, deviceID)
	cache[deviceID] = info

	if err := s.writeAll(cache); err != nil {
		return err
	}
	s.changed()
	return nil
}

func (s *DeviceInfoCacheStore) Delete(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cache, err := s.readAll()
	if err != nil {
		return err
	}

	removed, existed := cache[deviceID]
	delete(cache, deviceID)

	if err := s.writeAll(cache); err != nil {
		return err
	}
	if existed {
		s.recordRemoval(DeviceInfoSyncKey, deviceID, cachedInfoStamp(removed))
		s.changed()
	}
	return nil
}

func (s *DeviceInfoCacheStore) Count() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cache, err := s.readAll()
	if err != nil {
		return 0, err
	}
	return len(cache), nil
}

// Tombstones every snapshot, since a clear that only reached this host would be undone by the next merge.
func (s *DeviceInfoCacheStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cache, err := s.readAll()
	if err != nil {
		return err
	}
	if err := os.Remove(s.configPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	for deviceID, info := range cache {
		s.recordRemoval(DeviceInfoSyncKey, deviceID, cachedInfoStamp(info))
	}
	if len(cache) > 0 {
		s.changed()
	}
	return nil
}

func (s *DeviceInfoCacheStore) readAll() (map[string]CachedDeviceInfo, error) {
	data, err := os.ReadFile(s.configPath)
	if os.IsNotExist(err) {
		return make(map[string]CachedDeviceInfo), nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read device info cache: %w", err)
	}

	var cache map[string]CachedDeviceInfo
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("failed to parse device info cache: %w", err)
	}
	return cache, nil
}

func (s *DeviceInfoCacheStore) writeAll(cache map[string]CachedDeviceInfo) error {
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal device info cache: %w", err)
	}

	if err := writeFileAtomic(s.configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write device info cache: %w", err)
	}
	return nil
}

type cachedDeviceEntry struct {
	DeviceID string           `json:"deviceId"`
	Info     CachedDeviceInfo `json:"info"`
}

func cachedInfoStamp(info CachedDeviceInfo) int64 {
	return max(info.SyncedAt, info.CachedAt*1000)
}

func cachedInfoEntry(entry cachedDeviceEntry) syncedEntry[cachedDeviceEntry] {
	return syncedEntry[cachedDeviceEntry]{ID: entry.DeviceID, Stamp: cachedInfoStamp(entry.Info), Value: entry}
}

func (s *DeviceInfoCacheStore) SyncKey() string { return string(DeviceInfoSyncKey) }

func (s *DeviceInfoCacheStore) ExportSync() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cache, err := s.readAll()
	if err != nil {
		return "", err
	}
	var entries []syncedEntry[cachedDeviceEntry]
	for deviceID, info := range cache {
		entries = append(entries, cachedInfoEntry(cachedDeviceEntry{DeviceID: deviceID, Info: info}))
	}
	return encodeCollection(entries, s.removed(DeviceInfoSyncKey))
}

func (s *DeviceInfoCacheStore) MergeSync(remote string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cache, err := s.readAll()
	if err != nil {
		return false, err
	}
	var local []syncedEntry[cachedDeviceEntry]
	for deviceID, info := range cache {
		local = append(local, cachedInfoEntry(cachedDeviceEntry{DeviceID: deviceID, Info: info}))
	}

	merge, err := mergeInto(&s.syncHooks, DeviceInfoSyncKey, local, remote, cachedInfoEntry)
	if err != nil || !merge.changed {
		return false, err
	}
	merged := make(map[string]CachedDeviceInfo, len(merge.entries))
	for _, entry := range merge.entries {
		merged[entry.ID] = entry.Value.Info
	}
	if err := s.writeAll(merged); err != nil {
		return false, err
	}
	return true, s.replaceRemoved(DeviceInfoSyncKey, merge.removed)
}
