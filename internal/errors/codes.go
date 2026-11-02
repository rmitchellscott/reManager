package errors

type ErrorCode string

const (
	ErrAuthFailed         ErrorCode = "ERR_AUTH_FAILED"
	ErrConnectionRefused  ErrorCode = "ERR_CONNECTION_REFUSED"
	ErrTimeout            ErrorCode = "ERR_TIMEOUT"
	ErrNetworkUnreachable ErrorCode = "ERR_NETWORK_UNREACHABLE"
	ErrHostDown           ErrorCode = "ERR_HOST_DOWN"
	ErrDNSFailed          ErrorCode = "ERR_DNS_FAILED"
	ErrShellRCNoisy       ErrorCode = "ERR_SHELL_RC_NOISY"

	ErrKeyNotFound    ErrorCode = "ERR_KEY_NOT_FOUND"
	ErrKeyPassphrase  ErrorCode = "ERR_KEY_PASSPHRASE"
	ErrKeyParseFailed ErrorCode = "ERR_KEY_PARSE_FAILED"
	ErrKeyNotChosen   ErrorCode = "ERR_KEY_NOT_CHOSEN"

	ErrFileNotFound     ErrorCode = "ERR_FILE_NOT_FOUND"
	ErrPermissionDenied ErrorCode = "ERR_PERMISSION_DENIED"
	ErrDiskFull         ErrorCode = "ERR_DISK_FULL"

	ErrKeyringFailed ErrorCode = "ERR_KEYRING_FAILED"
	ErrStorageFailed ErrorCode = "ERR_STORAGE_FAILED"

	ErrDeviceNotFound    ErrorCode = "ERR_DEVICE_NOT_FOUND"
	ErrDeviceDetection   ErrorCode = "ERR_DEVICE_DETECTION"
	ErrUnsupportedDevice ErrorCode = "ERR_UNSUPPORTED_DEVICE"
	ErrDeviceChanged     ErrorCode = "ERR_DEVICE_CHANGED"

	ErrOperationCanceled ErrorCode = "ERR_OPERATION_CANCELED"
	ErrSFTPFailed        ErrorCode = "ERR_SFTP_FAILED"
	ErrInvalidInput      ErrorCode = "ERR_INVALID_INPUT"

	ErrTargetSlotUnhealthy  ErrorCode = "ERR_TARGET_SLOT_UNHEALTHY"
	ErrSlotCheckUnavailable ErrorCode = "ERR_SLOT_CHECK_UNAVAILABLE"
	ErrUpdateInProgress     ErrorCode = "ERR_UPDATE_IN_PROGRESS"
	ErrLockDirMissing       ErrorCode = "ERR_LOCK_DIR_MISSING"

	ErrUnknown ErrorCode = "ERR_UNKNOWN"
)
