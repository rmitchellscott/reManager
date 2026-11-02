//go:build windows && !msstore

package main

import (
	"os"

	"golang.org/x/sys/windows/registry"

	"reManager/internal/debug"
)

// The zipped exe has no installer to register the scheme, so every launch points the per-user scheme at whichever exe is running (the Store build's package manifest owns registration).
func registerDeepLinkScheme() {
	executable, err := os.Executable()
	if err != nil {
		debug.Println("[DEBUG] deep link scheme not registered:", err)
		return
	}
	schemeKeyPath := `Software\Classes\` + deepLinkScheme
	values := []struct{ path, name, data string }{
		{schemeKeyPath, "", "reManager package link"},
		{schemeKeyPath, "URL Protocol", ""},
		{schemeKeyPath + `\DefaultIcon`, "", executable + ",0"},
		{schemeKeyPath + `\shell\open\command`, "", `"` + executable + `" "%1"`},
	}
	for _, value := range values {
		key, _, err := registry.CreateKey(registry.CURRENT_USER, value.path, registry.SET_VALUE)
		if err != nil {
			debug.Println("[DEBUG] deep link scheme not registered:", err)
			return
		}
		err = key.SetStringValue(value.name, value.data)
		key.Close()
		if err != nil {
			debug.Println("[DEBUG] deep link scheme not registered:", err)
			return
		}
	}
}
