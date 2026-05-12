package executor

import (
	"fmt"
	"io"
	"sync"

	"golang.org/x/crypto/ssh"

	"reManager/internal/component"
)

type SSHExecutor struct {
	client  *ssh.Client
	mu      sync.Mutex
	session *ssh.Session
}

func NewSSHExecutor(client *ssh.Client) *SSHExecutor {
	return &SSHExecutor{client: client}
}

func (e *SSHExecutor) Execute(commands []component.CommandResult) error {
	for _, cmd := range commands {
		_, err := e.ExecuteWithOutput(cmd.Script)
		if err != nil {
			return fmt.Errorf("command failed: %s: %w", cmd.Description, err)
		}
	}
	return nil
}

func (e *SSHExecutor) ExecuteWithOutput(cmd string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.client == nil {
		return "", fmt.Errorf("not connected")
	}

	session, err := e.client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	output, err := session.CombinedOutput(cmd)
	return string(output), err
}

func (e *SSHExecutor) ExecuteStreaming(cmd string, onOutput func(line string)) error {
	e.mu.Lock()
	if e.client == nil {
		e.mu.Unlock()
		return fmt.Errorf("not connected")
	}

	session, err := e.client.NewSession()
	if err != nil {
		e.mu.Unlock()
		return err
	}
	e.session = session
	e.mu.Unlock()

	defer func() {
		session.Close()
		e.mu.Lock()
		e.session = nil
		e.mu.Unlock()
	}()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		return err
	}

	if err := session.RequestPty("xterm", 40, 80, ssh.TerminalModes{}); err != nil {
		return err
	}

	if err := session.Start(cmd); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)

	readOutput := func(r io.Reader) {
		defer wg.Done()
		buf := make([]byte, 1024)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				onOutput(string(buf[:n]))
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
		}
	}

	go readOutput(stdout)
	go readOutput(stderr)

	err = session.Wait()
	wg.Wait()
	return err
}

func (e *SSHExecutor) StopCurrentCommand() {
	e.mu.Lock()
	session := e.session
	e.mu.Unlock()

	if session != nil {
		session.Close()
	}
}

