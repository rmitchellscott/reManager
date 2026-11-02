package storage

import (
	"errors"
	"runtime"
	"strings"
	"sync"
)

// iOS always takes the keychain; macOS falls back to go-keyring when the build is not entitled to the data-protection keychain.
var dataProtectionKeychain = sync.OnceValue(func() bool {
	return runtime.GOOS == "ios" || keychainAvailable()
})

func nativeAvailable() bool { return dataProtectionKeychain() }

const keychainService = "io.scottlabs.reManager"

// The Keychain account namespace is flat, so the store's service name is folded into the account.
func nativeKey(service, key string) string {
	return service + "." + key
}

// Support bundle tokens stay on the uploading host, because the bundle list they belong to never syncs.
func syncsAcrossDevices(key string) bool {
	return !strings.HasPrefix(key, "bundle:")
}

func nativeSet(service, key, value string) error {
	return keychainSet(keychainService, nativeKey(service, key), value, syncsAcrossDevices(key))
}

// A pre-data-protection macOS secret is migrated from the login keychain lazily on first read (a startup sweep would raise one prompt per item); the login copy stays because an ad-hoc signed build can only read from there.
func nativeGet(service, key string) (string, error) {
	value, err := keychainGet(keychainService, nativeKey(service, key))
	if err == nil || !errors.Is(err, errSecretNotFound) || runtime.GOOS != "darwin" {
		return value, err
	}
	legacy, legacyErr := systemKeyringGet(service, key)
	if legacyErr != nil {
		return "", err
	}
	_ = nativeSet(service, key, legacy)
	return legacy, nil
}

func nativeDelete(service, key string) error {
	if runtime.GOOS == "darwin" {
		systemKeyringDelete(service, key)
	}
	return keychainDelete(keychainService, nativeKey(service, key))
}
