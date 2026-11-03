//go:build !linux || android

package storage

import "errors"

func retrievePortalSecret(uintptr) error {
	return errors.New("xdg desktop portal secrets are only available on Linux")
}
