package tablettest

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func dial(t *testing.T, addr string) *ssh.Client {
	t.Helper()
	client, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.Password(Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

type result struct {
	stdout, stderr string
	exit           int
	signal         string
}

func run(t *testing.T, client *ssh.Client, command string) result {
	t.Helper()
	got, err := runCommand(client, command)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func runCommand(client *ssh.Client, command string) (result, error) {
	session, err := client.NewSession()
	if err != nil {
		return result{}, err
	}
	defer session.Close()
	var stdout, stderr strings.Builder
	session.Stdout, session.Stderr = &stdout, &stderr
	err = session.Run(command)
	var exitErr *ssh.ExitError
	switch {
	case err == nil:
		return result{stdout.String(), stderr.String(), 0, ""}, nil
	case errors.As(err, &exitErr):
		return result{stdout.String(), stderr.String(), exitErr.ExitStatus(), exitErr.Signal()}, nil
	default:
		return result{}, err
	}
}

func tablet() *Cassette {
	return &Cassette{Exchanges: []*Exchange{
		{Command: "cat /sys/devices/soc0/machine", Stdout: "reMarkable Chiappa\n"},
		{Command: "vellum list", Stdout: "xovi\n"},
		{Command: "vellum list", Stdout: "xovi\nappload\n"},
		{Command: "vellum add nope", Stderr: "ERROR: unable to select packages\n", Exit: 1},
		{Command: "sleep 60", Signal: "TERM"},
		{Command: "rm -rf /tmp/remanager-<uuid>"},
		{Command: "export PATH=/home/root/.vellum/bin:/usr/bin:$PATH; systemctl is-active xochitl", Stdout: "active\n"},
	}}
}

func TestReplayAnswersInRecordedOrderAndRepeatsTheLast(t *testing.T) {
	var mu sync.Mutex
	var missed []string
	server, err := NewReplayServer(tablet(), func(command string) {
		mu.Lock()
		defer mu.Unlock()
		missed = append(missed, command)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client := dial(t, server.Addr())

	if got := run(t, client, "cat /sys/devices/soc0/machine"); got.stdout != "reMarkable Chiappa\n" || got.exit != 0 {
		t.Errorf("machine = %+v", got)
	}
	var lists []string
	for range 3 {
		lists = append(lists, run(t, client, "vellum list").stdout)
	}
	if want := []string{"xovi\n", "xovi\nappload\n", "xovi\nappload\n"}; !slices.Equal(lists, want) {
		t.Errorf("vellum list answers = %q, want %q", lists, want)
	}
	if got := run(t, client, "vellum add nope"); got.exit != 1 || !strings.Contains(got.stderr, "unable to select") {
		t.Errorf("failed add = %+v", got)
	}
	if got := run(t, client, "sleep 60"); got.signal != "TERM" {
		t.Errorf("signaled command = %+v, want signal TERM", got)
	}
	if got := run(t, client, "rm -rf /tmp/remanager-0b6f1c2e-8a1d-4c5e-9f00-123456789abc"); got.exit != 0 {
		t.Errorf("a command differing only by a uuid did not match: %+v", got)
	}
	if got := run(t, client, "export PATH=/opt/bin:$PATH; systemctl is-active xochitl"); got.stdout != "active\n" {
		t.Errorf("a command differing only by its PATH prefix did not match: %+v", got)
	}
	if got := run(t, client, "reboot"); got.exit != exitCommandNotFound {
		t.Errorf("unrecorded command = %+v, want exit %d", got, exitCommandNotFound)
	}
	server.Close()
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(missed, []string{"reboot"}) {
		t.Errorf("missed = %q, want only reboot", missed)
	}
	if unasked := server.Unasked(); len(unasked) != 0 {
		t.Errorf("unasked = %q, want none", unasked)
	}
}

func TestReplayReportsRecordedCommandsNeverAskedFor(t *testing.T) {
	server, err := NewReplayServer(tablet(), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	client := dial(t, server.Addr())
	run(t, client, "cat /sys/devices/soc0/machine")
	run(t, client, "vellum list")
	run(t, client, "vellum add nope")
	run(t, client, "sleep 60")
	server.Close()

	want := []string{"rm -rf /tmp/remanager-<uuid>", "systemctl is-active xochitl"}
	if got := server.Unasked(); !slices.Equal(got, want) {
		t.Errorf("unasked = %q, want %q; a second vellum list answer left unused is polling, not a skipped step", got, want)
	}
}

// gatedSession holds "slow" open until released, so a test can make a command that started first finish last.
type gatedSession struct {
	*replaySession
	started chan<- struct{}
	release <-chan struct{}
}

func (s *gatedSession) exec(channel ssh.Channel, command string) (int, string) {
	if command == "slow" {
		s.started <- struct{}{}
		<-s.release
	}
	return s.replaySession.exec(channel, command)
}

func TestRecordKeepsAnswersInStartOrder(t *testing.T) {
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("no ssh binary")
	}
	upstreamTablet := tablet()
	upstreamTablet.Exchanges = append(upstreamTablet.Exchanges, &Exchange{Command: "slow", Stdout: "slow\n"})
	replayer := newReplayer(upstreamTablet, func(string) {})
	started, release := make(chan struct{}), make(chan struct{})
	upstream, err := newServer(func() sessionBackend {
		return &gatedSession{replaySession: &replaySession{replayer: replayer}, started: started, release: release}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer upstream.Close()

	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(key, "")
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	host, port, _ := net.SplitHostPort(upstream.Addr())

	cassette := &Cassette{}
	recorder, err := NewRecordServer([]string{
		"-F", "/dev/null", "-i", keyPath, "-o", "IdentitiesOnly=yes", "-p", port,
		"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", "-o", "LogLevel=ERROR",
		"root@" + host,
	}, cassette)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Close()
	client := dial(t, recorder.Addr())

	slowDone := make(chan error, 1)
	go func() {
		_, err := runCommand(client, "slow")
		slowDone <- err
	}()
	<-started
	if got := run(t, client, "vellum add nope"); got.exit != 1 || !strings.Contains(got.stderr, "unable to select") {
		t.Errorf("proxied failure = %+v", got)
	}
	run(t, client, "vellum list")
	run(t, client, "vellum list")
	close(release)
	if err := <-slowDone; err != nil {
		t.Fatal(err)
	}

	var commands []string
	for _, exchange := range cassette.Exchanges {
		commands = append(commands, exchange.Command)
	}
	if want := []string{"slow", "vellum add nope", "vellum list", "vellum list"}; !slices.Equal(commands, want) {
		t.Fatalf("recorded = %q, want %q: slow started first and finished last", commands, want)
	}
	if got := cassette.Exchanges[0]; got.Stdout != "slow\n" {
		t.Errorf("slow recorded %q, want its answer in its started place", got.Stdout)
	}
	if got := cassette.Exchanges[3]; got.Stdout != "xovi\nappload\n" {
		t.Errorf("second vellum list recorded %q", got.Stdout)
	}
	if got := cassette.Exchanges[1]; got.Exit != 1 || !strings.Contains(got.Stderr, "unable to select") {
		t.Errorf("failure recorded as %+v", got)
	}
}

func TestReplayServesConcurrentSessions(t *testing.T) {
	server, err := NewReplayServer(tablet(), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client := dial(t, server.Addr())

	var wg sync.WaitGroup
	answers := make(chan string, 20)
	errs := make(chan error, 20)
	for range 20 {
		wg.Go(func() {
			got, err := runCommand(client, "vellum list")
			if err != nil {
				errs <- err
				return
			}
			answers <- got.stdout
		})
	}
	wg.Wait()
	close(answers)
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	counts := map[string]int{}
	for answer := range answers {
		counts[answer]++
	}
	if counts["xovi\n"] != 1 || counts["xovi\nappload\n"] != 19 {
		t.Errorf("concurrent vellum list answers = %v, want the first answer once and the last for the rest", counts)
	}
}

func TestReplayDropsAndRefusesAfterAReboot(t *testing.T) {
	cassette := &Cassette{Exchanges: []*Exchange{{Command: "reboot", DropsConnection: true}, {Command: "uptime", Stdout: "up 1 min\n"}}}
	server, err := NewReplayServer(cassette, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client := dial(t, server.Addr())
	run(t, client, "reboot")

	client.Wait()
	if conn, err := net.DialTimeout("tcp", server.Addr(), time.Second); err == nil {
		conn.Close()
		t.Fatal("a connection was accepted while the tablet was down")
	}
	deadline := time.Now().Add(replayDownFor + 3*time.Second)
	for {
		conn, err := net.DialTimeout("tcp", server.Addr(), time.Second)
		if err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the server never came back after the outage")
		}
		time.Sleep(200 * time.Millisecond)
	}
	if got := run(t, dial(t, server.Addr()), "uptime"); got.stdout != "up 1 min\n" {
		t.Errorf("after the outage, uptime = %+v", got)
	}
	if got := run(t, dial(t, server.Addr()), "reboot"); got.exit != 0 {
		t.Errorf("a replayed reboot answered %+v", got)
	}
}
