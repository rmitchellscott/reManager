//go:build !mas

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/skratchdot/open-golang/open"
	"github.com/wailsapp/wails/v3/pkg/application"

	"reManager/internal/platform"
)

func openURLInBrowser(rawURL string) error {
	app := application.Get()
	if app == nil {
		return fmt.Errorf("application not running")
	}
	return app.Browser.OpenURL(rawURL)
}

func revealInFileManager(path string) {
	dir := filepath.Dir(path)

	if platform.IsRunningInFlatpak() {
		file, err := os.Open(dir)
		if err != nil {
			open.Start(dir)
			return
		}
		defer file.Close()

		if err := portalOpenDirectory(file.Fd()); err != nil {
			open.Start(dir)
		}
		return
	}

	open.Start(dir)
}
