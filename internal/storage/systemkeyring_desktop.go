//go:build !android && !ios

package storage

import "github.com/zalando/go-keyring"

func systemKeyringSet(service, key, value string) error {
	return keyring.Set(service, key, value)
}

func systemKeyringGet(service, key string) (string, error) {
	return keyring.Get(service, key)
}

func systemKeyringDelete(service, key string) error {
	return keyring.Delete(service, key)
}
