package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	maxBundleRecords  = 20
	bundleRetentionDays = 30
)

type BundleRecord struct {
	ID           string `json:"id"`
	RemoteID     string `json:"remoteId"`
	URL          string `json:"url"`
	DeviceName   string `json:"deviceName"`
	DeviceID     string `json:"deviceId"`
	UploadedAt   int64  `json:"uploadedAt"`
	LastAppended int64  `json:"lastAppended,omitempty"`
	ExpiresAt    int64  `json:"expiresAt,omitempty"`
}

type BundleStore struct {
	configPath  string
	secretStore *SecretStore
	mu          sync.RWMutex
}

func NewBundleStore() (*BundleStore, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}

	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	secretStore, err := NewSecretStore(serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to create secret store: %w", err)
	}

	return &BundleStore{
		configPath:  filepath.Join(configDir, "bundles.json"),
		secretStore: secretStore,
	}, nil
}

func (bs *BundleStore) GetAll() ([]BundleRecord, error) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	records, err := bs.readRecords()
	if err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	var active []BundleRecord
	pruned := false
	for _, r := range records {
		bs.computeExpiry(&r)
		if r.ExpiresAt > now {
			active = append(active, r)
		} else {
			bs.secretStore.Delete("bundle:" + r.RemoteID)
			pruned = true
		}
	}

	if pruned {
		bs.writeRecords(active)
	}

	sort.Slice(active, func(i, j int) bool {
		return active[i].UploadedAt > active[j].UploadedAt
	})

	return active, nil
}

func (bs *BundleStore) Get(remoteID string) (*BundleRecord, error) {
	records, err := bs.GetAll()
	if err != nil {
		return nil, err
	}
	for _, r := range records {
		if r.RemoteID == remoteID {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("bundle not found: %s", remoteID)
}

func (bs *BundleStore) Add(record BundleRecord, deleteToken string) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	records, err := bs.readRecords()
	if err != nil {
		return err
	}

	if err := bs.secretStore.Set("bundle:"+record.RemoteID, deleteToken); err != nil {
		return fmt.Errorf("failed to store delete token: %w", err)
	}

	records = append([]BundleRecord{record}, records...)

	if len(records) > maxBundleRecords {
		for _, old := range records[maxBundleRecords:] {
			bs.secretStore.Delete("bundle:" + old.RemoteID)
		}
		records = records[:maxBundleRecords]
	}

	return bs.writeRecords(records)
}

func (bs *BundleStore) Remove(remoteID string) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	records, err := bs.readRecords()
	if err != nil {
		return err
	}

	newRecords := make([]BundleRecord, 0, len(records))
	for _, r := range records {
		if r.RemoteID != remoteID {
			newRecords = append(newRecords, r)
		}
	}

	if len(newRecords) == len(records) {
		return fmt.Errorf("bundle not found: %s", remoteID)
	}

	bs.secretStore.Delete("bundle:" + remoteID)
	return bs.writeRecords(newRecords)
}

func (bs *BundleStore) GetDeleteToken(remoteID string) (string, error) {
	return bs.secretStore.Get("bundle:" + remoteID)
}

func (bs *BundleStore) UpdateLastAppended(remoteID string, timestamp int64) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	records, err := bs.readRecords()
	if err != nil {
		return err
	}

	for i, r := range records {
		if r.RemoteID == remoteID {
			records[i].LastAppended = timestamp
			return bs.writeRecords(records)
		}
	}
	return fmt.Errorf("bundle not found: %s", remoteID)
}

func (bs *BundleStore) computeExpiry(r *BundleRecord) {
	baseTime := r.UploadedAt
	if r.LastAppended > baseTime {
		baseTime = r.LastAppended
	}
	r.ExpiresAt = baseTime + int64(bundleRetentionDays*24*60*60)
}

func (bs *BundleStore) readRecords() ([]BundleRecord, error) {
	data, err := os.ReadFile(bs.configPath)
	if os.IsNotExist(err) {
		return []BundleRecord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read bundles file: %w", err)
	}

	var records []BundleRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("failed to parse bundles file: %w", err)
	}
	return records, nil
}

func (bs *BundleStore) writeRecords(records []BundleRecord) error {
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal bundles: %w", err)
	}
	if err := os.WriteFile(bs.configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write bundles file: %w", err)
	}
	return nil
}
