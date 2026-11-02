//go:build integration

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	apperrors "reManager/internal/errors"
)

func TestConnectWithPassword(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	from := emitted.mark()
	app := newTestApp(t)

	result := app.Connect(tablet.addr, tablet.password)
	if !result.Success || result.Device != "rmpp" {
		t.Fatalf("Connect = %+v, want success on an rmpp", result)
	}
	if !app.isConnected() || app.inDemoMode() {
		t.Fatalf("isConnected=%v inDemoMode=%v after a real connect", app.isConnected(), app.inDemoMode())
	}
	emitted.waitFor(t, from, time.Minute, "vellum:ready")
	warnings, _ := emitted.waitFor(t, from, time.Minute, "connect:warnings")
	if warnings.Data == nil {
		t.Error("connect:warnings carried no device facts")
	}
	if info := app.GetDeviceInfo(); info["firmware"] != "3.26.0.68" {
		t.Errorf("GetDeviceInfo firmware = %q, want the tablet's 3.26.0.68 (info %v)", info["firmware"], info)
	}

	app.Disconnect()
	if app.isConnected() {
		t.Error("still connected after Disconnect")
	}
}

func TestConnectErrorsAreReadable(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := newTestApp(t)

	wrongPassword := app.Connect(tablet.addr, "not-the-password")
	if wrongPassword.Success || wrongPassword.Code != apperrors.ErrAuthFailed || wrongPassword.Retryable {
		t.Errorf("wrong password = %+v, want a non-retryable %s", wrongPassword, apperrors.ErrAuthFailed)
	}
	assertReadable(t, "wrong password", wrongPassword.Message)

	closedPort := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	unreachable := app.Connect(closedPort, "test")
	if unreachable.Success || unreachable.Code != apperrors.ErrConnectionRefused {
		t.Errorf("unreachable address = %+v, want %s", unreachable, apperrors.ErrConnectionRefused)
	}
	assertReadable(t, "unreachable address", unreachable.Message)

	if app.isConnected() {
		t.Error("a failed connect left the app connected")
	}
}

// A readable error is a sentence for the user rather than a Go error chain.
func assertReadable(t *testing.T, what, message string) {
	t.Helper()
	if message == "" || strings.Contains(message, "ssh:") || strings.Contains(message, "dial tcp") {
		t.Errorf("%s message %q is not a readable sentence", what, message)
	}
}

func TestCancelConnectDuringHandshake(t *testing.T) {
	requireDocker(t)
	// Accepts the connection and never speaks SSH, so the connect waits in the handshake until it is canceled.
	silent, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer silent.Close()
	go func() {
		for {
			conn, err := silent.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
		}
	}()

	app := newTestApp(t)
	results := make(chan ConnectionResult, 1)
	go func() { results <- app.Connect(silent.Addr().String(), "test") }()
	time.Sleep(time.Second)
	app.CancelConnect()

	select {
	case result := <-results:
		if result.Success || result.Code != apperrors.ErrOperationCanceled {
			t.Errorf("canceled connect = %+v, want %s", result, apperrors.ErrOperationCanceled)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Connect did not return after CancelConnect")
	}
	if app.isConnected() {
		t.Error("connected after a canceled connect")
	}
}

func TestConnectWithManagedKey(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	key := newTestKey(t, "")
	authorizeKey(tablet, key.signer)
	app := newTestApp(t)

	imported := app.ImportManagedKey("Integration key", key.privatePEM, "")
	if !imported.Success {
		t.Fatalf("ImportManagedKey: %s (%s)", imported.Message, imported.Code)
	}
	if imported.Key.Fingerprint != ssh.FingerprintSHA256(key.signer.PublicKey()) {
		t.Errorf("imported fingerprint %q, want %q", imported.Key.Fingerprint, ssh.FingerprintSHA256(key.signer.PublicKey()))
	}

	result := app.ConnectWithManagedKey(tablet.addr, imported.Key.ID, "")
	if !result.Success {
		t.Fatalf("ConnectWithManagedKey: %s (%s)", result.Message, result.Code)
	}
	app.Disconnect()

	if err := app.DeleteManagedKey(imported.Key.ID); err != nil {
		t.Fatalf("DeleteManagedKey: %v", err)
	}
	if keys := app.GetManagedKeys(); len(keys) != 0 {
		t.Errorf("GetManagedKeys after delete = %+v", keys)
	}
	if result := app.ConnectWithManagedKey(tablet.addr, imported.Key.ID, ""); result.Success {
		t.Error("connected with a deleted key")
	}
}

func TestConnectWithPassphraseProtectedManagedKey(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	key := newTestKey(t, "correct horse")
	authorizeKey(tablet, key.signer)
	app := newTestApp(t)

	if wrong := app.ImportManagedKey("Protected key", key.privatePEM, "wrong"); wrong.Success {
		t.Error("imported a protected key with the wrong passphrase")
	}
	imported := app.ImportManagedKey("Protected key", key.privatePEM, "correct horse")
	if !imported.Success || !imported.Key.Encrypted {
		t.Fatalf("ImportManagedKey = %+v, want an encrypted key", imported)
	}
	// The passphrase given at import is kept, so connecting needs no retyping.
	if result := app.ConnectWithManagedKey(tablet.addr, imported.Key.ID, ""); !result.Success {
		t.Fatalf("ConnectWithManagedKey: %s (%s)", result.Message, result.Code)
	}
}

func TestConnectWithDefaultKeyFile(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	key := newTestKey(t, "")
	authorizeKey(tablet, key.signer)

	sshDir := filepath.Join(os.Getenv("HOME"), ".ssh")
	keyPath := filepath.Join(sshDir, "id_ed25519")
	os.MkdirAll(sshDir, 0o700)
	if err := os.WriteFile(keyPath, []byte(key.privatePEM), 0o600); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(keyPath+".pub", ssh.MarshalAuthorizedKey(key.signer.PublicKey()), 0o644)
	t.Cleanup(func() { os.RemoveAll(sshDir) })

	app := newTestApp(t)
	found := app.GetDefaultSSHKeys()
	if len(found) != 1 || found[0].Path != keyPath {
		t.Fatalf("GetDefaultSSHKeys = %+v, want only %s", found, keyPath)
	}
	if result := app.ConnectWithKey(tablet.addr, found[0].Path, ""); !result.Success {
		t.Fatalf("ConnectWithKey: %s (%s)", result.Message, result.Code)
	}
}

func TestConnectThroughSSHAgent(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	key := newTestKey(t, "")
	authorizeKey(tablet, key.signer)
	app := newTestApp(t)

	socketPath := serveAgent(t, key)
	if app.IsSSHAgentAvailable() {
		t.Fatal("an agent was found before the setting pointed at one")
	}
	restoreSettingsAfter(t, app)
	if err := app.UpdateSettings(SettingsPatch{SSHAgentSocketPath: &socketPath}); err != nil {
		t.Fatal(err)
	}
	if !app.IsSSHAgentAvailable() {
		t.Fatal("the agent at the configured socket was not found")
	}
	if result := app.ConnectWithAgent(tablet.addr); !result.Success {
		t.Fatalf("ConnectWithAgent: %s (%s)", result.Message, result.Code)
	}
}

func TestSavedDeviceReconnectsWithStoredPassword(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	first := connectApp(t, tablet)
	id, err := first.SaveDevice("", "Desk tablet", tablet.addr, "password", tablet.password, "", "", "")
	if err != nil {
		t.Fatalf("SaveDevice: %v", err)
	}
	first.shutdown(t.Context())

	restarted := newTestApp(t)
	saved := restarted.GetSavedDevices()
	if len(saved) != 1 || saved[0].ID != id || saved[0].Name != "Desk tablet" {
		t.Fatalf("GetSavedDevices after restart = %+v", saved)
	}
	if result := restarted.ConnectToSavedDevice(id); !result.Success {
		t.Fatalf("ConnectToSavedDevice: %s (%s)", result.Message, result.Code)
	}

	if _, err := restarted.SaveDevice(id, "Bedside tablet", tablet.addr, "password", "", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if saved := restarted.GetSavedDevices(); saved[0].Name != "Bedside tablet" {
		t.Errorf("name after rename = %q", saved[0].Name)
	}
	restarted.Disconnect()
	if err := restarted.DeleteSavedDevice(id); err != nil {
		t.Fatal(err)
	}
	if saved := restarted.GetSavedDevices(); len(saved) != 0 {
		t.Errorf("GetSavedDevices after delete = %+v", saved)
	}
	if result := restarted.ConnectToSavedDevice(id); result.Success || result.Code != apperrors.ErrDeviceNotFound {
		t.Errorf("connect to a deleted device = %+v, want %s", result, apperrors.ErrDeviceNotFound)
	}
}

func TestSavedDeviceReconnectsWithManagedKey(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	key := newTestKey(t, "")
	authorizeKey(tablet, key.signer)
	first := newTestApp(t)
	imported := first.ImportManagedKey("Saved key", key.privatePEM, "")
	if !imported.Success {
		t.Fatal(imported.Message)
	}
	id, err := first.SaveDevice("", "Key tablet", tablet.addr, "key", "", "", "", imported.Key.ID)
	if err != nil {
		t.Fatal(err)
	}
	first.shutdown(t.Context())

	restarted := newTestApp(t)
	if result := restarted.ConnectToSavedDevice(id); !result.Success {
		t.Fatalf("ConnectToSavedDevice: %s (%s)", result.Message, result.Code)
	}
}

// A paused container is a tablet that stops answering without closing anything, as one that loses Wi-Fi does; the keepalive runs every 15 seconds and gives up after 5 more.
func TestIdleDropReconnects(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	from := emitted.mark()
	app := connectApp(t, tablet)
	emitted.waitFor(t, from, time.Minute, "connect:warnings")

	from = emitted.mark()
	tablet.docker("pause")
	emitted.waitFor(t, from, 30*time.Second, "connection:lost")
	emitted.waitFor(t, from, 10*time.Second, "connection:reconnecting")
	tablet.docker("unpause")
	assertReconnected(t, app, from)
}

// The connect checks hold a.mu while their command waits on the link, and the command only gives it up once the dead client is closed.
func TestDropDuringConnectChecksReconnects(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectApp(t, tablet)

	from := emitted.mark()
	tablet.docker("pause")
	emitted.waitFor(t, from, 30*time.Second, "connection:lost")
	tablet.docker("unpause")
	assertReconnected(t, app, from)
}

func TestReconnectCountsDownWhileTheTabletIsAway(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	from := emitted.mark()
	app := connectApp(t, tablet)
	emitted.waitFor(t, from, time.Minute, "connect:warnings")

	from = emitted.mark()
	tablet.docker("stop")
	emitted.waitFor(t, from, 30*time.Second, "connection:lost")
	retry, _ := emitted.waitFor(t, from, 30*time.Second, "connection:retry-scheduled", "connection:failed")
	if retry.Name == "connection:failed" {
		t.Fatalf("reconnect gave up at once: %v", retry.Data)
	}
	countdown, _ := retry.Data.(ReconnectPhase)
	if countdown.Attempt != 2 || countdown.DurationMs == 0 {
		t.Errorf("retry countdown = %v, want attempt 2 with a duration", countdown)
	}
	tablet.docker("start")
	assertReconnected(t, app, from)
}

// reconnectDialTimeout bounds one attempt and the UI sizes its reconnect window from it, so a tablet that accepts the connection and never answers must not hold an attempt open past it.
func TestReconnectAttemptIsBoundedWhenTheTabletIsSilent(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	from := emitted.mark()
	connectApp(t, tablet)
	emitted.waitFor(t, from, time.Minute, "connect:warnings")

	from = emitted.mark()
	tablet.docker("pause")
	t.Cleanup(func() { tablet.docker("unpause") })
	emitted.waitFor(t, from, 30*time.Second, "connection:reconnecting")
	emitted.waitFor(t, from, reconnectDialTimeout+5*time.Second, "connection:retry-scheduled", "connection:failed")
}

func assertReconnected(t *testing.T, app *testApp, from int) {
	t.Helper()
	event, _ := emitted.waitFor(t, from, time.Minute, "connection:restored", "connection:failed")
	if event.Name != "connection:restored" {
		t.Fatalf("reconnect ended with %s: %v", event.Name, event.Data)
	}
	if !app.isConnected() {
		t.Error("not connected after connection:restored")
	}
	if _, err := app.GetInstalledPackagesWithVersions(); err != nil {
		t.Errorf("the restored connection cannot run commands: %v", err)
	}
}

func TestReconnectRefusesADifferentTablet(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectApp(t, tablet)
	from := emitted.mark()

	// Another tablet answers at the same address once the first one is gone.
	tablet.run("tr -d - < /proc/sys/kernel/random/uuid > /etc/machine-id")
	tablet.docker("restart")
	emitted.waitFor(t, from, 30*time.Second, "connection:lost")

	event, _ := emitted.waitFor(t, from, time.Minute, "connection:restored", "connection:failed")
	failure, _ := event.Data.(ConnectionFailure)
	if event.Name != "connection:failed" || failure.Code != apperrors.ErrDeviceChanged {
		t.Fatalf("reconnect to a different tablet ended with %s %v, want connection:failed %s", event.Name, event.Data, apperrors.ErrDeviceChanged)
	}
	if app.isConnected() {
		t.Error("stayed connected to a tablet the user never chose")
	}
}

func TestDemoModeEntersAndExits(t *testing.T) {
	app := newTestApp(t)
	waitForCatalog(t, app)

	result := app.Connect("demo", "demo")
	if !result.Success || !app.inDemoMode() || !app.isConnected() {
		t.Fatalf("demo connect = %+v, demo=%v connected=%v", result, app.inDemoMode(), app.isConnected())
	}
	installed, err := app.GetInstalledPackagesWithVersions()
	if err != nil || len(installed) == 0 {
		t.Errorf("demo tablet has no installed mods: %v %v", installed, err)
	}
	if packages := app.GetPackages(result.Device, app.GetDeviceInfo()["firmware"], "aarch64"); len(packages) == 0 {
		t.Error("the demo catalog is empty")
	}

	app.Disconnect()
	if app.inDemoMode() || app.isConnected() {
		t.Errorf("after Disconnect demo=%v connected=%v", app.inDemoMode(), app.isConnected())
	}
}

func TestSettingsPersistAcrossRestart(t *testing.T) {
	first := newTestApp(t)
	restoreSettingsAfter(t, first)
	if err := first.UpdateSettings(SettingsPatch{
		ProxyMode:                  new(false),
		SuppressSystemFileWarnings: new(true),
		Theme:                      new("dark"),
		TerminalTheme:              new("dracula"),
		EditorTheme:                new("monokai"),
		CheckForUpdates:            new(false),
		SSHAgentSocketPath:         new("/tmp/agent.sock"),
	}); err != nil {
		t.Fatal(err)
	}
	first.shutdown(t.Context())

	settings := newTestApp(t).GetSettings()
	if settings.ProxyMode || !settings.SuppressSystemFileWarnings || settings.Theme != "dark" ||
		settings.TerminalTheme != "dracula" || settings.EditorTheme != "monokai" ||
		settings.CheckForUpdates || settings.SSHAgentSocketPath != "/tmp/agent.sock" {
		t.Errorf("settings after restart = %+v", settings)
	}
}

// Settings are stored in the sandbox every test shares, and proxy mode left off changes how a later install resolves its packages.
func restoreSettingsAfter(t *testing.T, app *testApp) {
	t.Helper()
	before := app.GetSettings()
	t.Cleanup(func() {
		newTestApp(t).UpdateSettings(SettingsPatch{
			ProxyMode:                  &before.ProxyMode,
			SuppressSystemFileWarnings: &before.SuppressSystemFileWarnings,
			PreventSleep:               &before.PreventSleep,
			CheckForUpdates:            &before.CheckForUpdates,
			Theme:                      &before.Theme,
			TerminalTheme:              &before.TerminalTheme,
			EditorTheme:                &before.EditorTheme,
			SSHAgentSocketPath:         &before.SSHAgentSocketPath,
		})
	})
}

func TestUpdateCheckFindsANewerRelease(t *testing.T) {
	released := version
	version = "0.0.1"
	t.Cleanup(func() { version = released })

	result := newTestApp(t).CheckForAppUpdate()
	if result.Error != "" || !result.UpdateAvailable || result.LatestVersion == "" || result.ReleaseURL == "" {
		t.Errorf("CheckForAppUpdate from 0.0.1 = %+v", result)
	}
}

type testKey struct {
	private    ed25519.PrivateKey
	signer     ssh.Signer
	privatePEM string
}

func newTestKey(t *testing.T, passphrase string) testKey {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var block *pem.Block
	if passphrase == "" {
		block, err = ssh.MarshalPrivateKey(private, "")
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(private, "", []byte(passphrase))
	}
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return testKey{private: private, signer: signer, privatePEM: string(pem.EncodeToMemory(block))}
}

func authorizeKey(tablet *fakeTablet, signer ssh.Signer) {
	key := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	tablet.run(fmt.Sprintf("mkdir -p -m 700 /home/root/.ssh && echo '%s' >> /home/root/.ssh/authorized_keys && chmod 600 /home/root/.ssh/authorized_keys", key))
}

func serveAgent(t *testing.T, key testKey) string {
	t.Helper()
	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: key.private}); err != nil {
		t.Fatal(err)
	}
	// Unix socket paths are capped near 104 bytes on macOS, which a nested t.TempDir can exceed.
	dir, err := os.MkdirTemp("/tmp", "agent")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socketPath := filepath.Join(dir, "sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go agent.ServeAgent(keyring, conn)
		}
	}()
	return socketPath
}
