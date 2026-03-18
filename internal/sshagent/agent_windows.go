//go:build windows

package sshagent

import (
	"net"
	"os"

	"github.com/Microsoft/go-winio"
)

func dialAgent(socketPath string) (net.Conn, error) {
	if socketPath != "" {
		if conn, err := net.Dial("unix", socketPath); err == nil {
			return conn, nil
		}
		return winio.DialPipe(socketPath, nil)
	}
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			return conn, nil
		}
	}
	return winio.DialPipe(`\\.\pipe\openssh-ssh-agent`, nil)
}
