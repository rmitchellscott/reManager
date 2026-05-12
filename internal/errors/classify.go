package errors

import (
	"net"
	"os"
	"strings"
)

func Classify(err error) *UserError {
	if err == nil {
		return nil
	}

	errStr := strings.ToLower(err.Error())

	// SSH authentication errors
	if strings.Contains(errStr, "authentication failed") ||
		strings.Contains(errStr, "unable to authenticate") ||
		strings.Contains(errStr, "permission denied (publickey") ||
		strings.Contains(errStr, "handshake failed") && strings.Contains(errStr, "authenticate") {
		return New(ErrAuthFailed, "Authentication failed. Please check your password or SSH key.", err, false)
	}

	// SSH key passphrase errors
	if strings.Contains(errStr, "passphrase") ||
		strings.Contains(errStr, "incorrect passphrase") ||
		strings.Contains(errStr, "decryption password") {
		return New(ErrKeyPassphrase, "Incorrect passphrase for SSH key.", err, false)
	}

	// SSH key parsing errors
	if strings.Contains(errStr, "failed to parse key") ||
		strings.Contains(errStr, "failed to parse private key") ||
		(strings.Contains(errStr, "not a valid") && strings.Contains(errStr, "key")) ||
		strings.Contains(errStr, "no key found") {
		return New(ErrKeyParseFailed, "Could not read SSH key. The file may be corrupted or in an unsupported format.", err, false)
	}

	// Key file not found
	if strings.Contains(errStr, "failed to read key file") ||
		(strings.Contains(errStr, "key") && strings.Contains(errStr, "no such file")) {
		return New(ErrKeyNotFound, "SSH key file not found. Please select a valid key file.", err, false)
	}

	// Connection refused
	if strings.Contains(errStr, "connection refused") {
		return New(ErrConnectionRefused, "Could not connect to device. Make sure it's powered on and SSH is enabled.", err, true)
	}

	// Timeout errors
	if strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "i/o timeout") ||
		strings.Contains(errStr, "deadline exceeded") ||
		strings.Contains(errStr, "timed out") {
		return New(ErrTimeout, "Connection timed out. The device may be busy or unreachable.", err, true)
	}

	// Network unreachable
	if strings.Contains(errStr, "network is unreachable") ||
		strings.Contains(errStr, "no route to host") {
		return New(ErrNetworkUnreachable, "Network unreachable. Check your network connection.", err, true)
	}

	// Host down
	if strings.Contains(errStr, "host is down") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "connection closed") {
		return New(ErrHostDown, "Device appears to be offline or connection was interrupted.", err, true)
	}

	// DNS errors
	if strings.Contains(errStr, "no such host") ||
		(strings.Contains(errStr, "lookup") && strings.Contains(errStr, "failed")) ||
		strings.Contains(errStr, "could not resolve") ||
		strings.Contains(errStr, "dns:") {
		return New(ErrDNSFailed, "Could not resolve hostname. Check the address or try using an IP address.", err, false)
	}

	// Permission denied (file operations)
	if strings.Contains(errStr, "permission denied") {
		return New(ErrPermissionDenied, "Permission denied. You may not have access to this resource.", err, false)
	}

	// File not found
	if os.IsNotExist(err) || strings.Contains(errStr, "no such file") ||
		strings.Contains(errStr, "file not found") ||
		strings.Contains(errStr, "does not exist") {
		return New(ErrFileNotFound, "File or directory not found.", err, false)
	}

	// Disk full
	if strings.Contains(errStr, "no space left") ||
		strings.Contains(errStr, "disk full") ||
		strings.Contains(errStr, "not enough space") {
		return New(ErrDiskFull, "Not enough storage space on device.", err, false)
	}

	// Keyring errors
	if strings.Contains(errStr, "keyring") ||
		strings.Contains(errStr, "secret service") ||
		strings.Contains(errStr, "failed to store password") ||
		strings.Contains(errStr, "failed to store key passphrase") ||
		strings.Contains(errStr, "credential") && strings.Contains(errStr, "store") {
		return New(ErrKeyringFailed, "Could not save credentials to system keyring. You may need to re-enter them next time.", err, false)
	}

	// SFTP errors
	if strings.Contains(errStr, "sftp") {
		return New(ErrSFTPFailed, "File transfer operation failed.", err, true)
	}

	// Device detection errors
	if strings.Contains(errStr, "failed to detect") ||
		strings.Contains(errStr, "device detection") {
		return New(ErrDeviceDetection, "Could not detect device type.", err, false)
	}

	// Unsupported device
	if strings.Contains(errStr, "unsupported") && strings.Contains(errStr, "device") {
		return New(ErrUnsupportedDevice, "This device is not supported.", err, false)
	}

	// Operation cancelled
	if strings.Contains(errStr, "cancelled") ||
		strings.Contains(errStr, "canceled") ||
		strings.Contains(errStr, "context canceled") {
		return New(ErrOperationCancelled, "Operation was cancelled.", err, false)
	}

	// Check for net.Error interface
	if netErr, ok := err.(net.Error); ok {
		if netErr.Timeout() {
			return New(ErrTimeout, "Connection timed out. The device may be busy or unreachable.", err, true)
		}
	}

	// Fallback - generic error
	return New(ErrUnknown, "An unexpected error occurred.", err, false)
}
