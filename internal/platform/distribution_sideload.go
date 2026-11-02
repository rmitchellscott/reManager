//go:build android && sideload

package platform

func DistributionChannel() string { return "sideload" }
