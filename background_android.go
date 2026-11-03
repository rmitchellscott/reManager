//go:build android

package main

import (
	"encoding/json"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func showNativeBackgroundWork(title, subtitle string, fraction float64) {
	percent := -1
	if fraction >= 0 {
		percent = int(fraction * 100)
	}
	payload, err := json.Marshal(map[string]any{"title": title, "text": subtitle, "percent": percent})
	if err != nil {
		return
	}
	application.Android.StartForegroundService(string(payload))
}

func endNativeBackgroundWorkFailed(title, message string) {
	payload, err := json.Marshal(map[string]any{"title": title, "text": message, "failed": true})
	if err != nil {
		return
	}
	application.Android.StartForegroundService(string(payload))
}

func endNativeBackgroundWork() {
	application.Android.StopForegroundService()
}

func notifyNeedsAttention(title, body string) {
	payload, err := json.Marshal(map[string]any{"title": title, "body": body, "onlyInBackground": true})
	if err != nil {
		return
	}
	application.Android.Notify(string(payload))
}
