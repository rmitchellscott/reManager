//go:build darwin && !ios && !mas

package platform

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation
#include <stdlib.h>
#include <dlfcn.h>
#import <Foundation/Foundation.h>

// SecTranslocateCreateOriginalPathForURL is private, so it is looked up at
// runtime the way LetsMove does rather than linked.
static char *originalPathForTranslocated(const char *path) {
	void *security = dlopen("/System/Library/Frameworks/Security.framework/Security", RTLD_LAZY);
	if (!security) return NULL;
	CFURLRef (*createOriginal)(CFURLRef, CFErrorRef *) = dlsym(security, "SecTranslocateCreateOriginalPathForURL");
	if (!createOriginal) return NULL;
	NSURL *translocated = [NSURL fileURLWithPath:[NSString stringWithUTF8String:path]];
	CFURLRef original = createOriginal((__bridge CFURLRef)translocated, NULL);
	if (!original) return NULL;
	char *result = strdup([[(__bridge_transfer NSURL *)original path] fileSystemRepresentation]);
	return result;
}

static int moveToTrash(const char *path) {
	NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:path]];
	return [[NSFileManager defaultManager] trashItemAtURL:url resultingItemURL:nil error:nil] ? 1 : 0;
}
*/
import "C"

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unsafe"
)

func runningBundlePath() (string, bool) {
	executable, err := os.Executable()
	if err != nil {
		return "", false
	}
	bundleEnd := strings.Index(executable, ".app/Contents/MacOS/")
	if bundleEnd < 0 {
		return "", false
	}
	return executable[:bundleEnd+len(".app")], true
}

// Gatekeeper translocates a quarantined bundle launched from Downloads to a
// random read-only path under /private/var/folders, which also reads as outside.
func IsOutsideApplicationsFolder() bool {
	bundlePath, ok := runningBundlePath()
	if !ok {
		return false
	}

	applicationsFolders := []string{"/Applications/"}
	if home, err := os.UserHomeDir(); err == nil {
		applicationsFolders = append(applicationsFolders, filepath.Join(home, "Applications")+"/")
	}
	for _, folder := range applicationsFolders {
		if strings.HasPrefix(bundlePath, folder) {
			return false
		}
	}
	return true
}

func originalBundlePath(bundlePath string) string {
	if !strings.Contains(bundlePath, "/AppTranslocation/") {
		return bundlePath
	}
	cPath := C.CString(bundlePath)
	defer C.free(unsafe.Pointer(cPath))
	original := C.originalPathForTranslocated(cPath)
	if original == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(original))
	return C.GoString(original)
}

func trash(path string) bool {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	return C.moveToTrash(cPath) == 1
}

// The moved copy only starts once this process exits, so the caller must quit.
func MoveToApplications() error {
	bundlePath, ok := runningBundlePath()
	if !ok {
		return fmt.Errorf("not running from an app bundle")
	}
	destination := filepath.Join("/Applications", filepath.Base(bundlePath))

	if _, err := os.Stat(destination); err == nil && !trash(destination) {
		return fmt.Errorf("could not move the existing %s to the Trash", destination)
	}
	if output, err := exec.Command("/usr/bin/ditto", bundlePath, destination).CombinedOutput(); err != nil {
		return fmt.Errorf("copy to %s: %v: %s", destination, err, strings.TrimSpace(string(output)))
	}
	// A bundle copied by anything but Finder keeps its quarantine flag, and
	// Gatekeeper would translocate it again on the next launch.
	_ = exec.Command("/usr/bin/xattr", "-dr", "com.apple.quarantine", destination).Run()

	if original := originalBundlePath(bundlePath); original != "" {
		trash(original)
	}

	// SingleInstance hands a second launch to the running process, so the moved
	// copy has to wait until this one is gone.
	relaunch := exec.Command("/bin/sh", "-c", `while kill -0 "$1" 2>/dev/null; do sleep 0.2; done; /usr/bin/open "$2"`,
		"relaunch", fmt.Sprint(os.Getpid()), destination)
	return relaunch.Start()
}
