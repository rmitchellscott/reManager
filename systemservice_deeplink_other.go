//go:build !android && !ios

package main

import "github.com/wailsapp/wails/v3/pkg/application"

func listenForPlatformDeepLinks(*application.App, *SystemService) {}
