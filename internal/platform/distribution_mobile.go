//go:build (ios || android) && !msstore && !sideload

package platform

func DistributionChannel() string { return "" }
