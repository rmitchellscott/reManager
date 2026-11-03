//go:build !darwin

package storage

import "fmt"

// Supplied by the host so this package needs no UI framework; Android's store is reached through an app-layer bridge.
type NativeSecretStore interface {
	Set(service, key, value string) error
	Get(service, key string) (string, error)
	Delete(service, key string) error
}

var nativeSecrets NativeSecretStore

// Must run before any secret is read or written.
func SetNativeSecretStore(s NativeSecretStore) { nativeSecrets = s }

func nativeAvailable() bool { return nativeSecrets != nil }

func nativeSet(service, key, value string) error {
	if nativeSecrets == nil {
		return fmt.Errorf("native secret store unavailable")
	}
	return nativeSecrets.Set(service, key, value)
}

func nativeGet(service, key string) (string, error) {
	if nativeSecrets == nil {
		return "", fmt.Errorf("native secret store unavailable")
	}
	return nativeSecrets.Get(service, key)
}

func nativeDelete(service, key string) error {
	if nativeSecrets == nil {
		return fmt.Errorf("native secret store unavailable")
	}
	return nativeSecrets.Delete(service, key)
}
