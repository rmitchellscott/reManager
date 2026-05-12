package errors

const (
	// Connection errors
	ErrAuthFailed         = "ERR_AUTH_FAILED"
	ErrConnectionRefused  = "ERR_CONNECTION_REFUSED"
	ErrTimeout            = "ERR_TIMEOUT"
	ErrNetworkUnreachable = "ERR_NETWORK_UNREACHABLE"
	ErrHostDown           = "ERR_HOST_DOWN"
	ErrDNSFailed          = "ERR_DNS_FAILED"

	// SSH key errors
	ErrKeyNotFound    = "ERR_KEY_NOT_FOUND"
	ErrKeyPassphrase  = "ERR_KEY_PASSPHRASE"
	ErrKeyParseFailed = "ERR_KEY_PARSE_FAILED"

	// File operation errors
	ErrFileNotFound     = "ERR_FILE_NOT_FOUND"
	ErrPermissionDenied = "ERR_PERMISSION_DENIED"
	ErrDiskFull = "ERR_DISK_FULL"

	// Storage/keyring errors
	ErrKeyringFailed = "ERR_KEYRING_FAILED"
	ErrStorageFailed = "ERR_STORAGE_FAILED"

	// Device errors
	ErrDeviceNotFound    = "ERR_DEVICE_NOT_FOUND"
	ErrDeviceDetection   = "ERR_DEVICE_DETECTION"
	ErrUnsupportedDevice = "ERR_UNSUPPORTED_DEVICE"

	// Operation errors
	ErrOperationCancelled = "ERR_OPERATION_CANCELLED"
	ErrSFTPFailed         = "ERR_SFTP_FAILED"

	// Fallback
	ErrUnknown = "ERR_UNKNOWN"
)
