//go:build !android

package main

/*
#include <locale.h>
#include <stdlib.h>
*/
import "C"

import (
	"os"
	"unsafe"
)

var localeVariables = []string{
	"LC_ALL",
	"LC_CTYPE",
	"LC_NUMERIC",
	"LC_TIME",
	"LC_COLLATE",
	"LC_MONETARY",
	"LC_MESSAGES",
	"LC_PAPER",
	"LC_NAME",
	"LC_ADDRESS",
	"LC_TELEPHONE",
	"LC_MEASUREMENT",
	"LC_IDENTIFICATION",
}

// Flatpak installs runtime locales only for the user's language, so a host with LC_TIME etc. set to another language fails setlocale; glibc then drops every category to the ASCII "C" locale, and WebKit's flatpak-spawn aborts the app on non-UTF-8 input.
func init() {
	if localeLoads("") {
		return
	}
	for _, variable := range localeVariables {
		if value := os.Getenv(variable); value != "" && !localeLoads(value) {
			os.Unsetenv(variable)
		}
	}
	if lang := os.Getenv("LANG"); lang != "" && !localeLoads(lang) {
		os.Setenv("LANG", "C.UTF-8")
	}
}

func localeLoads(name string) bool {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	return C.setlocale(C.LC_ALL, cName) != nil
}
