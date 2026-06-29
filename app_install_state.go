package main

import (
	"sync"

	"golang.org/x/crypto/ssh"
)

type installSession struct {
	deviceID        string
	targetPartition int
	version         string
	mode            string

	mu       sync.Mutex
	connLost bool
	resume   chan *ssh.Client
}

func (a *App) beginInstall() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.installActive {
		return false
	}
	a.installActive = true
	return true
}

func (a *App) endInstall() {
	a.mu.Lock()
	a.installActive = false
	a.installSession = nil
	a.mu.Unlock()
}

func (a *App) isInstallActive() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.installActive
}

func (a *App) setInstallSession(s *installSession) {
	a.mu.Lock()
	a.installSession = s
	a.mu.Unlock()
}

func (a *App) currentInstallSession() *installSession {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.installSession
}

func (a *App) clearInstallSession() {
	a.mu.Lock()
	a.installSession = nil
	a.mu.Unlock()
}
