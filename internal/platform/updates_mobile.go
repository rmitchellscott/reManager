//go:build (ios || android) && !msstore && !sideload

package platform

// The stores deliver mobile updates, and App Review rejects an app that sends its users anywhere else for one.
func HasUpdateCheck() bool { return false }
