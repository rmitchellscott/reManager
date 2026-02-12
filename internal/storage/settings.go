package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type TabVisibility map[string]bool

type Settings struct {
	TabVisibility              TabVisibility `json:"tabVisibility"`
	ProxyMode                  bool          `json:"proxyMode"`
	SuppressSystemFileWarnings bool          `json:"suppressSystemFileWarnings"`
	PreventSleep               bool          `json:"preventSleep"`
	Theme                      string        `json:"theme"`
	TerminalTheme              string        `json:"terminalTheme"`
	EditorTheme                string        `json:"editorTheme"`
	CheckForUpdates            bool          `json:"checkForUpdates"`
}

type SettingsStore struct {
	configPath string
	mu         sync.RWMutex
}

func NewSettingsStore() (*SettingsStore, error) {
	configDir, err := getConfigDir()
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

	data, err := os.ReadFile(s.configPath)
	if os.IsNotExist(err) {
		return s.defaultSettings(), nil
	}
	if err != nil {
		return nil, err
	}

	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return s.defaultSettings(), nil
	}

	// Check if fields were present in JSON (default to true/match if missing)
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

	// Merge with defaults so new tabs get their default values
	defaults := s.defaultSettings()
	if settings.TabVisibility == nil {
		settings.TabVisibility = defaults.TabVisibility
	} else {
		for key, val := range defaults.TabVisibility {
			if _, exists := settings.TabVisibility[key]; !exists {
				settings.TabVisibility[key] = val
			}
		}
	}

	return &settings, nil
}

func (s *SettingsStore) Save(settings *Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.configPath, data, 0600)
}

func (s *SettingsStore) defaultSettings() *Settings {
	return &Settings{
		TabVisibility: TabVisibility{
			"mods":        true,
			"maintenance": true,
			"utilities":   true,
		},
		ProxyMode:                  true,
		SuppressSystemFileWarnings: false,
		PreventSleep:               true,
		Theme:                      "system",
		TerminalTheme:              "match",
		EditorTheme:                "match",
		CheckForUpdates:            true,
	}
}
