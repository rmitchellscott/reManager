package main

import (
	"fmt"
	"net/url"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func isBrowsableURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	switch parsed.Scheme {
	case "https", "http":
		return parsed.Host != ""
	case "mailto":
		return parsed.Opaque != ""
	}
	return false
}

// Wails' Browser.OpenURL shells out to `open` on darwin, which on iOS is a forbidden fork/exec that fails silently, so application.Mobile routes to UIApplication.openURL instead.
func (s *SystemService) OpenExternalURL(rawURL string) error {
	if !isBrowsableURL(rawURL) {
		return fmt.Errorf("%q is not a web link", rawURL)
	}
	if runtime.GOOS == "ios" || runtime.GOOS == "android" {
		application.Mobile.OpenURL(rawURL)
		return nil
	}
	return openURLInBrowser(rawURL)
}
