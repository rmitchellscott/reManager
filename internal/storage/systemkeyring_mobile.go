//go:build android || ios

package storage

import "errors"

var errNoSystemKeyring = errors.New("mobile builds have no system keyring; secrets go through the native store")

func systemKeyringSet(string, string, string) error {
	return errNoSystemKeyring
}

func systemKeyringGet(string, string) (string, error) {
	return "", errNoSystemKeyring
}

func systemKeyringDelete(string, string) error {
	return errNoSystemKeyring
}
