//go:build mas

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation
#import <Foundation/Foundation.h>

static const char *sandboxTempDir(void) {
	return [NSTemporaryDirectory() fileSystemRepresentation];
}
*/
import "C"

import "os"

// The App Sandbox redirects HOME into the container but leaves TMPDIR at the shared /var/folders/.../T/, which it then denies writes to, so every os.CreateTemp("") fails until TMPDIR points at the container's tmp.
func init() {
	if dir := C.GoString(C.sandboxTempDir()); dir != "" {
		os.Setenv("TMPDIR", dir)
	}
}
