package storage

/*
#cgo LDFLAGS: -framework CoreFoundation -framework Security
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdlib.h>
#include <string.h>

static CFMutableDictionaryRef kcQuery(const char *service, const char *account) {
	CFMutableDictionaryRef q = CFDictionaryCreateMutable(NULL, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
	CFStringRef s = CFStringCreateWithCString(NULL, service, kCFStringEncodingUTF8);
	CFStringRef a = CFStringCreateWithCString(NULL, account, kCFStringEncodingUTF8);
	CFDictionarySetValue(q, kSecAttrService, s);
	CFDictionarySetValue(q, kSecAttrAccount, a);
	// Without this, macOS uses the legacy file keychain, which never syncs; iOS has only the data-protection keychain, so the flag is a no-op there.
	CFDictionarySetValue(q, kSecUseDataProtectionKeychain, kCFBooleanTrue);
	CFRelease(s);
	CFRelease(a);
	return q;
}

// Synchronizable is incompatible with a ThisDeviceOnly accessibility class: such items never sync to iCloud Keychain nor restore onto a new device.
static int kcSet(const char *service, const char *account, const char *value, int synchronizable) {
	CFMutableDictionaryRef q = kcQuery(service, account);
	CFDictionarySetValue(q, kSecAttrSynchronizable, kSecAttrSynchronizableAny);
	SecItemDelete(q);
	CFDictionarySetValue(q, kSecAttrSynchronizable, synchronizable ? kCFBooleanTrue : kCFBooleanFalse);
	CFDataRef d = CFDataCreate(NULL, (const UInt8 *)value, (CFIndex)strlen(value));
	CFDictionarySetValue(q, kSecValueData, d);
	CFDictionarySetValue(q, kSecAttrAccessible, kSecAttrAccessibleWhenUnlocked);
	OSStatus st = SecItemAdd(q, NULL);
	CFRelease(d);
	CFRelease(q);
	return (int)st;
}

static int kcGet(const char *service, const char *account, char **out) {
	CFMutableDictionaryRef q = kcQuery(service, account);
	// A query without this matches only non-synchronizable items, so Any is required to find both synchronizable and non-synchronizable items.
	CFDictionarySetValue(q, kSecAttrSynchronizable, kSecAttrSynchronizableAny);
	CFDictionarySetValue(q, kSecReturnData, kCFBooleanTrue);
	CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
	CFTypeRef res = NULL;
	OSStatus st = SecItemCopyMatching(q, &res);
	CFRelease(q);
	if (st == errSecSuccess && res != NULL) {
		CFDataRef d = (CFDataRef)res;
		CFIndex n = CFDataGetLength(d);
		char *buf = (char *)malloc((size_t)n + 1);
		memcpy(buf, CFDataGetBytePtr(d), (size_t)n);
		buf[n] = 0;
		*out = buf;
		CFRelease(res);
	}
	return (int)st;
}

// An unentitled build (such as ad-hoc signed macOS) is refused writes to the data-protection keychain but reads return item-not-found, so the probe must be a mutating call to surface the entitlement error.
static int kcAvailable(void) {
	CFMutableDictionaryRef q = kcQuery("io.scottlabs.reManager.probe", "probe");
	CFDictionarySetValue(q, kSecAttrSynchronizable, kSecAttrSynchronizableAny);
	OSStatus st = SecItemDelete(q);
	CFRelease(q);
	return st != errSecMissingEntitlement;
}

static int kcDelete(const char *service, const char *account) {
	CFMutableDictionaryRef q = kcQuery(service, account);
	CFDictionarySetValue(q, kSecAttrSynchronizable, kSecAttrSynchronizableAny);
	OSStatus st = SecItemDelete(q);
	CFRelease(q);
	return (int)st;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"

	"reManager/internal/debug"
)

const (
	errSecSuccess      = 0
	errSecItemNotFound = -25300
)

var errSecretNotFound = errors.New("secret not found")

// Wails' IOS.SecureSet/SecureGet use one fixed service with this-device-only accessibility (and do not exist on macOS), so these call the Security framework directly to separate services and allow iCloud sync.
func keychainSet(service, account, value string, synchronizable bool) error {
	cs, ca, cv := C.CString(service), C.CString(account), C.CString(value)
	defer C.free(unsafe.Pointer(cs))
	defer C.free(unsafe.Pointer(ca))
	defer C.free(unsafe.Pointer(cv))

	syncFlag := C.int(0)
	if synchronizable {
		syncFlag = 1
	}
	if st := int(C.kcSet(cs, ca, cv, syncFlag)); st != errSecSuccess {
		debug.Println("[DEBUG] keychain write failed:", account, "OSStatus:", st)
		return fmt.Errorf("keychain write failed (OSStatus %d)", st)
	}
	return nil
}

func keychainGet(service, account string) (string, error) {
	cs, ca := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(cs))
	defer C.free(unsafe.Pointer(ca))

	var out *C.char
	st := int(C.kcGet(cs, ca, &out))
	if st == errSecItemNotFound {
		return "", fmt.Errorf("%w: %s", errSecretNotFound, account)
	}
	if st != errSecSuccess {
		return "", fmt.Errorf("keychain read failed (OSStatus %d)", st)
	}
	if out == nil {
		return "", fmt.Errorf("%w: %s", errSecretNotFound, account)
	}
	defer C.free(unsafe.Pointer(out))
	return C.GoString(out), nil
}

func keychainDelete(service, account string) error {
	cs, ca := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(cs))
	defer C.free(unsafe.Pointer(ca))

	if st := int(C.kcDelete(cs, ca)); st != errSecSuccess && st != errSecItemNotFound {
		return fmt.Errorf("keychain delete failed (OSStatus %d)", st)
	}
	return nil
}

func keychainAvailable() bool { return C.kcAvailable() != 0 }
