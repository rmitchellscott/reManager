package commands

import (
	rmdevice "github.com/rmitchellscott/remarkable-go/device"

	"reManager/internal/component"
)

func GetDownloadArch(arch rmdevice.Architecture) string {
	if arch == rmdevice.Arm32 {
		return "armv7"
	}
	return string(arch)
}

// WritableRootSteps returns prepare and restore steps for the Paper Pro root; the restore steps check current mounts, so they are safe to run after any prepare step failed.
func WritableRootSteps(dev rmdevice.Type) (prepare, restore []component.CommandResult) {
	if !dev.IsPaperPro() {
		return nil, nil
	}
	prepare = []component.CommandResult{
		{
			Script:      `echo "[DEBUG] Current root mount state:" && grep ' / ' /proc/mounts && echo "[DEBUG] Checking if root is rw..." && if grep -q ' / .*\brw\b' /proc/mounts; then echo "[DEBUG] Root is already rw"; else echo "[DEBUG] Remounting root as rw..." && mount -o remount,rw / && echo "[DEBUG] Root remounted as rw"; fi && sleep 1`,
			Description: "Ensure root filesystem is writable",
		},
		{
			Script:      `echo "[DEBUG] Current /etc mount state:" && grep '/etc' /proc/mounts || echo "[DEBUG] No /etc in mounts" && echo "[DEBUG] Checking for overlay on /etc..." && if grep -q '^overlay.*/etc' /proc/mounts; then echo "[DEBUG] Overlay found, lazy unmounting..." && umount -l /etc && echo "[DEBUG] /etc overlay lazy unmounted"; else echo "[DEBUG] No overlay on /etc"; fi && sleep 1`,
			Description: "Unmount /etc overlay if mounted",
		},
	}
	restore = []component.CommandResult{
		{
			Script:      `echo "[DEBUG] Checking if overlay restore needed..." && if grep -q '^overlay.*/etc' /proc/mounts; then echo "[DEBUG] Overlay already mounted"; elif [ -d /var/volatile/.etc-work ]; then echo "[DEBUG] Workdir exists, restoring overlay..." && rm -rf /var/volatile/.etc-work/* && mount -t overlay overlay -o rw,relatime,lowerdir=/etc,upperdir=/var/volatile/etc,workdir=/var/volatile/.etc-work /etc && echo "[DEBUG] Overlay restored"; else echo "[DEBUG] No workdir, skipping overlay restore"; fi`,
			Description: "Restore /etc overlay",
		},
		{
			Script:      `echo "[DEBUG] Syncing and remounting root as ro..." && sync && mount -o remount,ro / && echo "[DEBUG] Root remounted as ro" && echo "[DEBUG] Final mount state:" && grep ' / ' /proc/mounts`,
			Description: "Remount root as read-only",
		},
	}
	return prepare, restore
}
