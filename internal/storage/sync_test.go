package storage

import (
	"os"
	"path/filepath"
	"testing"
)

type syncDomain interface {
	ExportSync() (string, error)
	MergeSync(remote string) (bool, error)
}

func deliver(t *testing.T, from, to syncDomain) bool {
	t.Helper()
	payload, err := from.ExportSync()
	if err != nil {
		t.Fatalf("ExportSync: %v", err)
	}
	changed, err := to.MergeSync(payload)
	if err != nil {
		t.Fatalf("MergeSync: %v", err)
	}
	return changed
}

func syncedHooks(dir string) syncHooks {
	return syncHooks{tombstones: &Tombstones{path: filepath.Join(dir, "sync-removed.json")}}
}

func newSyncedFavorites(t *testing.T) *FavoritesStore {
	dir := t.TempDir()
	return &FavoritesStore{syncHooks: syncedHooks(dir), configPath: filepath.Join(dir, "favorites.json")}
}

func newSyncedFollows(t *testing.T) *FollowsStore {
	dir := t.TempDir()
	return &FollowsStore{syncHooks: syncedHooks(dir), configPath: filepath.Join(dir, "follows.json")}
}

func newSyncedSettings(t *testing.T) *SettingsStore {
	dir := t.TempDir()
	return &SettingsStore{syncHooks: syncedHooks(dir), configPath: filepath.Join(dir, "settings.json")}
}

func newSyncedDevices(t *testing.T) (*DeviceStore, *fakeSecretBackend) {
	dir := t.TempDir()
	secrets := &fakeSecretBackend{values: map[string]string{}}
	keys := NewKeyStore(secrets, dir)
	keys.syncHooks = syncedHooks(dir)
	return &DeviceStore{
		syncHooks:   syncedHooks(dir),
		configPath:  filepath.Join(dir, "devices.json"),
		secretStore: secrets,
		keyStore:    keys,
	}, secrets
}

func starredNames(t *testing.T, store *FavoritesStore) map[string]bool {
	t.Helper()
	favorites, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	names := map[string]bool{}
	for _, fav := range favorites.Packages {
		names[fav.Name] = true
	}
	return names
}

func TestStarOnOneHostReachesTheOther(t *testing.T) {
	mac, phone := newSyncedFavorites(t), newSyncedFavorites(t)
	mac.Toggle("xovi")

	if !deliver(t, mac, phone) {
		t.Fatalf("merge reported no change")
	}
	if !starredNames(t, phone)["xovi"] {
		t.Fatalf("phone did not receive the star")
	}
}

func TestUnstarOnOneHostRemovesItOnTheOther(t *testing.T) {
	mac, phone := newSyncedFavorites(t), newSyncedFavorites(t)
	mac.Toggle("xovi")
	deliver(t, mac, phone)

	phone.Toggle("xovi")
	deliver(t, phone, mac)

	if starredNames(t, mac)["xovi"] {
		t.Fatalf("mac kept a star the phone removed")
	}
}

func TestRestarRightAfterUnstarSurvivesAMerge(t *testing.T) {
	mac, phone := newSyncedFavorites(t), newSyncedFavorites(t)
	mac.Toggle("xovi")
	mac.Toggle("xovi")
	mac.Toggle("xovi")

	deliver(t, mac, phone)

	if !starredNames(t, phone)["xovi"] {
		t.Fatalf("a re-star within the same second lost to its own tombstone")
	}
}

func TestMergingWhatAHostAlreadyHasChangesNothing(t *testing.T) {
	mac, phone := newSyncedFavorites(t), newSyncedFavorites(t)
	mac.Toggle("xovi")
	phone.Toggle("appload")
	deliver(t, mac, phone)
	deliver(t, phone, mac)

	if deliver(t, mac, phone) || deliver(t, phone, mac) {
		t.Fatalf("converged hosts still report changes, which would loop the sync")
	}
}

func TestAcknowledgedPackageReachesTheOtherHost(t *testing.T) {
	mac, phone := newSyncedFollows(t), newSyncedFollows(t)
	mac.Toggle("dev")
	deliver(t, mac, phone)

	phone.SetSeen("dev", "pkg", true)
	deliver(t, phone, mac)

	follows, _ := mac.Load()
	if len(follows.Developers) != 1 || len(follows.Developers[0].Seen) != 1 {
		t.Fatalf("mac follows = %+v, want dev with pkg seen", follows.Developers)
	}
}

func TestUnfollowReachesTheOtherHost(t *testing.T) {
	mac, phone := newSyncedFollows(t), newSyncedFollows(t)
	mac.Toggle("dev")
	deliver(t, mac, phone)

	phone.Toggle("dev")
	deliver(t, phone, mac)

	follows, _ := mac.Load()
	if len(follows.Developers) != 0 {
		t.Fatalf("mac still follows %+v", follows.Developers)
	}
}

func TestThemeSyncsButHostLocalSettingsDoNot(t *testing.T) {
	mac, phone := newSyncedSettings(t), newSyncedSettings(t)
	phoneSettings, _ := phone.Load()
	phoneSettings.LastUploadDir = "/phone/uploads"
	phone.Save(phoneSettings)

	macSettings, _ := mac.Load()
	macSettings.Theme = "dark"
	macSettings.LastUploadDir = "/Users/me/Desktop"
	mac.Save(macSettings)

	deliver(t, mac, phone)

	got, _ := phone.Load()
	if got.Theme != "dark" {
		t.Fatalf("theme = %q, want dark", got.Theme)
	}
	if got.LastUploadDir != "/phone/uploads" {
		t.Fatalf("host-local upload folder was overwritten: %q", got.LastUploadDir)
	}
}

func TestSavingOnlyHostLocalSettingsDoesNotOutrankAnotherHostsTheme(t *testing.T) {
	mac, phone := newSyncedSettings(t), newSyncedSettings(t)
	macSettings, _ := mac.Load()
	macSettings.Theme = "dark"
	mac.Save(macSettings)

	phoneSettings, _ := phone.Load()
	phoneSettings.LastDownloadDir = "/phone/downloads"
	phone.Save(phoneSettings)

	deliver(t, phone, mac)

	got, _ := mac.Load()
	if got.Theme != "dark" {
		t.Fatalf("a folder change on the phone reset the mac theme to %q", got.Theme)
	}
}

func TestPreSyncSettingsSurviveANewHostsDefaults(t *testing.T) {
	existing, fresh := newSyncedSettings(t), newSyncedSettings(t)
	preSync := []byte(`{"theme": "dark", "proxyMode": false}`)
	if err := os.WriteFile(existing.configPath, preSync, 0600); err != nil {
		t.Fatal(err)
	}

	deliver(t, fresh, existing)
	deliver(t, existing, fresh)

	for name, store := range map[string]*SettingsStore{"existing": existing, "fresh": fresh} {
		got, _ := store.Load()
		if got.Theme != "dark" || got.ProxyMode {
			t.Fatalf("%s host has theme %q proxyMode %v, want the pre-sync dark theme without proxy", name, got.Theme, got.ProxyMode)
		}
	}
}

func TestSettingsUpdateKeepsAMergeThatLandedAfterTheLastLoad(t *testing.T) {
	mac, phone := newSyncedSettings(t), newSyncedSettings(t)
	phoneSettings, _ := phone.Load()
	phoneSettings.Theme = "dark"
	phone.Save(phoneSettings)
	deliver(t, phone, mac)

	if err := mac.Update(func(settings *Settings) { settings.LastUploadDir = "/Users/me/Desktop" }); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := mac.Load()
	if got.Theme != "dark" || got.LastUploadDir != "/Users/me/Desktop" {
		t.Fatalf("got theme %q upload dir %q, want the merged theme and the new folder", got.Theme, got.LastUploadDir)
	}
}

func TestKeyFilePathStaysOnItsHost(t *testing.T) {
	mac, _ := newSyncedDevices(t)
	phone, _ := newSyncedDevices(t)
	id, _ := mac.Save(SavedDevice{Name: "rmpp", Host: "10.11.99.1", AuthType: "key", KeyPath: "/Users/me/.ssh/id_ed25519"}, "", "")

	deliver(t, mac, phone)

	device, err := phone.Get(id)
	if err != nil {
		t.Fatalf("phone did not receive the device: %v", err)
	}
	if device.KeyPath != "" || device.HasKey() {
		t.Fatalf("phone device = %+v, want no key", device)
	}
}

func TestManagedKeyAssignedOnPhoneLeavesTheMacKeyFileInPlace(t *testing.T) {
	mac, _ := newSyncedDevices(t)
	phone, _ := newSyncedDevices(t)
	id, _ := mac.Save(SavedDevice{Name: "rmpp", Host: "10.11.99.1", AuthType: "key", KeyPath: "/Users/me/.ssh/id_ed25519"}, "", "")
	deliver(t, mac, phone)

	synced, _ := phone.Get(id)
	synced.KeyID = "managed1"
	phone.Save(*synced, "", "")
	deliver(t, phone, mac)

	device, _ := mac.Get(id)
	if device.KeyID != "managed1" {
		t.Fatalf("mac did not receive the managed key: %+v", device)
	}
	if ref := device.KeyRef(); ref.Kind != KeyRefPath || ref.Path != "/Users/me/.ssh/id_ed25519" {
		t.Fatalf("mac resolves %+v, want its own key file", ref)
	}
}

func TestChoosingAKeyFileKeepsTheSyncedManagedKey(t *testing.T) {
	mac, _ := newSyncedDevices(t)
	id, _ := mac.Save(SavedDevice{Name: "rmpp", Host: "10.11.99.1", AuthType: "key", KeyID: "managed1"}, "", "")

	mac.Save(SavedDevice{ID: id, Name: "rmpp", Host: "10.11.99.1", AuthType: "key", KeyPath: "/Users/me/.ssh/id_ed25519"}, "", "")

	device, _ := mac.Get(id)
	if device.KeyID != "managed1" {
		t.Fatalf("editing the key file dropped the managed key other hosts use: %+v", device)
	}
}

func TestLastConnectedStaysOnItsHost(t *testing.T) {
	mac, _ := newSyncedDevices(t)
	phone, _ := newSyncedDevices(t)
	id, _ := mac.Save(SavedDevice{Name: "rmpp", Host: "10.11.99.1", AuthType: "password"}, "pw", "")
	deliver(t, mac, phone)
	phone.UpdateLastConnected(id, 42)

	if deliver(t, phone, mac) {
		t.Fatalf("a connection timestamp alone was treated as a synced change")
	}
}

func TestDeviceDeletedOnOneHostIsDeletedOnTheOther(t *testing.T) {
	mac, _ := newSyncedDevices(t)
	phone, phoneSecrets := newSyncedDevices(t)
	id, _ := mac.Save(SavedDevice{Name: "rmpp", Host: "10.11.99.1", AuthType: "password"}, "pw", "")
	deliver(t, mac, phone)
	phoneSecrets.values["device:"+id] = "pw"

	mac.Delete(id)
	deliver(t, mac, phone)

	if _, err := phone.Get(id); err == nil {
		t.Fatalf("phone kept a device the mac deleted")
	}
	if _, ok := phoneSecrets.values["device:"+id]; ok {
		t.Fatalf("phone kept the deleted device's password")
	}
}

func TestManagedKeyRenameAndRemovalReachTheOtherHost(t *testing.T) {
	macDevices, _ := newSyncedDevices(t)
	phoneDevices, _ := newSyncedDevices(t)
	mac, phone := macDevices.Keys(), phoneDevices.Keys()
	key, _ := phone.Add(ManagedKey{Name: "phone key"}, "private", "")
	deliver(t, phone, mac)

	mac.Rename(key.ID, "renamed")
	deliver(t, mac, phone)
	keys, _ := phone.List()
	if len(keys) != 1 || keys[0].Name != "renamed" {
		t.Fatalf("phone keys = %+v, want renamed", keys)
	}

	mac.Remove(key.ID)
	deliver(t, mac, phone)
	keys, _ = phone.List()
	if len(keys) != 0 {
		t.Fatalf("phone kept a key the mac removed: %+v", keys)
	}
}

func TestDevicesSavedBeforeSyncSurviveAHostMergingItsOwnPayload(t *testing.T) {
	phone, _ := newSyncedDevices(t)
	phone.writeDevices([]SavedDevice{{ID: "old", Name: "rmpp", Host: "10.11.99.1", AuthType: "password"}})

	deliver(t, phone, phone)

	if _, err := phone.Get("old"); err != nil {
		t.Fatalf("a device with no UpdatedAt was deleted by merging its own payload: %v", err)
	}
}

func TestConnectingOnOneHostDoesNotOverrideAnEditMadeOnAnother(t *testing.T) {
	mac, _ := newSyncedDevices(t)
	phone, _ := newSyncedDevices(t)
	id, _ := mac.Save(SavedDevice{Name: "rmpp", Host: "10.11.99.1", AuthType: "password"}, "pw", "")
	deliver(t, mac, phone)

	synced, _ := phone.Get(id)
	synced.AuthType, synced.KeyID = "key", "managed1"
	phone.Save(*synced, "", "")
	mac.UpdateMachineID(id, "machine")
	mac.FillMissingTimezone(id, "America/Denver")
	deliver(t, mac, phone)
	deliver(t, phone, mac)

	device, _ := mac.Get(id)
	if device.AuthType != "key" || device.KeyID != "managed1" {
		t.Fatalf("mac device = %+v, want the phone's key choice", device)
	}
	if device.MachineID != "machine" || device.Timezone != "America/Denver" {
		t.Fatalf("mac lost what it read from the tablet: %+v", device)
	}
}

func TestChosenTimezoneReachesTheOtherHost(t *testing.T) {
	mac, _ := newSyncedDevices(t)
	phone, _ := newSyncedDevices(t)
	id, _ := mac.Save(SavedDevice{Name: "rmpp", Host: "10.11.99.1", AuthType: "password"}, "pw", "")
	mac.FillMissingTimezone(id, "UTC")
	deliver(t, mac, phone)

	phone.UpdateTimezone(id, "America/Denver")
	deliver(t, phone, mac)

	device, _ := mac.Get(id)
	if device.Timezone != "America/Denver" {
		t.Fatalf("mac timezone = %q, want the phone's choice", device.Timezone)
	}
}

func TestTimezoneFilledOnOneHostReachesAHostWithout(t *testing.T) {
	mac, _ := newSyncedDevices(t)
	phone, _ := newSyncedDevices(t)
	id, _ := mac.Save(SavedDevice{Name: "rmpp", Host: "10.11.99.1", AuthType: "password"}, "pw", "")
	deliver(t, mac, phone)

	mac.FillMissingTimezone(id, "America/Denver")
	deliver(t, mac, phone)
	deliver(t, phone, mac)

	for name, host := range map[string]*DeviceStore{"mac": mac, "phone": phone} {
		device, _ := host.Get(id)
		if device.Timezone != "America/Denver" {
			t.Fatalf("%s timezone = %q, want the filled timezone on both", name, device.Timezone)
		}
	}
	if deliver(t, mac, phone) || deliver(t, phone, mac) {
		t.Fatalf("hosts keep trading the filled timezone, which would loop the sync")
	}
}

func newSyncedDeviceInfo(t *testing.T) *DeviceInfoCacheStore {
	dir := t.TempDir()
	return &DeviceInfoCacheStore{syncHooks: syncedHooks(dir), configPath: filepath.Join(dir, "device_info_cache.json")}
}

func TestNewerTabletSnapshotReplacesTheOlderOne(t *testing.T) {
	mac, phone := newSyncedDeviceInfo(t), newSyncedDeviceInfo(t)
	phone.Set("rmpp", CachedDeviceInfo{OSVersion: "3.27", CachedAt: 100})
	mac.Set("rmpp", CachedDeviceInfo{OSVersion: "3.28", CachedAt: 200})

	deliver(t, mac, phone)
	deliver(t, phone, mac)

	for name, host := range map[string]*DeviceInfoCacheStore{"mac": mac, "phone": phone} {
		info, _ := host.Get("rmpp")
		if info == nil || info.OSVersion != "3.28" {
			t.Fatalf("%s snapshot = %+v, want the newer 3.28", name, info)
		}
	}
}

func TestSnapshotTakenRightAfterItsRemovalSurvives(t *testing.T) {
	mac, phone := newSyncedDeviceInfo(t), newSyncedDeviceInfo(t)
	mac.Set("rmpp", CachedDeviceInfo{OSVersion: "old tablet", CachedAt: 100})
	mac.Delete("rmpp")
	mac.Set("rmpp", CachedDeviceInfo{OSVersion: "replacement tablet", CachedAt: 100})

	deliver(t, mac, phone)

	info, _ := phone.Get("rmpp")
	if info == nil || info.OSVersion != "replacement tablet" {
		t.Fatalf("phone snapshot = %+v, want the replacement", info)
	}
}

func TestClearingTheCacheClearsItOnTheOtherHost(t *testing.T) {
	mac, phone := newSyncedDeviceInfo(t), newSyncedDeviceInfo(t)
	mac.Set("rmpp", CachedDeviceInfo{OSVersion: "3.28", CachedAt: 100})
	deliver(t, mac, phone)

	mac.Clear()
	deliver(t, mac, phone)

	if count, _ := phone.Count(); count != 0 {
		t.Fatalf("phone kept %d snapshots after the mac cleared its cache", count)
	}
}
