#import <Foundation/Foundation.h>
#include <stdlib.h>
#include <string.h>

extern void cloudsyncExternalChange(int reason, char *keys);

int kvOpen(void) {
	@autoreleasepool {
		NSUbiquitousKeyValueStore *store = [NSUbiquitousKeyValueStore defaultStore];
		[[NSNotificationCenter defaultCenter]
			addObserverForName:NSUbiquitousKeyValueStoreDidChangeExternallyNotification
			            object:store
			             queue:nil
			        usingBlock:^(NSNotification *note) {
				NSNumber *reason = note.userInfo[NSUbiquitousKeyValueStoreChangeReasonKey];
				NSArray *keys = note.userInfo[NSUbiquitousKeyValueStoreChangedKeysKey];
				NSString *joined = [keys componentsJoinedByString:@"\n"] ?: @"";
				cloudsyncExternalChange(reason ? reason.intValue : -1, (char *)joined.UTF8String);
			}];
		// synchronize reports NO when the build lacks the key-value store
		// entitlement, which is how an ad-hoc signed build opts out of sync.
		return [store synchronize] ? 1 : 0;
	}
}

char *kvGet(const char *key) {
	@autoreleasepool {
		NSString *value = [[NSUbiquitousKeyValueStore defaultStore] stringForKey:[NSString stringWithUTF8String:key]];
		return value ? strdup(value.UTF8String) : NULL;
	}
}

void kvSet(const char *key, const char *value) {
	@autoreleasepool {
		[[NSUbiquitousKeyValueStore defaultStore] setString:[NSString stringWithUTF8String:value]
		                                            forKey:[NSString stringWithUTF8String:key]];
	}
}
