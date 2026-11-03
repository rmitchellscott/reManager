//go:build android

package main

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"

	"reManager/internal/storage"
)

// androidSecrets stores credentials in EncryptedSharedPreferences, reached
// through the Wails bridge.
type androidSecrets struct{}

// The preference namespace is flat, so the service name is folded into the key, matching the iOS Keychain path.
func androidSecretKey(service, key string) string {
	return service + "." + key
}

func (androidSecrets) Set(service, key, value string) error {
	return application.Android.SecureSet(androidSecretKey(service, key), value)
}

// A missing secret is an error, as from the Keychain and go-keyring, so a device whose password is gone fails to connect rather than dialing empty.
func (androidSecrets) Get(service, key string) (string, error) {
	value, found, err := application.Android.SecureGet(androidSecretKey(service, key))
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("no secret stored for %q", key)
	}
	return value, nil
}

func (androidSecrets) Delete(service, key string) error {
	return application.Android.SecureDelete(androidSecretKey(service, key))
}

func init() { storage.SetNativeSecretStore(androidSecrets{}) }
