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
	FirmwareVersion      string            `json:"firmwareVersion"`
	OSVersion            string            `json:"osVersion,omitempty"`
	InstalledPackages    map[string]string  `json:"installedPackages,omitempty"`
	UpdateServiceEnabled bool              `json:"updateServiceEnabled"`
	UpdateServiceRunning bool              `json:"updateServiceRunning"`
	HashtabInstalled     bool              `json:"hashtabInstalled"`
	HashtabVersion       string            `json:"hashtabVersion,omitempty"`
	HashtabNeedsRebuild  bool              `json:"hashtabNeedsRebuild"`
	PartitionInfo        any               `json:"partitionInfo,omitempty"`
	Timezone             string            `json:"timezone,omitempty"`
	CachedAt             int64             `json:"cachedAt"`
}

type DeviceInfoCacheStore struct {
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
		cache = make(map[string]CachedDeviceInfo)
	}

	cache[deviceID] = info

	return s.writeAll(cache)
}

func (s *DeviceInfoCacheStore) Delete(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cache, err := s.readAll()
	if err != nil {
		return nil
	}

	delete(cache, deviceID)

	return s.writeAll(cache)
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

	if err := os.WriteFile(s.configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write device info cache: %w", err)
	}
	return nil
}
