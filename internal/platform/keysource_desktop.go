//go:build !ios && !android && !mas

package platform

// The desktop builds run where the user has their own ~/.ssh and, usually, an agent, so reManager points at those rather than keeping a copy of a key.
func HasLocalKeyFiles() bool { return true }

func HasSSHAgent() bool { return true }
