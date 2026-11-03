package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type KeyRefKind string

const (
	// Desktop platforms use the user's own ~/.ssh rather than copying keys into app storage.
	KeyRefPath KeyRefKind = "path"
	// Holds the private key bytes in the secret store, for platforms with no filesystem the user can put a key on.
	KeyRefManaged KeyRefKind = "managed"
)

type KeyRef struct {
	Kind KeyRefKind `json:"kind"`
	Path string     `json:"path,omitempty"`
	ID   string     `json:"id,omitempty"`
}

func PathKeyRef(path string) KeyRef {
	return KeyRef{Kind: KeyRefPath, Path: path}
}

func ManagedKeyRef(id string) KeyRef {
	return KeyRef{Kind: KeyRefManaged, ID: id}
}

// Stable for the same key across every device that uses it, so a passphrase entered once is not re-prompted per device.
func (r KeyRef) KeyID() string {
	if r.Kind == KeyRefManaged {
		return r.ID
	}
	if r.Path == "" {
		return ""
	}
	abs, err := filepath.Abs(r.Path)
	if err != nil {
		abs = r.Path
	}
	sum := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(sum[:])[:16]
}

// Metadata only; the private bytes live in the secret store, never in this struct.
type ManagedKey struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	PublicKey   string `json:"publicKey,omitempty"`
	Encrypted   bool   `json:"encrypted,omitempty"`
	CreatedAt   int64  `json:"createdAt,omitempty"`
	UpdatedAt   int64  `json:"updatedAt,omitempty"`
}

type secretBackend interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
}

type KeyStore struct {
	syncHooks
	secretStore secretBackend
	configPath  string
	mu          sync.RWMutex
}

func NewKeyStore(secretStore secretBackend, configDir string) *KeyStore {
	return &KeyStore{
		secretStore: secretStore,
		configPath:  filepath.Join(configDir, "keys.json"),
	}
}

func passphraseSecretName(ref KeyRef) string {
	return "key:" + ref.KeyID() + ":passphrase"
}

func legacyPassphraseSecretName(deviceID string) string {
	return "device:" + deviceID + ":passphrase"
}

// Migrates a v1 device-scoped passphrase to its key-scoped name lazily on first use; a startup sweep would make macOS prompt once per saved device before the user connects to any.
func (ks *KeyStore) GetPassphrase(ref KeyRef, deviceID string) (string, error) {
	value, err := ks.secretStore.Get(passphraseSecretName(ref))
	if err == nil {
		return value, nil
	}
	if deviceID == "" {
		return "", err
	}

	value, legacyErr := ks.secretStore.Get(legacyPassphraseSecretName(deviceID))
	if legacyErr != nil {
		return "", err
	}
	if setErr := ks.secretStore.Set(passphraseSecretName(ref), value); setErr == nil {
		ks.secretStore.Delete(legacyPassphraseSecretName(deviceID))
	}
	return value, nil
}

func (ks *KeyStore) SetPassphrase(ref KeyRef, passphrase string) error {
	return ks.secretStore.Set(passphraseSecretName(ref), passphrase)
}

func (ks *KeyStore) DeletePassphrase(ref KeyRef) error {
	return ks.secretStore.Delete(passphraseSecretName(ref))
}

func (ks *KeyStore) GetPrivateKey(ref KeyRef) (string, error) {
	return ks.secretStore.Get("key:" + ref.KeyID() + ":private")
}

func (ks *KeyStore) SetPrivateKey(ref KeyRef, keyData string) error {
	return ks.secretStore.Set("key:"+ref.KeyID()+":private", keyData)
}

func (ks *KeyStore) DeletePrivateKey(ref KeyRef) error {
	return ks.secretStore.Delete("key:" + ref.KeyID() + ":private")
}

func (ks *KeyStore) List() ([]ManagedKey, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return ks.listUnsafe()
}

func (ks *KeyStore) listUnsafe() ([]ManagedKey, error) {
	data, err := os.ReadFile(ks.configPath)
	if os.IsNotExist(err) {
		return []ManagedKey{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read keys file: %w", err)
	}

	var keys []ManagedKey
	if err := json.Unmarshal(data, &keys); err != nil {
		return nil, fmt.Errorf("failed to parse keys file: %w", err)
	}
	return keys, nil
}

func (ks *KeyStore) writeKeys(keys []ManagedKey) error {
	data, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal keys: %w", err)
	}
	if err := writeFileAtomic(ks.configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write keys file: %w", err)
	}
	return nil
}

// Writes the secret before the metadata so a failure cannot leave a listed key with nothing behind it.
func (ks *KeyStore) Add(key ManagedKey, privateKey, passphrase string) (ManagedKey, error) {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if key.ID == "" {
		key.ID = generateID()
	}
	key.UpdatedAt = nowMillis()
	ref := ManagedKeyRef(key.ID)

	if err := ks.SetPrivateKey(ref, privateKey); err != nil {
		return ManagedKey{}, fmt.Errorf("failed to store private key: %w", err)
	}
	if passphrase != "" {
		if err := ks.SetPassphrase(ref, passphrase); err != nil {
			ks.DeletePrivateKey(ref)
			return ManagedKey{}, fmt.Errorf("failed to store key passphrase: %w", err)
		}
	}

	keys, err := ks.listUnsafe()
	if err != nil {
		return ManagedKey{}, err
	}
	if err := ks.writeKeys(append(keys, key)); err != nil {
		ks.DeletePrivateKey(ref)
		ks.DeletePassphrase(ref)
		return ManagedKey{}, err
	}
	ks.changed()
	return key, nil
}

func (ks *KeyStore) Rename(id, name string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	keys, err := ks.listUnsafe()
	if err != nil {
		return err
	}
	for i, k := range keys {
		if k.ID == id {
			keys[i].Name = name
			keys[i].UpdatedAt = nextStamp(keyEntry(k).Stamp)
			if err := ks.writeKeys(keys); err != nil {
				return err
			}
			ks.changed()
			return nil
		}
	}
	return fmt.Errorf("key not found: %s", id)
}

func (ks *KeyStore) Remove(id string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	keys, err := ks.listUnsafe()
	if err != nil {
		return err
	}

	var removedStamp int64
	remaining := make([]ManagedKey, 0, len(keys))
	for _, k := range keys {
		if k.ID != id {
			remaining = append(remaining, k)
		} else {
			removedStamp = keyEntry(k).Stamp
		}
	}
	if len(remaining) == len(keys) {
		return fmt.Errorf("key not found: %s", id)
	}

	if err := ks.writeKeys(remaining); err != nil {
		return err
	}

	ref := ManagedKeyRef(id)
	ks.DeletePrivateKey(ref)
	ks.DeletePassphrase(ref)
	ks.recordRemoval(KeysSyncKey, id, removedStamp)
	ks.changed()
	return nil
}

// The private key and its passphrase travel through the synchronizable keychain, so only the metadata syncs here.
func keyEntry(key ManagedKey) syncedEntry[ManagedKey] {
	return syncedEntry[ManagedKey]{ID: key.ID, Stamp: max(key.UpdatedAt, key.CreatedAt*1000), Value: key}
}

func (ks *KeyStore) SyncKey() string { return string(KeysSyncKey) }

func (ks *KeyStore) ExportSync() (string, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	keys, err := ks.listUnsafe()
	if err != nil {
		return "", err
	}
	var entries []syncedEntry[ManagedKey]
	for _, key := range keys {
		entries = append(entries, keyEntry(key))
	}
	return encodeCollection(entries, ks.removed(KeysSyncKey))
}

func (ks *KeyStore) MergeSync(remote string) (bool, error) {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	keys, err := ks.listUnsafe()
	if err != nil {
		return false, err
	}
	var local []syncedEntry[ManagedKey]
	for _, key := range keys {
		local = append(local, keyEntry(key))
	}

	merge, err := mergeInto(&ks.syncHooks, KeysSyncKey, local, remote, keyEntry)
	if err != nil || !merge.changed {
		return false, err
	}
	merged := make([]ManagedKey, 0, len(merge.entries))
	for _, entry := range merge.entries {
		merged = append(merged, entry.Value)
	}
	if err := ks.writeKeys(merged); err != nil {
		return false, err
	}
	return true, ks.replaceRemoved(KeysSyncKey, merge.removed)
}
