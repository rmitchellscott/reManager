//go:build !windows

package sshagent

import (
	"fmt"
	"net"
	"os"
)

func dialAgent(socketPath string) (net.Conn, error) {
	if socketPath != "" {
		return net.Dial("unix", socketPath)
	}
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, fmt.Errorf("SSH_AUTH_SOCK not set")
	}
	return net.Dial("unix", sock)
}
