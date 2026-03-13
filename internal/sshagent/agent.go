package sshagent

import (
	"fmt"
	"net"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func IsAvailable(socketPath string) bool {
	conn, err := dialAgent(socketPath)
	if err != nil {
		return false
	}
	defer conn.Close()

	keys, err := agent.NewClient(conn).List()
	if err != nil {
		return false
	}
	return len(keys) > 0
}

func Connect(socketPath string) (net.Conn, ssh.AuthMethod, error) {
	conn, err := dialAgent(socketPath)
	if err != nil {
		return nil, nil, fmt.Errorf("SSH agent is not running: %w", err)
	}

	agentClient := agent.NewClient(conn)

	keys, err := agentClient.List()
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("failed to list agent keys: %w", err)
	}
	if len(keys) == 0 {
		conn.Close()
		return nil, nil, fmt.Errorf("SSH agent has no keys loaded. Add keys with ssh-add.")
	}

	return conn, ssh.PublicKeysCallback(agentClient.Signers), nil
}
