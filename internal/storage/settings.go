package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Settings struct {
	ProxyMode                  bool   `json:"proxyMode"`
	SuppressSystemFileWarnings bool   `json:"suppressSystemFileWarnings"`
	PreventSleep               bool   `json:"preventSleep"`
	Theme                      string `json:"theme"`
	TerminalTheme              string `json:"terminalTheme"`
	EditorTheme                string `json:"editorTheme"`
	CheckForUpdates            bool   `json:"checkForUpdates"`
	SSHAgentSocketPath         string `json:"sshAgentSocketPath,omitempty"`
	LastUploadDir              string `json:"lastUploadDir,omitempty"`
	LastDownloadDir            string `json:"lastDownloadDir,omitempty"`
	GuideSeenVersion           string `json:"guideSeenVersion,omitempty"`
	SyncedAt                   int64  `json:"syncedAt,omitempty"`
}

// The Settings fields that follow the user between hosts; the rest is host-local.
type syncedSettings struct {
	ProxyMode                  bool   `json:"proxyMode"`
	SuppressSystemFileWarnings bool   `json:"suppressSystemFileWarnings"`
	Theme                      string `json:"theme"`
	TerminalTheme              string `json:"terminalTheme"`
	EditorTheme                string `json:"editorTheme"`
}

type settingsPayload struct {
	Settings  syncedSettings `json:"settings"`
	UpdatedAt int64          `json:"updatedAt"`
}

func (s *Settings) synced() syncedSettings {
	return syncedSettings{
		ProxyMode:                  s.ProxyMode,
		SuppressSystemFileWarnings: s.SuppressSystemFileWarnings,
		Theme:                      s.Theme,
		TerminalTheme:              s.TerminalTheme,
		EditorTheme:                s.EditorTheme,
	}
}

func (s *Settings) applySynced(synced syncedSettings) {
	s.ProxyMode = synced.ProxyMode
	s.SuppressSystemFileWarnings = synced.SuppressSystemFileWarnings
	s.Theme = synced.Theme
	s.TerminalTheme = synced.TerminalTheme
	s.EditorTheme = synced.EditorTheme
}

type SettingsStore struct {
	syncHooks
	configPath string
	mu         sync.RWMutex
}

func NewSettingsStore() (*SettingsStore, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, err
	}

	return &SettingsStore{
		configPath: filepath.Join(configDir, "settings.json"),
	}, nil
}

func (s *SettingsStore) Load() (*Settings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.load()
}

func (s *SettingsStore) load() (*Settings, error) {
	data, err := os.ReadFile(s.configPath)
	if os.IsNotExist(err) {
		return s.defaultSettings(), nil
	}
	if err != nil {
		return nil, err
	}

	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		if setAsideErr := setAsideUnreadable(s.configPath, err); setAsideErr != nil {
			return nil, setAsideErr
		}
		return s.defaultSettings(), nil
	}

	// A field missing from an older settings file takes its default, not the zero value.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err == nil {
		if _, exists := raw["proxyMode"]; !exists {
			settings.ProxyMode = true
		}
		if _, exists := raw["theme"]; !exists {
			settings.Theme = "system"
		}
		if _, exists := raw["terminalTheme"]; !exists {
			settings.TerminalTheme = "match"
		}
		if _, exists := raw["editorTheme"]; !exists {
			settings.EditorTheme = "match"
		}
		if _, exists := raw["checkForUpdates"]; !exists {
			settings.CheckForUpdates = true
		}
	}

	return &settings, nil
}

// Stamps SyncedAt only when a synced field changed, so saving a host-local field never outranks another host's synced change.
func (s *SettingsStore) Save(settings *Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	previous, err := s.load()
	if err != nil {
		return err
	}
	return s.saveOver(previous, settings)
}

// Applies change under the store lock, so a remote merge landing between read and save is not reverted.
func (s *SettingsStore) Update(change func(*Settings)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	previous, err := s.load()
	if err != nil {
		return err
	}
	updated := *previous
	change(&updated)
	return s.saveOver(previous, &updated)
}

func (s *SettingsStore) saveOver(previous, settings *Settings) error {
	syncedChanged := previous.synced() != settings.synced()
	settings.SyncedAt = previous.SyncedAt
	if syncedChanged {
		settings.SyncedAt = nextStamp(previous.SyncedAt)
	}

	if err := s.write(settings); err != nil {
		return err
	}
	if syncedChanged {
		s.changed()
	}
	return nil
}

func (s *SettingsStore) write(settings *Settings) error {
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.configPath, data, 0600)
}

func (s *SettingsStore) SyncKey() string { return string(SettingsSyncKey) }

func (s *SettingsStore) ExportSync() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	settings, err := s.load()
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(settingsPayload{Settings: settings.synced(), UpdatedAt: settings.SyncedAt})
	return string(data), err
}

func (s *SettingsStore) MergeSync(remote string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var payload settingsPayload
	if err := json.Unmarshal([]byte(remote), &payload); err != nil {
		return false, err
	}
	settings, err := s.load()
	if err != nil {
		return false, err
	}
	local := settingsPayload{settings.synced(), settings.SyncedAt}
	if payload.Settings == settings.synced() || !remoteSettingsWin(payload, local, s.defaultSettings().synced()) {
		return false, nil
	}
	settings.applySynced(payload.Settings)
	settings.SyncedAt = payload.UpdatedAt
	return true, s.write(settings)
}

func (s *SettingsStore) defaultSettings() *Settings {
	return &Settings{
		ProxyMode:                  true,
		SuppressSystemFileWarnings: false,
		PreventSleep:               true,
		Theme:                      "system",
		TerminalTheme:              "match",
		EditorTheme:                "match",
		CheckForUpdates:            true,
	}
}

// At stamp 0 (pre-sync settings and fresh defaults carry none) the side differing from the defaults holds the user's choice and wins; other ties compare encoded settings, as mergeCollections does.
func remoteSettingsWin(remote, local settingsPayload, defaults syncedSettings) bool {
	if remote.UpdatedAt != local.UpdatedAt {
		return remote.UpdatedAt > local.UpdatedAt
	}
	if remote.UpdatedAt == 0 {
		remoteIsDefault := remote.Settings == defaults
		localIsDefault := local.Settings == defaults
		if remoteIsDefault != localIsDefault {
			return localIsDefault
		}
	}
	remoteJSON, _ := json.Marshal(remote.Settings)
	localJSON, _ := json.Marshal(local.Settings)
	return string(remoteJSON) > string(localJSON)
}
