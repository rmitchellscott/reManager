package main

import (
	"testing"

	"reManager/internal/storage"
)

func TestUpdateSettingsKeepsFieldsItDoesNotCarry(t *testing.T) {
	storage.SetConfigDir(t.TempDir())
	t.Cleanup(func() { storage.SetConfigDir("") })
	store, err := storage.NewSettingsStore()
	if err != nil {
		t.Fatal(err)
	}
	system := &SystemService{core: &core{settingsStore: store}}

	if err := store.Update(func(s *storage.Settings) { s.Theme = "dark" }); err != nil {
		t.Fatal(err)
	}
	proxyMode := false
	if err := system.UpdateSettings(SettingsPatch{ProxyMode: &proxyMode}); err != nil {
		t.Fatal(err)
	}

	settings, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if settings.ProxyMode || settings.Theme != "dark" {
		t.Errorf("settings = %+v, want proxy mode off and the dark theme kept", settings)
	}
}
