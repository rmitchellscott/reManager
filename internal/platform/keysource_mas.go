//go:build mas

package platform

// The App Sandbox gives the app its own home, so ~/.ssh is out of reach, and it refuses a connection to the agent's socket, so reManager holds imported keys itself.
func HasLocalKeyFiles() bool { return false }

func HasSSHAgent() bool { return false }
