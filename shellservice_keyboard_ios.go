//go:build ios

package main

/*
#include <stdbool.h>
extern void ios_set_disable_input_accessory(bool disabled);
*/
import "C"

// IOSOptions.DisableInputAccessoryView is read once at startup, but the flag behind it is re-checked each time a field takes focus, which is what makes per-session suppression possible.
func setInputAccessoryDisabled(disabled bool) {
	C.ios_set_disable_input_accessory(C.bool(disabled))
}
