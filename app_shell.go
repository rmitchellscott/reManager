package main

import (
	"fmt"
	"io"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"

	"reManager/internal/debug"
)

func (a *App) RunCommand(cmd string) string {
	a.mu.Lock()
	defer a.mu.Unlock()

	output, err := a.runCommand(cmd)
	if err != nil {
		return fmt.Sprintf("Error: %v\n%s", err, output)
	}
	return output
}

func (a *App) runCommand(cmd string) (string, error) {
	if a.client == nil {
		return "", fmt.Errorf("not connected")
	}

	debug.Printf("[DEBUG] runCommand creating session: %s\n", cmd[:min(50, len(cmd))])
	session, err := a.client.NewSession()
	if err != nil {
		debug.Printf("[DEBUG] runCommand session error: %v\n", err)
		if isConnectionDeadError(err) {
			go a.triggerConnectionCheck()
		}
		return "", err
	}
	defer session.Close()

	debug.Printf("[DEBUG] runCommand executing: %s\n", cmd[:min(50, len(cmd))])
	output, err := session.CombinedOutput(cmd)
	debug.Printf("[DEBUG] runCommand done: %s, err: %v\n", cmd[:min(50, len(cmd))], err)
	return string(output), err
}

func (a *App) RunCommandWithOutput(cmd string, requiresPTY bool) {
	debug.Println("[DEBUG] RunCommandWithOutput called:", cmd[:min(50, len(cmd))], "requiresPTY:", requiresPTY)
	cmdLog := a.operationLog
	ownLog := cmdLog == nil
	if ownLog && a.logger != nil {
		cmdLog = a.logger.StartCommandLog(a.connectedDeviceID, cmd)
	}
	if !ownLog {
		cmdLog.Write(fmt.Sprintf("\n$ %s\n", cmd))
	}
	go func() {
		defer func() {
			if ownLog && cmdLog != nil {
				cmdLog.Close()
			}
		}()
		a.mu.Lock()

		if a.client == nil {
			a.mu.Unlock()
			debug.Println("[DEBUG] Not connected, emitting error")
			runtime.EventsEmit(a.ctx, "command:output", "Error: not connected\n")
			runtime.EventsEmit(a.ctx, "command:done", false)
			return
		}

		session, err := a.client.NewSession()
		if err != nil {
			a.mu.Unlock()
			debug.Println("[DEBUG] Session error:", err)
			if isConnectionDeadError(err) {
				go a.triggerConnectionCheck()
			}
			runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Error: %v\n", err))
			runtime.EventsEmit(a.ctx, "command:done", false)
			return
		}

		if requiresPTY {
			err = session.RequestPty("xterm-256color", 40, 120, ssh.TerminalModes{
				ssh.ECHO:          1,
				ssh.TTY_OP_ISPEED: 14400,
				ssh.TTY_OP_OSPEED: 14400,
			})
			if err != nil {
				a.mu.Unlock()
				debug.Println("[DEBUG] PTY request error:", err)
				runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Error requesting PTY: %v\n", err))
				runtime.EventsEmit(a.ctx, "command:done", false)
				return
			}
			debug.Println("[DEBUG] PTY allocated successfully")
		}

		a.commandSession = session
		a.mu.Unlock()

		defer func() {
			session.Close()
			a.mu.Lock()
			a.commandSession = nil
			a.commandStdin = nil
			a.mu.Unlock()
		}()

		stdout, err := session.StdoutPipe()
		if err != nil {
			runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Error: %v\n", err))
			runtime.EventsEmit(a.ctx, "command:done", false)
			return
		}

		stderr, err := session.StderrPipe()
		if err != nil {
			runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Error: %v\n", err))
			runtime.EventsEmit(a.ctx, "command:done", false)
			return
		}

		stdin, err := session.StdinPipe()
		if err != nil {
			runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Error: %v\n", err))
			runtime.EventsEmit(a.ctx, "command:done", false)
			return
		}

		a.mu.Lock()
		a.commandStdin = stdin
		a.mu.Unlock()

		debug.Println("[DEBUG] Starting command")
		if err := session.Start(cmd); err != nil {
			debug.Println("[DEBUG] Start error:", err)
			runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Error: %v\n", err))
			runtime.EventsEmit(a.ctx, "command:done", false)
			return
		}

		if !requiresPTY {
			stdin.Close()
		}

		go func() {
			buf := make([]byte, 1024)
			for {
				n, err := stdout.Read(buf)
				if n > 0 {
					debug.Printf("[DEBUG] stdout: %d bytes\n", n)
					chunk := string(buf[:n])
					runtime.EventsEmit(a.ctx, "command:output", chunk)
					cmdLog.Write(chunk)
				}
				if err == io.EOF {
					break
				}
				if err != nil {
					break
				}
			}
		}()

		go func() {
			buf := make([]byte, 1024)
			for {
				n, err := stderr.Read(buf)
				if n > 0 {
					debug.Printf("[DEBUG] stderr: %d bytes\n", n)
					chunk := string(buf[:n])
					runtime.EventsEmit(a.ctx, "command:output", chunk)
					cmdLog.Write(chunk)
				}
				if err == io.EOF {
					break
				}
				if err != nil {
					break
				}
			}
		}()

		err = session.Wait()
		if ownLog {
			cmdLog.WriteExitCode(err)
		}
		debug.Println("[DEBUG] Command done, success:", err == nil)
		runtime.EventsEmit(a.ctx, "command:done", err == nil)
	}()
}

func (a *App) StopCommand() {
	a.mu.Lock()
	stdin := a.commandStdin
	a.mu.Unlock()

	if stdin != nil {
		debug.Println("[DEBUG] Sending Ctrl+C (0x03) to stdin")
		_, err := stdin.Write([]byte{0x03})
		if err != nil {
			debug.Printf("[DEBUG] Error writing Ctrl+C to stdin: %v\n", err)
		} else {
			debug.Println("[DEBUG] Ctrl+C sent successfully")
		}
	} else {
		debug.Println("[DEBUG] No stdin available to send Ctrl+C")
	}
}

func (a *App) StartShell(rows, cols int) error {
	a.shellMu.Lock()
	defer a.shellMu.Unlock()

	if a.shellActive {
		return fmt.Errorf("shell already running")
	}

	a.mu.Lock()
	if a.client == nil {
		a.mu.Unlock()
		return fmt.Errorf("not connected")
	}

	session, err := a.client.NewSession()
	if err != nil {
		a.mu.Unlock()
		return fmt.Errorf("failed to create session: %w", err)
	}
	a.mu.Unlock()

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

	a.shellSession = session
	a.shellStdin = stdin
	a.shellActive = true

	runtime.EventsEmit(a.ctx, "shell:started")

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				runtime.EventsEmit(a.ctx, "shell:output", string(buf[:n]))
			}
			if err != nil {
				break
			}
		}

		a.shellMu.Lock()
		a.shellActive = false
		a.shellSession = nil
		a.shellStdin = nil
		a.shellMu.Unlock()

		runtime.EventsEmit(a.ctx, "shell:stopped")
	}()

	return nil
}

func (a *App) WriteToShell(data string) error {
	a.shellMu.Lock()
	stdin := a.shellStdin
	a.shellMu.Unlock()

	if stdin == nil {
		return fmt.Errorf("shell not active")
	}

	_, err := stdin.Write([]byte(data))
	return err
}

func (a *App) ResizeShell(rows, cols int) error {
	a.shellMu.Lock()
	session := a.shellSession
	a.shellMu.Unlock()

	if session == nil {
		return fmt.Errorf("shell not active")
	}

	return session.WindowChange(rows, cols)
}

func (a *App) StopShell() {
	a.shellMu.Lock()
	session := a.shellSession
	stdin := a.shellStdin
	a.shellMu.Unlock()

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

func (a *App) IsShellActive() bool {
	a.shellMu.Lock()
	defer a.shellMu.Unlock()
	return a.shellActive
}

