// Package deviceid derives a stable, opaque identity for a connected
// reMarkable, so reManager can tell one tablet from another at the same
// address rather than trusting the saved entry it was reached through.
package deviceid

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// systemd's sd_id128_get_machine_app_specific: HMAC-SHA256 keyed by the machine ID over this application ID, truncated to 128 bits and stamped as a version 4 UUID.
var applicationID = [16]byte{
	0x6f, 0x2d, 0x4b, 0x1e, 0x8c, 0x53, 0x4a, 0x7d,
	0x9e, 0x21, 0xb4, 0x60, 0xf3, 0x85, 0xa7, 0xc9,
}

// Derive turns /etc/machine-id's contents into the stored identity; the returned value is safe to keep, the input is not.
func Derive(machineIDFile string) (string, error) {
	raw := strings.TrimSpace(machineIDFile)
	if raw == "" {
		return "", fmt.Errorf("machine ID is empty")
	}

	machineID, err := hex.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("machine ID is not hexadecimal: %w", err)
	}
	if len(machineID) != 16 {
		return "", fmt.Errorf("machine ID is %d bytes, want 16", len(machineID))
	}

	mac := hmac.New(sha256.New, machineID)
	mac.Write(applicationID[:])

	derived := mac.Sum(nil)[:16]
	derived[6] = (derived[6] & 0x0f) | 0x40
	derived[8] = (derived[8] & 0x3f) | 0x80

	return hex.EncodeToString(derived), nil
}
