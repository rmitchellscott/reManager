//go:build android && sideload

package platform

func HasUpdateCheck() bool { return true }
