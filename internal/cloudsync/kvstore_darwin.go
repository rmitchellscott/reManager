//go:build darwin

package cloudsync

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation
#include <stdlib.h>

int kvOpen(void);
char *kvGet(const char *key);
void kvSet(const char *key, const char *value);
*/
import "C"

import (
	"strings"
	"sync"
	"unsafe"

	"reManager/internal/debug"
)

const quotaViolationChange = 2

var (
	externalChangeMu sync.Mutex
	onExternalChange func(keys []string)
)

//export cloudsyncExternalChange
func cloudsyncExternalChange(reason C.int, keys *C.char) {
	if int(reason) == quotaViolationChange {
		debug.Println("[DEBUG] cloudsync: iCloud key-value store quota exceeded")
	}
	externalChangeMu.Lock()
	handler := onExternalChange
	externalChangeMu.Unlock()
	if handler == nil {
		return
	}
	joined := C.GoString(keys)
	if joined == "" {
		return
	}
	handler(strings.Split(joined, "\n"))
}

type iCloudStore struct{}

// Open reports false when the build carries no iCloud entitlement, such as an ad-hoc signed development build, and the app then runs without sync.
func Open(onChange func(keys []string)) (KeyValueStore, bool) {
	externalChangeMu.Lock()
	onExternalChange = onChange
	externalChangeMu.Unlock()
	if C.kvOpen() == 0 {
		return nil, false
	}
	return iCloudStore{}, true
}

func (iCloudStore) Get(key string) (string, bool) {
	ck := C.CString(key)
	defer C.free(unsafe.Pointer(ck))
	value := C.kvGet(ck)
	if value == nil {
		return "", false
	}
	defer C.free(unsafe.Pointer(value))
	return C.GoString(value), true
}

func (iCloudStore) Set(key, value string) {
	ck, cv := C.CString(key), C.CString(value)
	defer C.free(unsafe.Pointer(ck))
	defer C.free(unsafe.Pointer(cv))
	C.kvSet(ck, cv)
}
