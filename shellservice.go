package main

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/crypto/ssh"

	"reManager/internal/debug"
	"reManager/internal/executor"
)

const (
	eventShellOutput  = "shell:output"
	eventShellStopped = "shell:stopped"
)

func init() {
	application.RegisterEvent[string](eventShellOutput)
	application.RegisterEvent[application.Void](eventShellStopped)
}

type ShellService struct {
	*core
	shellSession    *ssh.Session
	shellStdin      io.WriteCloser
	shellMu         sync.Mutex
	shellActive     bool
	demoShellActive bool
	demoShellInput  string
}

func (c *core) runCommandWithTimeout(cmd string, timeout time.Duration) (string, error) {
	client := c.getClient()
	if client == nil {
		return "", fmt.Errorf("not connected")
	}

	debug.Printf("[DEBUG] runCommand creating session: %s\n", cmd[:min(50, len(cmd))])
	c.sessionMu.Lock()
	session, err := client.NewSession()
	c.sessionMu.Unlock()
	if err != nil {
		debug.Printf("[DEBUG] runCommand session error: %v\n", err)
		if isConnectionDeadError(err) {
			go c.triggerConnectionCheck()
		}
		return "", err
	}
	defer session.Close()

	type result struct {
		output []byte
		err    error
	}
	done := make(chan result, 1)
	go func() {
		output, err := session.CombinedOutput(executor.WithDevicePath(cmd))
		done <- result{output, err}
	}()

	select {
	case r := <-done:
		return string(r.output), r.err
	case <-time.After(timeout):
		// Closing alone would leave the command running on the reMarkable.
		session.Signal(ssh.SIGKILL)
		session.Close()
		debug.Printf("[DEBUG] runCommand timed out after %s: %s\n", timeout, cmd[:min(50, len(cmd))])
		return "", fmt.Errorf("command timed out after %s", timeout)
	}
}

// A non-zero exit is the command's answer; any other error means no answer came back, so a status read from it would describe the link, not the tablet.
func isTransportError(err error) bool {
	var exitErr *ssh.ExitError
	return err != nil && !errors.As(err, &exitErr)
}

func (c *core) runCommand(cmd string) (string, error) {
	client := c.getClient()
	if client == nil {
		return "", fmt.Errorf("not connected")
	}

	debug.Printf("[DEBUG] runCommand creating session: %s\n", cmd[:min(50, len(cmd))])
	c.sessionMu.Lock()
	session, err := client.NewSession()
	c.sessionMu.Unlock()
	if err != nil {
		debug.Printf("[DEBUG] runCommand session error: %v\n", err)
		if isConnectionDeadError(err) {
			go c.triggerConnectionCheck()
		}
		return "", err
	}
	defer session.Close()

	debug.Printf("[DEBUG] runCommand executing: %s\n", cmd[:min(50, len(cmd))])
	output, err := session.CombinedOutput(executor.WithDevicePath(cmd))
	debug.Printf("[DEBUG] runCommand done: %s, err: %v\n", cmd[:min(50, len(cmd))], err)
	return string(output), err
}

func (s *ShellService) StartShell(rows, cols int) error {
	setInputAccessoryDisabled(true)
	application.Mobile.SetKeyboardWatch(true)
	s.shellMu.Lock()
	defer s.shellMu.Unlock()

	if s.shellActive {
		return fmt.Errorf("shell already running")
	}

	s.mu.Lock()
	if s.demo {
		s.mu.Unlock()
		s.demoShellActive = true
		s.shellActive = true
		s.greetDemoShell()
		return nil
	}
	client := s.client
	s.mu.Unlock()
	if client == nil {
		return fmt.Errorf("not connected")
	}

	s.sessionMu.Lock()
	session, err := client.NewSession()
	s.sessionMu.Unlock()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	if rows <= 0 {
		rows = 24
	}
	if cols <= 0 {
		cols = 80
	}

	err = session.RequestPty("xterm-256color", rows, cols, ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	})
	if err != nil {
		session.Close()
		return fmt.Errorf("failed to request PTY: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		return fmt.Errorf("failed to get stdout: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		return fmt.Errorf("failed to get stdin: %w", err)
	}

	if err := session.Shell(); err != nil {
		session.Close()
		return fmt.Errorf("failed to start shell: %w", err)
	}

	s.shellSession = session
	s.shellStdin = stdin
	s.shellActive = true

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				s.wails.Event.Emit(eventShellOutput, string(buf[:n]))
			}
			if err != nil {
				break
			}
		}

		s.shellMu.Lock()
		s.shellActive = false
		s.shellSession = nil
		s.shellStdin = nil
		s.shellMu.Unlock()

		s.wails.Event.Emit(eventShellStopped)
	}()

	return nil
}

func (s *ShellService) WriteToShell(data string) error {
	if s.demoShellRunning() {
		s.writeToDemoShell(data)
		return nil
	}
	s.shellMu.Lock()
	stdin := s.shellStdin
	s.shellMu.Unlock()

	if stdin == nil {
		return fmt.Errorf("shell not active")
	}

	_, err := stdin.Write([]byte(data))
	return err
}

func (s *ShellService) ResizeShell(rows, cols int) error {
	if s.demoShellRunning() {
		return nil
	}
	s.shellMu.Lock()
	session := s.shellSession
	s.shellMu.Unlock()

	if session == nil {
		return fmt.Errorf("shell not active")
	}

	return session.WindowChange(rows, cols)
}

func (s *ShellService) StopShell() {
	setInputAccessoryDisabled(false)
	application.Mobile.SetKeyboardWatch(false)
	s.stopDemoShell()
	s.shellMu.Lock()
	session := s.shellSession
	stdin := s.shellStdin
	s.shellMu.Unlock()

	if session != nil {
		session.Signal(ssh.SIGHUP)
	}
	if stdin != nil {
		stdin.Close()
	}
	if session != nil {
		session.Close()
	}
}
