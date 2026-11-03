//go:build mas

package platform

// The Mac App Store delivers updates to its installs, and App Review rejects an app that sends its users anywhere else for one.
func HasUpdateCheck() bool { return false }
