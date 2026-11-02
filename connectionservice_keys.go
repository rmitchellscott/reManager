package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	apperrors "reManager/internal/errors"
	"reManager/internal/platform"
	"reManager/internal/storage"
	"reManager/internal/support"
)

// PlatformCapabilities reports what the build can reach, which is not the same
// question as what the pointer looks like: a touchscreen desktop still has the
// user's own ~/.ssh and an agent.
type PlatformCapabilities struct {
	// ManagedKeys is every platform now, because keys held in the keychain are
	// the ones that sync between a user's hosts. LocalKeyFiles says whether the
	// user's own ~/.ssh is also there to pick from.
	ManagedKeys   bool `json:"managedKeys"`
	LocalKeyFiles bool `json:"localKeyFiles"`
	SSHAgent      bool `json:"sshAgent"`
	UpdateCheck   bool `json:"updateCheck"`
	// Android resizes the web view for the soft keyboard (adjustResize), while
	// iOS lays the keyboard over it. The runtime's IsAndroid cannot say which,
	// because the Android build never populates its environment.
	KeyboardResizesView bool `json:"keyboardResizesView"`
	// The docked iPad keyboard carries Tab, a dismiss key and a shortcut row
	// with paste, which the terminal's key bar would otherwise repeat.
	IPad          bool `json:"iPad"`
	SupportUpload bool `json:"supportUpload"`
}

func (s *SystemService) GetPlatformCapabilities() PlatformCapabilities {
	return PlatformCapabilities{
		ManagedKeys:         true,
		LocalKeyFiles:       platform.HasLocalKeyFiles(),
		SSHAgent:            platform.HasSSHAgent(),
		UpdateCheck:         platform.HasUpdateCheck(),
		KeyboardResizesView: runtime.GOOS == "android",
		IPad:                platform.IsIPad(),
		SupportUpload:       support.UploadAvailable(),
	}
}

type ManagedKeyInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	PublicKey   string `json:"publicKey,omitempty"`
	Encrypted   bool   `json:"encrypted,omitempty"`
	CreatedAt   int64  `json:"createdAt,omitempty"`
}

type ImportKeyResult struct {
	Success bool                `json:"success"`
	Message string              `json:"message,omitempty"`
	Code    apperrors.ErrorCode `json:"code,omitempty"`
	Key     ManagedKeyInfo      `json:"key,omitempty"`
}

func toManagedKeyInfo(key storage.ManagedKey) ManagedKeyInfo {
	return ManagedKeyInfo{
		ID:          key.ID,
		Name:        key.Name,
		Type:        key.Type,
		Fingerprint: key.Fingerprint,
		PublicKey:   key.PublicKey,
		Encrypted:   key.Encrypted,
		CreatedAt:   key.CreatedAt,
	}
}

func (s *ConnectionService) GetManagedKeys() []ManagedKeyInfo {
	if s.deviceStore == nil {
		return []ManagedKeyInfo{}
	}
	keys, err := s.deviceStore.Keys().List()
	if err != nil {
		fmt.Printf("Error listing managed keys: %v\n", err)
		return []ManagedKeyInfo{}
	}

	result := make([]ManagedKeyInfo, len(keys))
	for i, k := range keys {
		result[i] = toManagedKeyInfo(k)
	}
	return result
}

// describeKey rejects anything that is not a usable private key at import time,
// so a bad paste fails in the key manager rather than on a later connect.
func describeKey(privateKey, passphrase string) (storage.ManagedKey, error) {
	data := []byte(privateKey)

	var signer ssh.Signer
	var err error
	if passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(passphrase))
	} else {
		signer, err = ssh.ParsePrivateKey(data)
	}
	if err == nil {
		public := signer.PublicKey()
		return storage.ManagedKey{
			Type:        public.Type(),
			Fingerprint: ssh.FingerprintSHA256(public),
			PublicKey:   strings.TrimSpace(string(ssh.MarshalAuthorizedKey(public))),
			Encrypted:   passphrase != "",
		}, nil
	}

	// An encrypted key imported without its passphrase is still worth keeping;
	// it just cannot describe itself until the passphrase is supplied.
	var missing *ssh.PassphraseMissingError
	if errors.As(err, &missing) {
		key := storage.ManagedKey{Encrypted: true}
		if missing.PublicKey != nil {
			key.Type = missing.PublicKey.Type()
			key.Fingerprint = ssh.FingerprintSHA256(missing.PublicKey)
			key.PublicKey = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(missing.PublicKey)))
		}
		return key, nil
	}

	return storage.ManagedKey{}, err
}

func (s *ConnectionService) ImportManagedKey(name, privateKey, passphrase string) ImportKeyResult {
	if s.deviceStore == nil {
		return ImportKeyResult{
			Success: false,
			Message: "Could not load your saved reMarkables. Restart reManager and try again.",
			Code:    apperrors.ErrStorageFailed,
		}
	}

	privateKey = strings.TrimSpace(privateKey) + "\n"

	key, err := describeKey(privateKey, passphrase)
	if err != nil {
		ue := apperrors.Classify(err)
		return ImportKeyResult{Success: false, Message: ue.Message, Code: ue.Code}
	}

	key.Name = strings.TrimSpace(name)
	if key.Name == "" {
		key.Name = "SSH key"
	}
	key.CreatedAt = time.Now().Unix()

	stored, err := s.deviceStore.Keys().Add(key, privateKey, passphrase)
	if err != nil {
		return ImportKeyResult{
			Success: false,
			Message: "Could not save the key to the system keychain.",
			Code:    apperrors.ErrKeyringFailed,
		}
	}

	return ImportKeyResult{Success: true, Key: toManagedKeyInfo(stored)}
}

type KeyFileContents struct {
	Name       string `json:"name"`
	PrivateKey string `json:"privateKey"`
}

// ChooseKeyFileToImport fills the desktop import form from a key file, since
// copying a private key out of ~/.ssh by hand is where a paste goes wrong. An
// empty result means the user canceled.
func (s *ConnectionService) ChooseKeyFileToImport() (KeyFileContents, error) {
	path, err := openFileDialog("Select SSH private key", "")
	if err != nil || path == "" {
		return KeyFileContents{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return KeyFileContents{}, fmt.Errorf("could not read %s: %w", filepath.Base(path), err)
	}
	return KeyFileContents{Name: filepath.Base(path), PrivateKey: string(data)}, nil
}

func (s *ConnectionService) RenameManagedKey(id, name string) error {
	if s.deviceStore == nil {
		return fmt.Errorf("device store not initialized")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("a key needs a name")
	}
	return s.deviceStore.Keys().Rename(id, name)
}

func (s *ConnectionService) DeleteManagedKey(id string) error {
	if s.deviceStore == nil {
		return fmt.Errorf("device store not initialized")
	}
	return s.deviceStore.Keys().Remove(id)
}
