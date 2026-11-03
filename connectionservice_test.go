package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	apperrors "reManager/internal/errors"
)

func TestIsRetryableError(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want bool
	}{
		{"handshake reset by peer", "ssh: handshake failed: read tcp 10.11.99.16:59146->10.11.99.1:22: read: connection reset by peer", true},
		{"windows conn refused", "dial tcp 192.168.2.46:22: connectex: No connection could be made because the target machine actively refused it.", true},
		{"windows timed out", "read tcp 192.168.2.32:59111->192.168.2.46:22: wsarecv: A connection attempt failed because the connected party did not properly respond after a period of time", true},
		{"unix i/o timeout", "dial tcp 10.11.99.1:22: i/o timeout", true},
		{"auth failure not retryable", "ssh: handshake failed: ssh: unable to authenticate, attempted methods [none password]", false},
		{"permission denied not retryable", "ssh: handshake failed: permission denied", false},
		{"bare handshake not retryable", "ssh: handshake failed", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isRetryableError(errString(c.msg)); got != c.want {
				t.Errorf("isRetryableError(%q) = %v, want %v", c.msg, got, c.want)
			}
		})
	}
}

// Auto-reconnect keeps trying on result.Retryable, which comes from the raw
// dial error, so the failures seen while a tablet reboots must classify as
// retryable there.
func TestReconnectRetriesTransportFailures(t *testing.T) {
	for _, raw := range []string{
		"dial tcp 10.11.99.1:22: connect: connection refused",
		"dial tcp 10.11.99.1:22: connect: no route to host",
		"dial tcp 10.11.99.1:22: connect: network is unreachable",
		"ssh: handshake failed: read tcp 10.11.99.16:59146->10.11.99.1:22: read: connection reset by peer",
		"dial tcp 10.11.99.1:22: i/o timeout",
	} {
		if ue := apperrors.Classify(errString(raw)); !ue.Retryable {
			t.Errorf("Classify(%q).Retryable = false, want true", raw)
		}
	}
	if ue := apperrors.Classify(errString("ssh: handshake failed: ssh: unable to authenticate, attempted methods [none password]")); ue.Retryable {
		t.Error("an auth failure classified as retryable")
	}
}

func TestHandshakeWithASilentPeerTimesOut(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
		}
	}()

	connection := newServices(&core{}).connection
	connection.beginReconnect()
	config := &ssh.ClientConfig{User: "root", HostKeyCallback: ssh.InsecureIgnoreHostKey()}
	started := time.Now()
	client, err := connection.dialWithContext(context.Background(), listener.Addr().String(), config)
	if err == nil {
		client.Close()
		t.Fatal("handshake with a silent peer succeeded")
	}
	if elapsed := time.Since(started); elapsed > reconnectDialTimeout+2*time.Second {
		t.Errorf("handshake gave up after %v, want about %v", elapsed, reconnectDialTimeout)
	}
}

func TestEndedReconnectLeavesTheNextOneRunning(t *testing.T) {
	connection := newServices(&core{}).connection
	stale := connection.beginReconnect()
	if connection.beginReconnect() != nil {
		t.Fatal("a second reconnect started while one was running")
	}
	connection.stopReconnect()
	select {
	case <-stale.stop:
	default:
		t.Fatal("stopReconnect did not wake the reconnect's backoff wait")
	}

	current := connection.beginReconnect()
	if current == nil {
		t.Fatal("a reconnect could not start after the previous one was stopped")
	}
	connection.endReconnect(stale)
	if !connection.reconnectCurrent(current) || !connection.isFastDialMode() {
		t.Error("the stopped reconnect cleared the state of the one that replaced it")
	}
	if connection.reconnectCurrent(stale) {
		t.Error("the stopped reconnect still counts as current")
	}
}

func TestIsConnectionDeadError(t *testing.T) {
	dead := []string{
		"read tcp: wsarecv: A connection attempt failed because the connected party did not properly respond",
		"read: connection reset by peer",
		"ssh: unexpected packet in response to channel open: <nil>",
		"use of closed network connection",
	}
	for _, m := range dead {
		if !isConnectionDeadError(errString(m)) {
			t.Errorf("isConnectionDeadError(%q) = false, want true", m)
		}
	}
	if isConnectionDeadError(errString("Process exited with status 1")) {
		t.Errorf("command exit misclassified as dead connection")
	}
}

// A reMarkable whose link died mid-command: the command is accepted and never
// answered, so the session blocks until the client is closed.
func hangingDevice(t *testing.T) (*ssh.Client, <-chan struct{}) {
	t.Helper()
	_, hostKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(hostKey)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })

	commandStarted := make(chan struct{}, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		t.Cleanup(func() { conn.Close() })
		_, channels, requests, err := ssh.NewServerConn(conn, config)
		if err != nil {
			return
		}
		go ssh.DiscardRequests(requests)
		for newChannel := range channels {
			_, channelRequests, err := newChannel.Accept()
			if err != nil {
				continue
			}
			go func() {
				for req := range channelRequests {
					req.Reply(req.Type == "exec", nil)
					if req.Type == "exec" {
						commandStarted <- struct{}{}
					}
				}
			}()
		}
	}()

	client, err := ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{
		User:            "root",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return client, commandStarted
}

func TestConnectionLossIsHandledWhileAStatusCommandWaitsOnTheLink(t *testing.T) {
	client, commandStarted := hangingDevice(t)
	s := newServices(&core{client: client})
	s.connection.currentConn = &connTarget{host: "10.11.99.1:22"}
	// A newer generation makes handleConnectionLost stop before it emits, which
	// needs a Wails app; the lock order under test comes before that.
	s.connection.connGen.Store(1)

	statusErr := make(chan error, 1)
	go func() {
		_, err := s.device.GetUpdateServiceStatus()
		statusErr <- err
	}()
	select {
	case <-commandStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("the status command never reached the reMarkable")
	}

	handled := make(chan struct{})
	go func() {
		s.connection.handleConnectionLost(client, errString("keepalive timeout"))
		close(handled)
	}()
	select {
	case <-handled:
	case <-time.After(5 * time.Second):
		t.Fatal("handleConnectionLost blocked behind a command waiting on the dead link")
	}
	select {
	case err := <-statusErr:
		if err == nil {
			t.Error("the status command reported a status from a closed connection")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the status command was not released by closing the client")
	}
}

type errString string

func (e errString) Error() string { return string(e) }

// A probe's non-zero exit is how it answers "no", so only an error that
// carries no exit status may be treated as the status being unknown.
func TestIsTransportError(t *testing.T) {
	exited := &ssh.ExitError{}
	if isTransportError(nil) {
		t.Error("nil misclassified as a transport error")
	}
	if isTransportError(exited) {
		t.Error("command exit misclassified as a transport error")
	}
	if isTransportError(fmt.Errorf("probe: %w", exited)) {
		t.Error("wrapped command exit misclassified as a transport error")
	}
	for _, err := range []error{
		errNotConnected,
		errString("ssh: unexpected packet in response to channel open: <nil>"),
		errString("read tcp 10.11.99.3:59714->10.11.99.1:22: read: can't assign requested address"),
	} {
		if !isTransportError(err) {
			t.Errorf("isTransportError(%q) = false, want true", err)
		}
	}
}

func TestStatusGettersReportUnknownWhenDisconnected(t *testing.T) {
	s := newServices(&core{})
	if _, err := s.device.GetXochitlStatusQuick(); err == nil {
		t.Error("GetXochitlStatusQuick without a client reported a status")
	}
	if _, err := s.device.GetUpdateServiceStatus(); err == nil {
		t.Error("GetUpdateServiceStatus without a client reported a status")
	}
	if _, err := s.device.CheckHashtabVersion(); err == nil {
		t.Error("CheckHashtabVersion without a client reported a status")
	}
	if _, err := s.device.GetTimezoneStatus(); err == nil {
		t.Error("GetTimezoneStatus without a client reported a status")
	}
	if _, err := s.config.IsSleepScreenSupported(); err == nil {
		t.Error("IsSleepScreenSupported without a client reported a status")
	}
	if _, err := s.packages.GetInstalledPackagesWithVersions(); err == nil {
		t.Error("GetInstalledPackagesWithVersions without a client reported packages")
	}
	if _, err := s.packages.GetInstalledPackagesWithOsCheck(); err == nil {
		t.Error("GetInstalledPackagesWithOsCheck without a client reported packages")
	}
}

func TestCurrentConnectionIsReportedOnlyOnceTheDeviceIsKnown(t *testing.T) {
	connection := newServices(&core{}).connection
	if got := connection.GetCurrentConnection(); got != nil {
		t.Fatalf("disconnected app reported %+v", got)
	}
	connection.demo = true
	if got := connection.GetCurrentConnection(); got != nil {
		t.Fatalf("a connection with no detected model reported %+v", got)
	}
	connection.connectedDeviceType = demoDeviceType
	connection.connectedDeviceID = "saved-id"
	got := connection.GetCurrentConnection()
	if got == nil || got.DeviceType != string(demoDeviceType) || got.DeviceID != "saved-id" {
		t.Fatalf("GetCurrentConnection = %+v, want the demo device", got)
	}
}

func TestReloadResendsTheCurrentConnectionsCheckResults(t *testing.T) {
	var emitted []string
	connection := newServices(&core{demo: true, operationSink: func(name string, _ any) { emitted = append(emitted, name) }}).connection
	stale := connection.connGen.Add(1)
	connection.emitVellumState(stale, false, nil)
	gen := connection.connGen.Add(1)
	connection.emitVellumState(gen, true, []string{"remarkable-os"})
	connection.emitConnectWarnings(gen, ConnectWarnings{})
	connection.emitGuideStatus(gen, UserGuideStatus{Unseen: true})
	connection.emitVellumState(stale, false, nil)

	emitted = nil
	connection.ResendConnectState()
	want := []string{eventVellumReady, eventVellumBrokenInstall, eventConnectWarnings, eventGuideStatus}
	if fmt.Sprint(emitted) != fmt.Sprint(want) {
		t.Fatalf("resent %v, want %v", emitted, want)
	}

	connection.recordVellumState(gen, false, nil)
	emitted = nil
	connection.ResendConnectState()
	if len(emitted) == 0 || emitted[0] != eventVellumMissing {
		t.Errorf("after Vellum was uninstalled the reload was told %v", emitted)
	}

	connection.connGen.Add(1)
	emitted = nil
	connection.ResendConnectState()
	if len(emitted) != 0 {
		t.Errorf("a replaced connection's results were resent: %v", emitted)
	}
}
