//go:build android

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Wails reports no launch URL on Android, so the activity holds the link and Go takes it: once at start for a cold launch, and on each android:deeplink while running.
func listenForPlatformDeepLinks(wailsApp *application.App, app *SystemService) {
	takePendingDeepLink := func() {
		if link := application.Android.TakeDeepLink(); link != "" {
			app.receiveDeepLink(link)
		}
	}
	wailsApp.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		takePendingDeepLink()
	})
	wailsApp.Event.On("android:deeplink", func(*application.CustomEvent) {
		takePendingDeepLink()
	})
}
