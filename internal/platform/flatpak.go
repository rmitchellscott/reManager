package platform

import "os"

func IsRunningInFlatpak() bool {
	_, err := os.Stat("/.flatpak-info")
	return err == nil
}
