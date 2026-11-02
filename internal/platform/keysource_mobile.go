//go:build ios || android

package platform

// A mobile app gets no filesystem the user can put a key file on, and no agent to ask, so reManager holds imported keys itself.
func HasLocalKeyFiles() bool { return false }

func HasSSHAgent() bool { return false }
