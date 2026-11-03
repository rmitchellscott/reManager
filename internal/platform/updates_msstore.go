//go:build msstore

package platform

// The Microsoft Store delivers updates to a packaged install, and Store Policy 10.8.1 rejects an app that sends its users anywhere else for one.
func HasUpdateCheck() bool { return false }
