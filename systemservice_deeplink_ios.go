//go:build ios

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Wails reports no launch URL on iOS, so the app delegate holds the link and Go takes it: once at start, and on each ios:deeplink while running.
func listenForPlatformDeepLinks(wailsApp *application.App, app *SystemService) {
	takePendingDeepLink := func() {
		if link := application.IOS.TakeDeepLink(); link != "" {
			app.receiveDeepLink(link)
		}
	}
	wailsApp.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		takePendingDeepLink()
	})
	wailsApp.Event.On("ios:deeplink", func(*application.CustomEvent) {
		takePendingDeepLink()
	})
}
