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

	if strings.Contains(errStr, "authentication failed") ||
		strings.Contains(errStr, "unable to authenticate") ||
		strings.Contains(errStr, "permission denied (publickey") ||
		strings.Contains(errStr, "handshake failed") && strings.Contains(errStr, "authenticate") {
		return New(ErrAuthFailed, "Authentication failed. Check your password or SSH key.", err, false)
	}

	if strings.Contains(errStr, "passphrase") ||
		strings.Contains(errStr, "incorrect passphrase") ||
		strings.Contains(errStr, "decryption password") {
		return New(ErrKeyPassphrase, "Incorrect passphrase for SSH key.", err, false)
	}

	if strings.Contains(errStr, "failed to parse key") ||
		strings.Contains(errStr, "failed to parse private key") ||
		(strings.Contains(errStr, "not a valid") && strings.Contains(errStr, "key")) ||
		strings.Contains(errStr, "no key found") {
		return New(ErrKeyParseFailed, "Could not read SSH key. The file may be corrupted or in an unsupported format.", err, false)
	}

	if strings.Contains(errStr, "failed to read key file") ||
		(strings.Contains(errStr, "key") && strings.Contains(errStr, "no such file")) {
		return New(ErrKeyNotFound, "SSH key file not found. Select a valid key file.", err, false)
	}

	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "actively refused") {
		return New(ErrConnectionRefused, "Could not connect to your reMarkable. Make sure it's on and SSH is enabled, or try restarting it.", err, true)
	}

	if strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "i/o timeout") ||
		strings.Contains(errStr, "deadline exceeded") ||
		strings.Contains(errStr, "timed out") ||
		strings.Contains(errStr, "did not properly respond") ||
		strings.Contains(errStr, "wsarecv") ||
		strings.Contains(errStr, "wsasend") {
		return New(ErrTimeout, "Connection timed out. Your reMarkable may be busy or unreachable.", err, true)
	}

	if strings.Contains(errStr, "network is unreachable") ||
		strings.Contains(errStr, "no route to host") {
		return New(ErrNetworkUnreachable, "Network unreachable. Check your network connection.", err, true)
	}

	if strings.Contains(errStr, "host is down") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "connection closed") {
		return New(ErrHostDown, "Your reMarkable appears to be offline, or the connection was interrupted.", err, true)
	}

	if strings.Contains(errStr, "no such host") ||
		(strings.Contains(errStr, "lookup") && strings.Contains(errStr, "failed")) ||
		strings.Contains(errStr, "could not resolve") ||
		strings.Contains(errStr, "dns:") {
		return New(ErrDNSFailed, "Could not resolve hostname. Check the address, or use an IP address instead.", err, false)
	}

	if strings.Contains(errStr, "permission denied") {
		return New(ErrPermissionDenied, "Permission denied.", err, false)
	}

	if os.IsNotExist(err) || strings.Contains(errStr, "no such file") ||
		strings.Contains(errStr, "file not found") ||
		strings.Contains(errStr, "does not exist") {
		return New(ErrFileNotFound, "File or folder not found.", err, false)
	}

	if strings.Contains(errStr, "no space left") ||
		strings.Contains(errStr, "disk full") ||
		strings.Contains(errStr, "not enough space") {
		return New(ErrDiskFull, "Not enough storage space on your reMarkable.", err, false)
	}

	if strings.Contains(errStr, "keyring") ||
		strings.Contains(errStr, "secret service") ||
		strings.Contains(errStr, "failed to store password") ||
		strings.Contains(errStr, "failed to store key passphrase") ||
		strings.Contains(errStr, "credential") && strings.Contains(errStr, "store") {
		return New(ErrKeyringFailed, "Could not save credentials to the system keychain. You may need to enter them again next time.", err, false)
	}

	if strings.Contains(errStr, "sftp") {
		return New(ErrSFTPFailed, "File transfer failed.", err, true)
	}

	if strings.Contains(errStr, "failed to detect") ||
		strings.Contains(errStr, "device detection") {
		return New(ErrDeviceDetection, "Could not detect which reMarkable model this is.", err, false)
	}

	if strings.Contains(errStr, "unsupported") && strings.Contains(errStr, "device") {
		return New(ErrUnsupportedDevice, "This reMarkable model is not supported.", err, false)
	}

	if strings.Contains(errStr, "canceled") {
		return New(ErrOperationCanceled, "Operation was canceled.", err, false)
	}

	if netErr, ok := err.(net.Error); ok {
		if netErr.Timeout() {
			return New(ErrTimeout, "Connection timed out. Your reMarkable may be busy or unreachable.", err, true)
		}
	}

	return New(ErrUnknown, "Something went wrong. Try again, or restart your reMarkable if it keeps happening.", err, false)
}
