package storage

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteFileAtomicReplacesContentsAndLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "store.json")
	if err := os.WriteFile(path, []byte("old contents that are longer"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := writeFileAtomic(path, []byte("new"), 0600); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("contents = %q, want %q", data, "new")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("directory holds %d entries, want only the store file", len(entries))
	}
}

func TestWriteFileAtomicKeepsExistingPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows has no unix permission bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "store.json")
	if err := os.WriteFile(path, []byte("{}"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}

	if err := writeFileAtomic(path, []byte("{}"), 0600); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0640 {
		t.Fatalf("mode = %v, want 0640", info.Mode().Perm())
	}
}

func TestWriteFileAtomicUsesPermForNewFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows has no unix permission bits")
	}
	path := filepath.Join(t.TempDir(), "store.json")

	if err := writeFileAtomic(path, []byte("{}"), 0600); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func unreadableCopies(t *testing.T, path string) []string {
	t.Helper()
	matches, err := filepath.Glob(path + ".unreadable-*")
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

func TestUnparseableSettingsAreSetAsideBeforeTheNextSave(t *testing.T) {
	store := newSyncedSettings(t)
	truncated := []byte(`{"theme": "dark", "proxyMo`)
	if err := os.WriteFile(store.configPath, truncated, 0600); err != nil {
		t.Fatal(err)
	}

	settings, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	settings.Theme = "light"
	if err := store.Save(settings); err != nil {
		t.Fatalf("Save: %v", err)
	}

	copies := unreadableCopies(t, store.configPath)
	if len(copies) != 1 {
		t.Fatalf("found %d set-aside copies, want 1", len(copies))
	}
	preserved, err := os.ReadFile(copies[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(preserved) != string(truncated) {
		t.Fatalf("set-aside copy = %q, want the original bytes", preserved)
	}
}

func TestUnparseableDevicesAreSetAsideAndSavingStillWorks(t *testing.T) {
	store, _ := newSyncedDevices(t)
	truncated := []byte(`[{"id": "a1", "name": "rmpp", "ho`)
	if err := os.WriteFile(store.configPath, truncated, 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Save(SavedDevice{Name: "rmpp", Host: "10.11.99.1", AuthType: "password"}, "secret", ""); err != nil {
		t.Fatalf("Save: %v", err)
	}

	devices, err := store.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(devices) != 1 || devices[0].Host != "10.11.99.1" {
		t.Fatalf("devices = %+v, want the one just saved", devices)
	}
	copies := unreadableCopies(t, store.configPath)
	if len(copies) != 1 {
		t.Fatalf("found %d set-aside copies, want 1", len(copies))
	}
	preserved, err := os.ReadFile(copies[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(preserved) != string(truncated) {
		t.Fatalf("set-aside copy = %q, want the original bytes", preserved)
	}
}

func TestUnparseableFavoritesAreSetAside(t *testing.T) {
	store := newSyncedFavorites(t)
	if err := os.WriteFile(store.configPath, []byte(`{"packages": [`), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Toggle("xovi"); err != nil {
		t.Fatalf("Toggle: %v", err)
	}

	if len(unreadableCopies(t, store.configPath)) != 1 {
		t.Fatal("the unparseable favorites file was not set aside")
	}
}

func TestUnparseableTombstonesAreSetAside(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sync-removed.json")
	if err := os.WriteFile(path, []byte(`{"favorites.v1": {"xo`), 0600); err != nil {
		t.Fatal(err)
	}
	tombstones := &Tombstones{path: path}

	if err := tombstones.Record(FavoritesSyncKey, "xovi", 1); err != nil {
		t.Fatalf("Record: %v", err)
	}

	if len(unreadableCopies(t, path)) != 1 {
		t.Fatal("the unparseable tombstones file was not set aside")
	}
}

func TestDeviceInfoCacheWritesFailOnUnparseableFile(t *testing.T) {
	dir := t.TempDir()
	store := &DeviceInfoCacheStore{configPath: filepath.Join(dir, "device_info_cache.json")}
	truncated := []byte(`{"a": {"machineType": "rm`)
	if err := os.WriteFile(store.configPath, truncated, 0600); err != nil {
		t.Fatal(err)
	}

	if err := store.Set("b", CachedDeviceInfo{MachineType: "ferrari"}); err == nil {
		t.Fatal("Set succeeded over an unparseable cache")
	}
	if err := store.Delete("a"); err == nil {
		t.Fatal("Delete succeeded over an unparseable cache")
	}
	if err := store.Clear(); err == nil {
		t.Fatal("Clear succeeded over an unparseable cache")
	}

	data, err := os.ReadFile(store.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(truncated) {
		t.Fatalf("cache file was rewritten: %q", data)
	}
}
