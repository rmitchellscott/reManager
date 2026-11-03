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

func (c *core) beginInstall() bool {
	c.installMu.Lock()
	defer c.installMu.Unlock()
	if c.installActive {
		return false
	}
	c.installActive = true
	return true
}

func (c *core) endInstall() {
	c.installMu.Lock()
	c.installActive = false
	c.installSession = nil
	c.installMu.Unlock()
}

func (c *core) isInstallActive() bool {
	c.installMu.Lock()
	defer c.installMu.Unlock()
	return c.installActive
}

func (c *core) setInstallSession(s *installSession) {
	c.installMu.Lock()
	c.installSession = s
	c.installMu.Unlock()
}
