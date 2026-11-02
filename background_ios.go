//go:build ios

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework BackgroundTasks -framework UserNotifications
#include <stdlib.h>
extern void ios_show_background_work(const char *title, const char *subtitle, double fraction);
extern void ios_end_background_work(void);
extern void ios_fail_background_work(const char *title, const char *message);
extern void ios_notify_needs_attention(const char *title, const char *body);
*/
import "C"

import "unsafe"

func showNativeBackgroundWork(title, subtitle string, fraction float64) {
	cTitle := C.CString(title)
	cSubtitle := C.CString(subtitle)
	defer C.free(unsafe.Pointer(cTitle))
	defer C.free(unsafe.Pointer(cSubtitle))
	C.ios_show_background_work(cTitle, cSubtitle, C.double(fraction))
}

func endNativeBackgroundWork() {
	C.ios_end_background_work()
}

func endNativeBackgroundWorkFailed(title, message string) {
	cTitle := C.CString(title)
	cMessage := C.CString(message)
	defer C.free(unsafe.Pointer(cTitle))
	defer C.free(unsafe.Pointer(cMessage))
	C.ios_fail_background_work(cTitle, cMessage)
}

func notifyNeedsAttention(title, body string) {
	cTitle := C.CString(title)
	cBody := C.CString(body)
	defer C.free(unsafe.Pointer(cTitle))
	defer C.free(unsafe.Pointer(cBody))
	C.ios_notify_needs_attention(cTitle, cBody)
}
