//go:build !android

package storage

import "github.com/rymdport/portal/secret"

func retrievePortalSecret(fd uintptr) error {
	_, err := secret.RetrieveSecret(fd, nil)
	return err
}
