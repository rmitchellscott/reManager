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

	"github.com/zalando/go-keyring"
)

const serviceName = "reManager"

type SavedDevice struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Host          string `json:"host"`
	AuthType      string `json:"authType"`
	KeyPath       string `json:"keyPath,omitempty"`
	LastConnected int64  `json:"lastConnected,omitempty"`
	Timezone      string `json:"timezone,omitempty"`
}

type DeviceStore struct {
	configPath string
	mu         sync.RWMutex
}

func NewDeviceStore() (*DeviceStore, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}

	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	return &DeviceStore{
		configPath: filepath.Join(configDir, "devices.json"),
	}, nil
}

func getConfigDir() (string, error) {
	switch runtime.GOOS {
	case "darwin":
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

	data, err := os.ReadFile(ds.configPath)
	if os.IsNotExist(err) {
		return []SavedDevice{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read devices file: %w", err)
	}

	var devices []SavedDevice
	if err := json.Unmarshal(data, &devices); err != nil {
		return nil, fmt.Errorf("failed to parse devices file: %w", err)
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

	found := false
	for i, d := range devices {
		if d.ID == device.ID {
			devices[i] = device
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

	if device.AuthType == "password" && password != "" {
		if err := keyring.Set(serviceName, "device:"+device.ID, password); err != nil {
			return device.ID, fmt.Errorf("saved device but failed to store password in keyring: %w", err)
		}
	}

	if device.AuthType == "key" && keyPassphrase != "" {
		if err := keyring.Set(serviceName, "device:"+device.ID+":passphrase", keyPassphrase); err != nil {
			return device.ID, fmt.Errorf("saved device but failed to store key passphrase in keyring: %w", err)
		}
	}

	return device.ID, nil
}

func (ds *DeviceStore) Delete(id string) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	devices, err := ds.getAllUnsafe()
	if err != nil {
		return err
	}

	var authType string
	newDevices := make([]SavedDevice, 0, len(devices))
	for _, d := range devices {
		if d.ID != id {
			newDevices = append(newDevices, d)
		} else {
			authType = d.AuthType
		}
	}

	if len(newDevices) == len(devices) {
		return fmt.Errorf("device not found: %s", id)
	}

	if err := ds.writeDevices(newDevices); err != nil {
		return err
	}

	if authType == "password" {
		keyring.Delete(serviceName, "device:"+id)
	} else if authType == "key" {
		keyring.Delete(serviceName, "device:"+id+":passphrase")
	}

	return nil
}

func (ds *DeviceStore) GetPassword(id string) (string, error) {
	return keyring.Get(serviceName, "device:"+id)
}

func (ds *DeviceStore) GetKeyPassphrase(id string) (string, error) {
	return keyring.Get(serviceName, "device:"+id+":passphrase")
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
		return nil, fmt.Errorf("failed to parse devices file: %w", err)
	}
	return devices, nil
}

func (ds *DeviceStore) writeDevices(devices []SavedDevice) error {
	data, err := json.MarshalIndent(devices, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal devices: %w", err)
	}

	if err := os.WriteFile(ds.configPath, data, 0600); err != nil {
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
			devices[i].Name = name
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
			devices[i].Timezone = timezone
			return ds.writeDevices(devices)
		}
	}
	return fmt.Errorf("device not found: %s", id)
}
