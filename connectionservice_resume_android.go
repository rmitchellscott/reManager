//go:build android

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Android blocks a backgrounded app's network and destroys its TCP sockets about 5 seconds after it leaves the foreground, which a system file picker does, while the process keeps running. The session is checked on return instead of at the next keepalive, and a reconnect that ran out of attempts against the blocked network starts again.
func listenForForegroundReturn(wailsApp *application.App, connection *ConnectionService) {
	wailsApp.Event.OnApplicationEvent(events.Android.ActivityResumed, func(*application.ApplicationEvent) {
		go connection.reconnectAfterForegroundReturn()
	})
}

func (s *ConnectionService) reconnectAfterForegroundReturn() {
	if s.getClient() != nil {
		s.triggerConnectionCheck()
		return
	}
	if s.wakeReconnect() {
		return
	}
	s.mu.Lock()
	target := s.currentConn
	s.mu.Unlock()
	if target == nil || target.deviceID == "" || target.gen != s.connGen.Load() || !s.takeExhaustedReconnect() {
		return
	}
	s.attemptReconnect(*target)
}
