//go:build ios

package main

import "github.com/wailsapp/wails/v3/pkg/application"

func setStatusBarContentStyle(style string) {
	application.IOS.SetStatusBar(`{"style":"` + style + `"}`)
}
