//go:build !ios && !android && !msstore && !mas

package platform

func HasUpdateCheck() bool { return true }
