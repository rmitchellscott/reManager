package storage

import (
	"fmt"
	"testing"
)

type fakeSecretBackend struct {
	values map[string]string
}

func newFakeKeyStore() (*KeyStore, *fakeSecretBackend) {
	backend := &fakeSecretBackend{values: map[string]string{}}
	return &KeyStore{secretStore: backend}, backend
}

func TestKeyIDIsSharedByDevicesUsingTheSameKey(t *testing.T) {
	first := SavedDevice{ID: "a", AuthType: "key", KeyPath: "/home/u/.ssh/id_ed25519"}
	second := SavedDevice{ID: "b", AuthType: "key", KeyPath: "/home/u/.ssh/id_ed25519"}

	if first.KeyRef().KeyID() != second.KeyRef().KeyID() {
		t.Fatalf("same key path produced different key IDs")
	}

	other := SavedDevice{ID: "c", AuthType: "key", KeyPath: "/home/u/.ssh/id_rsa"}
	if first.KeyRef().KeyID() == other.KeyRef().KeyID() {
		t.Fatalf("different key paths produced the same key ID")
	}
}

func TestKeyRefPrefersThisHostsKeyFileOverTheSyncedManagedKey(t *testing.T) {
	device := SavedDevice{ID: "a", KeyPath: "/home/u/.ssh/id_ed25519", KeyID: "managed1"}

	ref := device.KeyRef()
	if ref.Kind != KeyRefPath || ref.Path != "/home/u/.ssh/id_ed25519" {
		t.Fatalf("expected path ref, got %+v", ref)
	}
}

func TestGetPassphraseMigratesLegacyDeviceScopedSecret(t *testing.T) {
	keyStore, backend := newFakeKeyStore()
	device := SavedDevice{ID: "dev1", AuthType: "key", KeyPath: "/home/u/.ssh/id_ed25519"}
	ref := device.KeyRef()
	backend.values[legacyPassphraseSecretName(device.ID)] = "hunter2"

	value, err := keyStore.GetPassphrase(ref, device.ID)
	if err != nil {
		t.Fatalf("GetPassphrase: %v", err)
	}
	if value != "hunter2" {
		t.Fatalf("got %q, want hunter2", value)
	}

	if backend.values[passphraseSecretName(ref)] != "hunter2" {
		t.Fatalf("passphrase was not rewritten to its key-scoped name")
	}
	if _, ok := backend.values[legacyPassphraseSecretName(device.ID)]; ok {
		t.Fatalf("legacy device-scoped passphrase was left behind")
	}
}

func TestMigratedPassphraseIsFoundByASecondDeviceOnTheSameKey(t *testing.T) {
	keyStore, backend := newFakeKeyStore()
	first := SavedDevice{ID: "dev1", AuthType: "key", KeyPath: "/home/u/.ssh/id_ed25519"}
	second := SavedDevice{ID: "dev2", AuthType: "key", KeyPath: "/home/u/.ssh/id_ed25519"}
	backend.values[legacyPassphraseSecretName(first.ID)] = "hunter2"

	if _, err := keyStore.GetPassphrase(first.KeyRef(), first.ID); err != nil {
		t.Fatalf("GetPassphrase: %v", err)
	}

	value, err := keyStore.GetPassphrase(second.KeyRef(), second.ID)
	if err != nil {
		t.Fatalf("second device GetPassphrase: %v", err)
	}
	if value != "hunter2" {
		t.Fatalf("got %q, want hunter2", value)
	}
}

func TestGetPassphraseReportsMissingSecret(t *testing.T) {
	keyStore, _ := newFakeKeyStore()
	device := SavedDevice{ID: "dev1", AuthType: "key", KeyPath: "/home/u/.ssh/id_ed25519"}

	if _, err := keyStore.GetPassphrase(device.KeyRef(), device.ID); err == nil {
		t.Fatalf("expected an error for a key with no stored passphrase")
	}
}

func (f *fakeSecretBackend) Get(key string) (string, error) {
	value, ok := f.values[key]
	if !ok {
		return "", fmt.Errorf("secret not found: %s", key)
	}
	return value, nil
}

func (f *fakeSecretBackend) Set(key, value string) error {
	f.values[key] = value
	return nil
}

func (f *fakeSecretBackend) Delete(key string) error {
	delete(f.values, key)
	return nil
}
