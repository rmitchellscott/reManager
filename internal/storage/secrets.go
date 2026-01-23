package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"reManager/internal/platform"
	"github.com/rymdport/portal/secret"
	"github.com/zalando/go-keyring"
)

type SecretStore struct {
	serviceName   string
	secretsPath   string
	masterKey     []byte
	masterKeyOnce sync.Once
	mu            sync.RWMutex
}

func NewSecretStore(serviceName string) (*SecretStore, error) {
	ss := &SecretStore{
		serviceName: serviceName,
	}

	if platform.IsRunningInFlatpak() {
		dataDir := os.Getenv("XDG_DATA_HOME")
		if dataDir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("failed to get home directory: %w", err)
			}
			dataDir = filepath.Join(home, ".local", "share")
		}
		secretsDir := filepath.Join(dataDir, "reManager")
		if err := os.MkdirAll(secretsDir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create secrets directory: %w", err)
		}
		ss.secretsPath = filepath.Join(secretsDir, "secrets.json")
	}

	return ss, nil
}

func (ss *SecretStore) Set(key, value string) error {
	if platform.IsRunningInFlatpak() {
		return ss.setFlatpak(key, value)
	}
	return keyring.Set(ss.serviceName, key, value)
}

func (ss *SecretStore) Get(key string) (string, error) {
	if platform.IsRunningInFlatpak() {
		return ss.getFlatpak(key)
	}
	return keyring.Get(ss.serviceName, key)
}

func (ss *SecretStore) Delete(key string) error {
	if platform.IsRunningInFlatpak() {
		return ss.deleteFlatpak(key)
	}
	return keyring.Delete(ss.serviceName, key)
}

func (ss *SecretStore) getMasterKey() ([]byte, error) {
	var initErr error
	ss.masterKeyOnce.Do(func() {
		r, w, err := os.Pipe()
		if err != nil {
			initErr = fmt.Errorf("failed to create pipe: %w", err)
			return
		}
		defer r.Close()

		_, err = secret.RetrieveSecret(w.Fd(), nil)
		w.Close()
		if err != nil {
			initErr = fmt.Errorf("failed to retrieve portal secret: %w", err)
			return
		}

		secretData, err := io.ReadAll(r)
		if err != nil {
			initErr = fmt.Errorf("failed to read portal secret: %w", err)
			return
		}

		hash := sha256.Sum256(secretData)
		ss.masterKey = hash[:]
	})

	if initErr != nil {
		return nil, initErr
	}
	if ss.masterKey == nil {
		return nil, fmt.Errorf("master key not initialized")
	}
	return ss.masterKey, nil
}

func (ss *SecretStore) encrypt(plaintext string) (string, error) {
	key, err := ss.getMasterKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (ss *SecretStore) decrypt(encoded string) (string, error) {
	key, err := ss.getMasterKey()
	if err != nil {
		return "", err
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	if len(ciphertext) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce := ciphertext[:gcm.NonceSize()]
	ciphertext = ciphertext[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}

func (ss *SecretStore) loadSecrets() (map[string]string, error) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	data, err := os.ReadFile(ss.secretsPath)
	if os.IsNotExist(err) {
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read secrets file: %w", err)
	}

	var secrets map[string]string
	if err := json.Unmarshal(data, &secrets); err != nil {
		return nil, fmt.Errorf("failed to parse secrets file: %w", err)
	}
	return secrets, nil
}

func (ss *SecretStore) saveSecrets(secrets map[string]string) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	data, err := json.MarshalIndent(secrets, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal secrets: %w", err)
	}

	if err := os.WriteFile(ss.secretsPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write secrets file: %w", err)
	}
	return nil
}

func (ss *SecretStore) setFlatpak(key, value string) error {
	encrypted, err := ss.encrypt(value)
	if err != nil {
		return err
	}

	secrets, err := ss.loadSecrets()
	if err != nil {
		return err
	}

	secrets[key] = encrypted
	return ss.saveSecrets(secrets)
}

func (ss *SecretStore) getFlatpak(key string) (string, error) {
	secrets, err := ss.loadSecrets()
	if err != nil {
		return "", err
	}

	encrypted, ok := secrets[key]
	if !ok {
		return "", fmt.Errorf("secret not found: %s", key)
	}

	return ss.decrypt(encrypted)
}

func (ss *SecretStore) deleteFlatpak(key string) error {
	secrets, err := ss.loadSecrets()
	if err != nil {
		return err
	}

	delete(secrets, key)
	return ss.saveSecrets(secrets)
}
