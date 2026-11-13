package tablettest

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/user"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

const exitUnknown = 255

// Each session runs the ssh binary with upstream, such as []string{"rm"}, so ~/.ssh/config applies as it does in a terminal. Interactive shells and SFTP writes pass through unrecorded.
func NewRecordServer(upstream []string, cassette *Cassette) (*Server, error) {
	// ssh reads its config from HOME, which the integration suite points at a sandbox.
	account, err := user.Current()
	if err != nil {
		return nil, err
	}
	controlDir, err := os.MkdirTemp("/tmp", "rmrec")
	if err != nil {
		return nil, err
	}
	base := []string{
		"-o", "BatchMode=yes",
		// ssh warnings such as "Permanently added ... to the list of known hosts" go to stderr, which the session relays and the shell noise probe rejects.
		"-o", "LogLevel=ERROR",
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + controlDir + "/%C",
		"-o", "ControlPersist=30",
	}
	server, err := newServer(func() sessionBackend {
		return &recordSession{home: account.HomeDir, base: base, upstream: upstream, cassette: cassette}
	})
	if err != nil {
		os.RemoveAll(controlDir)
		return nil, err
	}
	watcher := &outageWatcher{home: account.HomeDir, upstream: upstream, server: server, cassette: cassette, stop: make(chan struct{})}
	go watcher.run()
	server.onClose = func() {
		watcher.close()
		stop := exec.Command("ssh", append(append(append([]string{}, base...), "-O", "exit"), upstream...)...)
		stop.Env = append(os.Environ(), "HOME="+account.HomeDir)
		stop.Run()
		os.RemoveAll(controlDir)
	}
	return server, nil
}

// outageWatcher holds a connection of its own to the tablet; when the tablet reboots, that connection ends and the server goes down with it until the tablet answers again, so the app sees the same outage it would see direct.
type outageWatcher struct {
	home     string
	upstream []string
	server   *Server
	cassette *Cassette
	stop     chan struct{}
	mu       sync.Mutex
	probe    *exec.Cmd
}

func (w *outageWatcher) ssh(options []string, remote ...string) *exec.Cmd {
	args := append([]string{"-o", "BatchMode=yes", "-o", "LogLevel=ERROR", "-o", "ControlMaster=no", "-o", "ControlPath=none"}, options...)
	cmd := exec.Command("ssh", append(append(args, w.upstream...), remote...)...)
	cmd.Env = append(os.Environ(), "HOME="+w.home)
	return cmd
}

func (w *outageWatcher) stopped() bool {
	select {
	case <-w.stop:
		return true
	default:
		return false
	}
}

func (w *outageWatcher) run() {
	for !w.stopped() {
		probe := w.ssh([]string{"-T", "-o", "ServerAliveInterval=1", "-o", "ServerAliveCountMax=3"}, "while sleep 60; do :; done")
		started := time.Now()
		w.mu.Lock()
		if w.stopped() {
			w.mu.Unlock()
			return
		}
		err := probe.Start()
		w.probe = probe
		w.mu.Unlock()
		if err == nil {
			probe.Wait()
		}
		if w.stopped() {
			return
		}
		if time.Since(started) < time.Second {
			// Never connected, so there is no outage to record.
			time.Sleep(time.Second)
			continue
		}
		w.cassette.markDrop()
		w.server.goDown()
		for !w.stopped() && w.ssh([]string{"-o", "ConnectTimeout=3"}, "true").Run() != nil {
			time.Sleep(2 * time.Second)
		}
		w.server.comeUp()
	}
}

func (w *outageWatcher) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	close(w.stop)
	if w.probe != nil && w.probe.Process != nil {
		w.probe.Process.Kill()
	}
}

type recordSession struct {
	home     string
	base     []string
	upstream []string
	cassette *Cassette
	ptyTerm  string
	mu       sync.Mutex
	process  *os.Process
}

func (s *recordSession) pty(term string, _, _ uint32) bool {
	s.ptyTerm = term
	return true
}

func (s *recordSession) command(flags []string, remote ...string) *exec.Cmd {
	args := append(append([]string{}, s.base...), flags...)
	args = append(append(args, s.upstream...), remote...)
	cmd := exec.Command("ssh", args...)
	cmd.Env = append(os.Environ(), "HOME="+s.home)
	if s.ptyTerm != "" {
		cmd.Env = append(cmd.Env, "TERM="+s.ptyTerm)
	}
	return cmd
}

func (s *recordSession) ttyFlag() string {
	if s.ptyTerm != "" {
		return "-tt"
	}
	return "-T"
}

// Stdin is copied by hand rather than set as Cmd.Stdin, because Wait would then block until the client closed its side, which a client reading output never does.
func (s *recordSession) start(cmd *exec.Cmd, channel ssh.Channel, stdout, stderr io.Writer) error {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		io.Copy(stdin, channel)
		stdin.Close()
	}()
	s.mu.Lock()
	s.process = cmd.Process
	s.mu.Unlock()
	return nil
}

func (s *recordSession) exec(channel ssh.Channel, command string) (int, string) {
	exchange := s.cassette.reserve(command)
	var stdout, stderr bytes.Buffer
	cmd := s.command([]string{s.ttyFlag()}, command)
	if err := s.start(cmd, channel, io.MultiWriter(channel, &stdout), io.MultiWriter(channel.Stderr(), &stderr)); err != nil {
		io.WriteString(channel.Stderr(), err.Error()+"\n")
		s.cassette.finish(exchange, "", err.Error()+"\n", exitUnknown, "")
		return exitUnknown, ""
	}
	exit := exitOf(cmd.Wait())
	s.cassette.finish(exchange, stdout.String(), stderr.String(), exit, "")
	return exit, ""
}

func exitOf(err error) int {
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exitErr) && exitErr.ExitCode() >= 0:
		return exitErr.ExitCode()
	default:
		return exitUnknown
	}
}

func (s *recordSession) subsystem(channel ssh.Channel, name string) bool {
	if name != "sftp" {
		return false
	}
	cmd := s.command([]string{"-T", "-s"}, name)
	toTablet, err := cmd.StdinPipe()
	if err != nil {
		return false
	}
	fromTablet, err := cmd.StdoutPipe()
	if err != nil {
		return false
	}
	if err := cmd.Start(); err != nil {
		return false
	}
	s.mu.Lock()
	s.process = cmd.Process
	s.mu.Unlock()
	defer cmd.Wait()
	defer toTablet.Close()
	client, err := sftp.NewClientPipe(fromTablet, toTablet)
	if err != nil {
		return false
	}
	defer client.Close()
	files := &recordingFiles{client: client, cassette: s.cassette}
	server := sftp.NewRequestServer(channel, files.handlers())
	server.Serve()
	server.Close()
	return true
}

func (s *recordSession) shell(channel ssh.Channel) bool {
	cmd := s.command([]string{s.ttyFlag()})
	if s.start(cmd, channel, channel, channel.Stderr()) != nil {
		return false
	}
	cmd.Wait()
	return true
}

func (s *recordSession) signal(string) { s.kill() }

func (s *recordSession) kill() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.process != nil {
		s.process.Kill()
	}
}

func (s *recordSession) windowChange(uint32, uint32) {}

func (s *recordSession) close() { s.kill() }
