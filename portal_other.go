//go:build !linux || android

package main

import "errors"

var errPortalUnavailable = errors.New("xdg desktop portal is only available on Linux")

func portalOpenFiles(string, portalOpenOptions) ([]string, error) {
	return nil, errPortalUnavailable
}

func portalSaveFile(string, string, string) ([]string, error) {
	return nil, errPortalUnavailable
}

func portalOpenDirectory(uintptr) error {
	return errPortalUnavailable
}

func watchSystemColorScheme(func(ColorScheme)) {}

func systemColorScheme() (ColorScheme, error) {
	return ColorSchemeUnknown, errPortalUnavailable
}
