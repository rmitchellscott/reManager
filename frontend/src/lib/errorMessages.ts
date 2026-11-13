import type { ErrorCode } from '@/lib/types';

interface UserError {
  code: ErrorCode;
  message: string;
  retryable: boolean;
}

const errorMessages = {
  ERR_AUTH_FAILED:
    "Authentication failed. Check your password or SSH key.",
  ERR_CONNECTION_REFUSED:
    "Could not connect to your reMarkable. Make sure it's on and SSH is enabled, or try restarting it.",
  ERR_TIMEOUT: "Connection timed out. Your reMarkable may be busy or unreachable.",
  ERR_NETWORK_UNREACHABLE: "Network unreachable. Check your network connection.",
  ERR_HOST_DOWN:
    "Your reMarkable appears to be offline, or the connection was interrupted.",
  ERR_DNS_FAILED:
    "Could not resolve hostname. Check the address, or use an IP address instead.",
  ERR_VPN_BLOCKS_LOCAL:
    "Could not reach your reMarkable because a VPN is on. Turn off the VPN, or exclude reManager in the VPN app's settings.",
  ERR_SHELL_RC_NOISY:
    "Your reMarkable's shell prints output on SSH, which breaks file transfers. Guard ~/.bashrc so it stays silent in non-interactive sessions.",
  ERR_KEY_NOT_FOUND: "SSH key file not found. Select a valid key file.",
  ERR_KEY_PASSPHRASE: "Incorrect passphrase for SSH key.",
  ERR_KEY_PARSE_FAILED:
    "Could not read SSH key. The file may be corrupted or in an unsupported format.",
  ERR_KEY_NOT_CHOSEN:
    "This reMarkable was saved on another device. Choose an SSH key to use here.",
  ERR_FILE_NOT_FOUND: "File or folder not found.",
  ERR_PERMISSION_DENIED:
    "Permission denied.",
  ERR_DISK_FULL: "Not enough storage space on your reMarkable.",
  ERR_KEYRING_FAILED:
    "Could not save credentials to the system keychain. You may need to enter them again next time.",
  ERR_STORAGE_FAILED: "Could not save data.",
  ERR_DEVICE_NOT_FOUND: "Could not find this reMarkable.",
  ERR_DEVICE_DETECTION: "Could not detect which reMarkable model this is.",
  ERR_UNSUPPORTED_DEVICE: "This reMarkable model is not supported.",
  ERR_DEVICE_CHANGED:
    "A different reMarkable answered at this address. Connect again to continue with it.",
  ERR_OPERATION_CANCELED: "Operation was canceled.",
  ERR_SFTP_FAILED: "File transfer failed.",
  ERR_UNKNOWN: "Something went wrong. Try again, or restart your reMarkable if it keeps happening.",
} satisfies Partial<Record<ErrorCode, string>>;

const messageByCode: Partial<Record<ErrorCode, string>> = errorMessages;

const errorPatterns: Array<{ pattern: RegExp; message: string }> = [
  {
    pattern: /authentication failed|unable to authenticate/i,
    message: errorMessages.ERR_AUTH_FAILED,
  },
  {
    pattern: /bind to local network.*EPERM/i,
    message: errorMessages.ERR_VPN_BLOCKS_LOCAL,
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
    message: errorMessages.ERR_OPERATION_CANCELED,
  },
];

function isUserError(value: unknown): value is UserError {
  return typeof value === "object" && value !== null && typeof (value as { code?: unknown }).code === "string";
}

function userErrorOf(error: unknown): UserError | null {
  if (isUserError(error)) return error;
  const cause = typeof error === "object" && error !== null ? (error as { cause?: unknown }).cause : undefined;
  return isUserError(cause) ? cause : null;
}

function recognizedMessage(error: unknown): string | null {
  const ue = userErrorOf(error);
  if (ue) {
    return ue.message || messageByCode[ue.code] || errorMessages.ERR_UNKNOWN;
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

  return null;
}

function rawText(error: unknown): string {
  if (error instanceof Error) return error.message;
  if (typeof error === "object" && error !== null && "message" in error) {
    return String((error as { message: unknown }).message);
  }
  return String(error);
}

export function handleError(error: unknown, fallback: string): string {
  console.error(`[${fallback || "Error"}]`, error);
  return recognizedMessage(error) ?? fallback;
}

export function errorDetail(error: unknown): string {
  return recognizedMessage(error) === null ? rawText(error) : "";
}
