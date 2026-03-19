export interface UserError {
  code: string;
  message: string;
  retryable: boolean;
}

const errorMessages: Record<string, string> = {
  // Connection errors
  ERR_AUTH_FAILED:
    "Authentication failed. Please check your password or SSH key.",
  ERR_CONNECTION_REFUSED:
    "Could not connect to reMarkable. Make sure it's powered on, SSH is enabled, and try restarting the reMarkable.",
  ERR_TIMEOUT: "Connection timed out. The reMarkable may be busy or unreachable.",
  ERR_NETWORK_UNREACHABLE: "Network unreachable. Check your network connection.",
  ERR_HOST_DOWN:
    "reMarkable appears to be offline or connection was interrupted.",
  ERR_DNS_FAILED:
    "Could not resolve hostname. Check the address or try using an IP address.",

  // SSH key errors
  ERR_KEY_NOT_FOUND: "SSH key file not found. Please select a valid key file.",
  ERR_KEY_PASSPHRASE: "Incorrect passphrase for SSH key.",
  ERR_KEY_PARSE_FAILED:
    "Could not read SSH key. The file may be corrupted or in an unsupported format.",

  // File operation errors
  ERR_FILE_NOT_FOUND: "File or directory not found.",
  ERR_PERMISSION_DENIED:
    "Permission denied. You may not have access to this resource.",
  ERR_DISK_FULL: "Not enough storage space on reMarkable.",
  ERR_PATH_INVALID: "Invalid file path.",

  // Storage/keyring errors
  ERR_KEYRING_FAILED:
    "Could not save credentials to system keyring. You may need to re-enter them next time.",
  ERR_STORAGE_FAILED: "Failed to save data.",

  // Device errors
  ERR_DEVICE_NOT_FOUND: "reMarkable not found.",
  ERR_DEVICE_DETECTION: "Could not detect reMarkable type.",
  ERR_UNSUPPORTED_DEVICE: "This device is not supported.",

  // Operation errors
  ERR_OPERATION_FAILED: "Operation failed.",
  ERR_OPERATION_CANCELLED: "Operation was cancelled.",
  ERR_SFTP_FAILED: "File transfer operation failed.",

  // Fallback
  ERR_UNKNOWN: "An unexpected error occurred. Try restarting the reMarkable.",
};

const errorPatterns: Array<{ pattern: RegExp; message: string }> = [
  {
    pattern: /authentication failed|unable to authenticate/i,
    message: errorMessages.ERR_AUTH_FAILED,
  },
  {
    pattern: /connection refused/i,
    message: errorMessages.ERR_CONNECTION_REFUSED,
  },
  { pattern: /timeout|timed out/i, message: errorMessages.ERR_TIMEOUT },
  {
    pattern: /network is unreachable|no route to host/i,
    message: errorMessages.ERR_NETWORK_UNREACHABLE,
  },
  { pattern: /host is down/i, message: errorMessages.ERR_HOST_DOWN },
  { pattern: /no such host/i, message: errorMessages.ERR_DNS_FAILED },
  { pattern: /passphrase/i, message: errorMessages.ERR_KEY_PASSPHRASE },
  {
    pattern: /failed to parse key|failed to parse private key/i,
    message: errorMessages.ERR_KEY_PARSE_FAILED,
  },
  {
    pattern: /failed to read key file/i,
    message: errorMessages.ERR_KEY_NOT_FOUND,
  },
  { pattern: /permission denied/i, message: errorMessages.ERR_PERMISSION_DENIED },
  {
    pattern: /no such file|file not found|does not exist/i,
    message: errorMessages.ERR_FILE_NOT_FOUND,
  },
  {
    pattern: /no space left|disk full/i,
    message: errorMessages.ERR_DISK_FULL,
  },
  {
    pattern: /keyring|failed to store password|failed to store key passphrase/i,
    message: errorMessages.ERR_KEYRING_FAILED,
  },
  { pattern: /sftp/i, message: errorMessages.ERR_SFTP_FAILED },
  {
    pattern: /failed to detect|device detection/i,
    message: errorMessages.ERR_DEVICE_DETECTION,
  },
  {
    pattern: /unsupported.*device/i,
    message: errorMessages.ERR_UNSUPPORTED_DEVICE,
  },
  {
    pattern: /cancelled|canceled|context canceled/i,
    message: errorMessages.ERR_OPERATION_CANCELLED,
  },
];

export function getUserFriendlyMessage(error: unknown): string {
  if (typeof error === "object" && error !== null && "code" in error) {
    const ue = error as UserError;
    return errorMessages[ue.code] || ue.message || errorMessages.ERR_UNKNOWN;
  }

  const errorStr = String(error);

  for (const { pattern, message } of errorPatterns) {
    if (pattern.test(errorStr)) {
      return message;
    }
  }

  if (
    errorStr.includes(":") &&
    (errorStr.includes("dial ") ||
      errorStr.includes("ssh:") ||
      errorStr.includes("tcp ") ||
      errorStr.includes("read ") ||
      errorStr.includes("write "))
  ) {
    return errorMessages.ERR_UNKNOWN;
  }

  return errorStr;
}

export function handleError(error: unknown, context?: string): string {
  const message = getUserFriendlyMessage(error);
  console.error(`[${context || "Error"}]`, error);
  return message;
}
