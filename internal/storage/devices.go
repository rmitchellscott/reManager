package storage

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
)

const serviceName = "reManager"

type SavedDevice struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Host          string `json:"host"`
	AuthType      string `json:"authType"`
	KeyPath       string `json:"keyPath,omitempty"`
	KeyID         string `json:"keyId,omitempty"`
	MachineID     string `json:"machineId,omitempty"`
	LastConnected int64  `json:"lastConnected,omitempty"`
	Timezone      string `json:"timezone,omitempty"`
	UpdatedAt     int64  `json:"updatedAt,omitempty"`
}

// KeyRef prefers KeyPath: a host-local override of the managed key the device record carries between hosts.
func (d SavedDevice) KeyRef() KeyRef {
	if d.KeyPath != "" {
		return PathKeyRef(d.KeyPath)
	}
	if d.KeyID != "" {
		return ManagedKeyRef(d.KeyID)
	}
	return PathKeyRef("")
}

// A device synced from another host that used a key file arrives with neither reference.
func (d SavedDevice) HasKey() bool {
	return d.KeyPath != "" || d.KeyID != ""
}

type DeviceStore struct {
	syncHooks
	configPath  string
	secretStore secretBackend
	keyStore    *KeyStore
	mu          sync.RWMutex
}

func NewDeviceStore() (*DeviceStore, error) {
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

	return &DeviceStore{
		configPath:  filepath.Join(configDir, "devices.json"),
		secretStore: secretStore,
		keyStore:    NewKeyStore(secretStore, configDir),
	}, nil
}

// Android reports no usable home directory; its private files directory comes from the Android context, not the environment.
var configDir string

// Must run before any store is constructed.
func SetConfigDir(dir string) { configDir = dir }

func GetConfigDir() (string, error) {
	if configDir != "" {
		return configDir, nil
	}
	switch runtime.GOOS {
	// iOS reports GOOS "ios" and permits writes only under Documents, Library and tmp, so the XDG default below fails.
	case "darwin", "ios":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "reManager"), nil
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", fmt.Errorf("APPDATA not set")
		}
		return filepath.Join(appData, "reManager"), nil
	default:
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			configHome = filepath.Join(home, ".config")
		}
		return filepath.Join(configHome, "reManager"), nil
	}
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (ds *DeviceStore) GetAll() ([]SavedDevice, error) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	devices, err := ds.getAllUnsafe()
	if err != nil {
		return nil, err
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].LastConnected > devices[j].LastConnected
	})

	return devices, nil
}

func (ds *DeviceStore) Get(id string) (*SavedDevice, error) {
	devices, err := ds.GetAll()
	if err != nil {
		return nil, err
	}

	for _, d := range devices {
		if d.ID == id {
			return &d, nil
		}
	}
	return nil, fmt.Errorf("device not found: %s", id)
}

func (ds *DeviceStore) Save(device SavedDevice, password string, keyPassphrase string) (string, error) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	devices, err := ds.getAllUnsafe()
	if err != nil {
		return "", err
	}

	if device.ID == "" {
		device.ID = generateID()
	}

	if device.AuthType == "password" && password != "" {
		if err := ds.secretStore.Set("device:"+device.ID, password); err != nil {
			return "", fmt.Errorf("failed to store password: %w", err)
		}
	}

	if device.AuthType == "key" && keyPassphrase != "" {
		if err := ds.keyStore.SetPassphrase(device.KeyRef(), keyPassphrase); err != nil {
			return "", fmt.Errorf("failed to store key passphrase: %w", err)
		}
	}

	device.UpdatedAt = nowMillis()
	found := false
	for i, d := range devices {
		if d.ID == device.ID {
			device.UpdatedAt = nextStamp(d.UpdatedAt)
			devices[i] = keepUnsubmittedFields(device, d)
			found = true
			break
		}
	}
	if !found {
		devices = append(devices, device)
	}

	if err := ds.writeDevices(devices); err != nil {
		return "", err
	}
	ds.changed()

	return device.ID, nil
}

// Carries over fields the edit form never sends; a key file chosen here keeps the managed key another host assigned, since the path only overrides it locally.
func keepUnsubmittedFields(submitted, existing SavedDevice) SavedDevice {
	if submitted.MachineID == "" {
		submitted.MachineID = existing.MachineID
	}
	if submitted.Timezone == "" {
		submitted.Timezone = existing.Timezone
	}
	if submitted.KeyID == "" && submitted.KeyPath != "" {
		submitted.KeyID = existing.KeyID
	}
	return submitted
}

func (ds *DeviceStore) Delete(id string) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	devices, err := ds.getAllUnsafe()
	if err != nil {
		return err
	}

	var authType string
	var removedStamp int64
	newDevices := make([]SavedDevice, 0, len(devices))
	for _, d := range devices {
		if d.ID != id {
			newDevices = append(newDevices, d)
		} else {
			authType = d.AuthType
			removedStamp = d.UpdatedAt
		}
	}

	if len(newDevices) == len(devices) {
		return fmt.Errorf("device not found: %s", id)
	}

	if err := ds.writeDevices(newDevices); err != nil {
		return err
	}
	ds.recordRemoval(DevicesSyncKey, id, removedStamp)
	ds.changed()

	// A key passphrase is keyed by the key, not the device, so another saved device may still use it; it is removed with the key itself.
	if authType == "password" {
		ds.secretStore.Delete("device:" + id)
	}

	return nil
}

func (ds *DeviceStore) GetPassword(id string) (string, error) {
	return ds.secretStore.Get("device:" + id)
}

func (ds *DeviceStore) GetKeyPassphrase(ref KeyRef, deviceID string) (string, error) {
	return ds.keyStore.GetPassphrase(ref, deviceID)
}

func (ds *DeviceStore) Keys() *KeyStore {
	return ds.keyStore
}

func (ds *DeviceStore) getAllUnsafe() ([]SavedDevice, error) {
	data, err := os.ReadFile(ds.configPath)
	if os.IsNotExist(err) {
		return []SavedDevice{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read devices file: %w", err)
	}

	var devices []SavedDevice
	if err := json.Unmarshal(data, &devices); err != nil {
		if setAsideErr := setAsideUnreadable(ds.configPath, err); setAsideErr != nil {
			return nil, setAsideErr
		}
		return []SavedDevice{}, nil
	}
	return devices, nil
}

func (ds *DeviceStore) writeDevices(devices []SavedDevice) error {
	data, err := json.MarshalIndent(devices, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal devices: %w", err)
	}

	if err := writeFileAtomic(ds.configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write devices file: %w", err)
	}
	return nil
}

func (ds *DeviceStore) UpdateLastConnected(id string, timestamp int64) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	devices, err := ds.getAllUnsafe()
	if err != nil {
		return err
	}

	for i, d := range devices {
		if d.ID == id {
			devices[i].LastConnected = timestamp
			return ds.writeDevices(devices)
		}
	}
	return nil
}

func (ds *DeviceStore) UpdateName(id string, name string) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	devices, err := ds.getAllUnsafe()
	if err != nil {
		return err
	}

	for i, d := range devices {
		if d.ID == id {
			if d.Name == name {
				return nil
			}
			devices[i].Name = name
			devices[i].UpdatedAt = nextStamp(d.UpdatedAt)
			if err := ds.writeDevices(devices); err != nil {
				return err
			}
			ds.changed()
			return nil
		}
	}
	return fmt.Errorf("device not found: %s", id)
}

func (ds *DeviceStore) UpdateMachineID(id string, machineID string) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	devices, err := ds.getAllUnsafe()
	if err != nil {
		return err
	}

	for i, d := range devices {
		if d.ID == id {
			devices[i].MachineID = machineID
			return ds.writeDevices(devices)
		}
	}
	return fmt.Errorf("device not found: %s", id)
}

func (ds *DeviceStore) UpdateTimezone(id string, timezone string) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	devices, err := ds.getAllUnsafe()
	if err != nil {
		return err
	}

	for i, d := range devices {
		if d.ID == id {
			if d.Timezone == timezone {
				return nil
			}
			devices[i].Timezone = timezone
			devices[i].UpdatedAt = nextStamp(d.UpdatedAt)
			if err := ds.writeDevices(devices); err != nil {
				return err
			}
			ds.changed()
			return nil
		}
	}
	return fmt.Errorf("device not found: %s", id)
}

// Adopts the tablet's timezone only when none was chosen; not an edit, so it bumps no stamp and cannot outrank another host's change.
func (ds *DeviceStore) FillMissingTimezone(id string, timezone string) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	devices, err := ds.getAllUnsafe()
	if err != nil {
		return err
	}

	for i, d := range devices {
		if d.ID == id {
			if d.Timezone != "" {
				return nil
			}
			devices[i].Timezone = timezone
			if err := ds.writeDevices(devices); err != nil {
				return err
			}
			ds.changed()
			return nil
		}
	}
	return fmt.Errorf("device not found: %s", id)
}

// Drops host-local KeyPath and LastConnected, and MachineID too: it is re-read on every connect, and syncing it would let a connect overwrite another host's rename or key choice.
func deviceEntry(device SavedDevice) syncedEntry[SavedDevice] {
	device.KeyPath = ""
	device.LastConnected = 0
	device.MachineID = ""
	return syncedEntry[SavedDevice]{ID: device.ID, Stamp: device.UpdatedAt, Value: device}
}

func (ds *DeviceStore) SyncKey() string { return string(DevicesSyncKey) }

func (ds *DeviceStore) ExportSync() (string, error) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	devices, err := ds.getAllUnsafe()
	if err != nil {
		return "", err
	}
	var entries []syncedEntry[SavedDevice]
	for _, device := range devices {
		entries = append(entries, deviceEntry(device))
	}
	return encodeCollection(entries, ds.removed(DevicesSyncKey))
}

// Both sides fill each other's timezone gaps before merging, so a tablet-derived timezone reaches a host that has none without outranking anything on its stamp.
func (ds *DeviceStore) MergeSync(remote string) (bool, error) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	devices, err := ds.getAllUnsafe()
	if err != nil {
		return false, err
	}
	remoteCollection, err := decodeCollection[SavedDevice](remote)
	if err != nil {
		return false, err
	}

	timezones := map[string]string{}
	for _, device := range append(devices, remoteCollection.Entries...) {
		timezones[device.ID] = max(timezones[device.ID], device.Timezone)
	}
	withTimezone := func(device SavedDevice) SavedDevice {
		if device.Timezone == "" {
			device.Timezone = timezones[device.ID]
		}
		return device
	}

	hostLocal := map[string]SavedDevice{}
	timezoneFilled := false
	var local []syncedEntry[SavedDevice]
	for _, device := range devices {
		hostLocal[device.ID] = device
		filled := withTimezone(device)
		timezoneFilled = timezoneFilled || filled.Timezone != device.Timezone
		local = append(local, deviceEntry(filled))
	}

	merge, err := mergeInto(&ds.syncHooks, DevicesSyncKey, local, remote, func(device SavedDevice) syncedEntry[SavedDevice] {
		return deviceEntry(withTimezone(device))
	})
	if err != nil || !(merge.changed || timezoneFilled) {
		return false, err
	}
	merged := make([]SavedDevice, 0, len(merge.entries))
	for _, entry := range merge.entries {
		device := entry.Value
		if existing, wasHere := hostLocal[device.ID]; wasHere {
			device.KeyPath = existing.KeyPath
			device.LastConnected = existing.LastConnected
			device.MachineID = existing.MachineID
		}
		merged = append(merged, device)
	}
	if err := ds.writeDevices(merged); err != nil {
		return false, err
	}
	for id := range merge.removed {
		if _, wasHere := hostLocal[id]; wasHere {
			ds.secretStore.Delete("device:" + id)
		}
	}
	return true, ds.replaceRemoved(DevicesSyncKey, merge.removed)
}
