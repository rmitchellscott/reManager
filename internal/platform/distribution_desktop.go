//go:build !ios && !android && !msstore && !mas

package platform

func DistributionChannel() string {
	if IsRunningInFlatpak() {
		return "flatpak"
	}
	return "direct"
}
