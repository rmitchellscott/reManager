//go:build mas

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>
#include <stdlib.h>

static void workspaceOpenURL(const char *raw) {
	NSURL *url = [NSURL URLWithString:[NSString stringWithUTF8String:raw]];
	if (url == nil) {
		return;
	}
	dispatch_async(dispatch_get_main_queue(), ^{
		[[NSWorkspace sharedWorkspace] openURL:url];
	});
}

static void workspaceRevealPath(const char *path) {
	NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:path]];
	dispatch_async(dispatch_get_main_queue(), ^{
		[[NSWorkspace sharedWorkspace] activateFileViewerSelectingURLs:@[url]];
	});
}
*/
import "C"

import "unsafe"

// Wails and open-golang both launch /usr/bin/open, a child process the App Sandbox confines like the app itself, so the Mac App Store build asks Launch Services through NSWorkspace instead.
func openURLInBrowser(rawURL string) error {
	cURL := C.CString(rawURL)
	defer C.free(unsafe.Pointer(cURL))
	C.workspaceOpenURL(cURL)
	return nil
}

func revealInFileManager(path string) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	C.workspaceRevealPath(cPath)
}
