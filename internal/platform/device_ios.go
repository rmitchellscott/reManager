//go:build ios

package platform

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework UIKit
#include <stdbool.h>
#import <UIKit/UIKit.h>

static bool isIPad(void) {
	__block bool pad = false;
	void (^readIdiom)(void) = ^{
		pad = [UIDevice currentDevice].userInterfaceIdiom == UIUserInterfaceIdiomPad;
	};
	if ([NSThread isMainThread]) {
		readIdiom();
	} else {
		dispatch_sync(dispatch_get_main_queue(), readIdiom);
	}
	return pad;
}
*/
import "C"

import "sync"

var isIPad = sync.OnceValue(func() bool { return bool(C.isIPad()) })

func IsIPad() bool { return isIPad() }
