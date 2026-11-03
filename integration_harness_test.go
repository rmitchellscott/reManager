//go:build integration

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/zalando/go-keyring"

	"reManager/internal/httputil"
	"reManager/internal/storage"
)

// The suite drives the bound service methods against a fake reMarkable in Docker and needs network access for the catalog and Vellum releases; the TestRecorded tests (integration_recorded_test.go) need neither.

const fakeImage = "remanager-fake-remarkable-base"

var (
	wailsTestApp *application.App
	emitted      = &eventLog{}
	sandboxDir   string
)

func TestMain(m *testing.M) {
	os.Exit(runSuite(m))
}

func runSuite(m *testing.M) int {
	dir, err := os.MkdirTemp("", "remanager-integration-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	sandboxDir = dir

	// Stores, keys and ~/.ssh discovery all resolve from these, so a run never touches the developer's own devices, keychain or SSH keys.
	home := filepath.Join(dir, "home")
	for _, sub := range []string{"home", "config", "data"} {
		os.MkdirAll(filepath.Join(dir, sub), 0o700)
	}
	os.Setenv("HOME", home)
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	os.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	os.Unsetenv("SSH_AUTH_SOCK")
	storage.SetConfigDir(filepath.Join(dir, "config", "reManager"))
	keyring.MockInit()
	httputil.Init("integration-test")

	// Bundles and reports go here, never to support.remanager.io.
	support := httptest.NewServer(supportStub)
	defer support.Close()
	os.Setenv("REMANAGER_SUPPORT_URL", support.URL)

	wailsTestApp = application.New(application.Options{Name: "reManager-integration", Transport: emitted})

	return m.Run()
}

type supportService struct {
	mu       sync.Mutex
	requests []supportRequest
}

type supportRequest struct {
	method string
	path   string
	header http.Header
	body   []byte
}

var supportStub = &supportService{}

func (s *supportService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.requests = append(s.requests, supportRequest{r.Method, r.URL.Path, r.Header.Clone(), body})
	count := len(s.requests)
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/bundles":
		id := fmt.Sprintf("bundle-%d", count)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"id": id, "url": "http://" + r.Host + "/b/" + id, "delete_token": "token-" + id,
			"created_at": time.Now().UTC().Format(time.RFC3339),
		})
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/append"):
		json.NewEncoder(w).Encode(map[string]any{
			"id": strings.Split(r.URL.Path, "/")[3], "updated_at": time.Now().UTC().Format(time.RFC3339), "size": len(body),
		})
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/bundles/"):
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && r.URL.Path == "/api/reports":
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("{}"))
	default:
		http.NotFound(w, r)
	}
}

func (s *supportService) received() []supportRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]supportRequest(nil), s.requests...)
}

var (
	fakeImageOnce sync.Once
	fakeImageErr  error
)

// Built on first use, so the recorded tests run where there is no Docker.
func requireFakeImage(t *testing.T) {
	t.Helper()
	fakeImageOnce.Do(func() { fakeImageErr = buildFakeImage() })
	if fakeImageErr != nil {
		t.Fatalf("fake reMarkable image: %v", fakeImageErr)
	}
}

func buildFakeImage() error {
	cmd := exec.Command("docker", "build", "-q", "-t", fakeImage, "-")
	cmd.Stdin = strings.NewReader("FROM alpine:3\nRUN apk add --no-cache openssh openssh-sftp-server bash coreutils openssl umount tzdata\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}

// eventLog is the app's transport, which Wails hands every emitted event serially in emit order; Go listeners on Event.On run in their own goroutines and can observe events out of order.
type eventLog struct {
	mu      sync.Mutex
	entries []*application.CustomEvent
	changed chan struct{}
}

func (l *eventLog) Start(context.Context, *application.MessageProcessor) error { return nil }
func (l *eventLog) JSClient() []byte                                           { return nil }
func (l *eventLog) Stop() error                                                { return nil }

func (l *eventLog) DispatchWailsEvent(event *application.CustomEvent) {
	l.mu.Lock()
	l.entries = append(l.entries, event)
	if l.changed != nil {
		close(l.changed)
		l.changed = nil
	}
	l.mu.Unlock()
}

func (l *eventLog) mark() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}

func (l *eventLog) since(from int) []*application.CustomEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]*application.CustomEvent(nil), l.entries[from:]...)
}

func (l *eventLog) waitFor(t *testing.T, from int, timeout time.Duration, names ...string) (*application.CustomEvent, int) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		l.mu.Lock()
		for i := from; i < len(l.entries); i++ {
			for _, name := range names {
				if l.entries[i].Name == name {
					event := l.entries[i]
					l.mu.Unlock()
					return event, i + 1
				}
			}
		}
		if l.changed == nil {
			l.changed = make(chan struct{})
		}
		changed := l.changed
		l.mu.Unlock()
		select {
		case <-changed:
		case <-deadline:
			t.Fatalf("no %v within %v; events since: %s", names, timeout, eventNames(l.since(from)))
			return nil, from
		}
	}
}

func (l *eventLog) count(from int, name string) int {
	n := 0
	for _, event := range l.since(from) {
		if event.Name == name {
			n++
		}
	}
	return n
}

func eventNames(list []*application.CustomEvent) string {
	names := make([]string, len(list))
	for i, event := range list {
		names[i] = event.Name
	}
	return strings.Join(names, ", ")
}

// fakeTablet is one container of scripts/fake-remarkable with its own copy of the directory, so its step time and failure switch are per-tablet.
type fakeTablet struct {
	t         *testing.T
	container string
	dir       string
	addr      string
	password  string
}

type fakeTabletOptions struct {
	stepSeconds    int
	hashtabSeconds int
	// Empty keeps the fake tablet's own default, 3.26, which the fake Vellum was written against.
	osVersion string
	// realVellum leaves Vellum off the tablet, for the bootstrap to install the real one.
	realVellum bool
}

const currentOSVersion = "3.27.3.2"

func startFakeTablet(t *testing.T, opts fakeTabletOptions) *fakeTablet {
	t.Helper()
	requireFakeImage(t)
	dir := t.TempDir()
	if out, err := exec.Command("cp", "-R", "scripts/fake-remarkable/.", dir).CombinedOutput(); err != nil {
		t.Fatalf("copy fake tablet: %v: %s", err, out)
	}
	os.Remove(filepath.Join(dir, "fail-installs"))
	os.Remove(filepath.Join(dir, "rebooted"))
	os.WriteFile(filepath.Join(dir, "step-seconds"), []byte(fmt.Sprint(opts.stepSeconds)), 0o644)
	os.WriteFile(filepath.Join(dir, "hashtab-seconds"), []byte(fmt.Sprint(opts.hashtabSeconds)), 0o644)
	fakeVellum := "on"
	if opts.realVellum {
		fakeVellum = "off"
	}
	port := freePort(t)
	tablet := &fakeTablet{
		t:         t,
		container: fmt.Sprintf("remanager-it-%d", time.Now().UnixNano()),
		dir:       dir,
		addr:      fmt.Sprintf("127.0.0.1:%d", port),
		password:  "test",
	}
	// The restart policy is what brings the tablet back after systemctl reboot.
	args := []string{"run", "-d", "--privileged", "--restart", "unless-stopped",
		"--name", tablet.container, "-p", fmt.Sprintf("127.0.0.1:%d:22", port),
		"-e", "FAKE_PASSWORD=" + tablet.password, "-e", "FAKE_VELLUM=" + fakeVellum,
		"-v", dir + ":/fake", "-v", "/home"}
	if opts.osVersion != "" {
		args = append(args, "-e", "FAKE_OS_VERSION="+opts.osVersion)
	}
	out, err := exec.Command("docker", append(args, fakeImage, "/fake/container-setup.sh")...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker run: %v: %s", err, out)
	}
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", "-v", tablet.container).Run() })
	tablet.waitUntilBooted(1)
	return tablet
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

// sshd logs this line once per boot, so after a reboot the log holds one more.
func (f *fakeTablet) waitUntilBooted(boots int) {
	f.t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		logs, _ := exec.Command("docker", "logs", f.container).CombinedOutput()
		if strings.Count(string(logs), "Server listening on 0.0.0.0") >= boots {
			return
		}
		if time.Now().After(deadline) {
			f.t.Fatalf("fake tablet never booted: %s", logs)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (f *fakeTablet) run(script string) string {
	f.t.Helper()
	out, err := exec.Command("docker", "exec", f.container, "bash", "-c", script).CombinedOutput()
	if err != nil {
		f.t.Fatalf("on tablet %q: %v: %s", script, err, out)
	}
	return string(out)
}

func (f *fakeTablet) setStepSeconds(seconds int) {
	os.WriteFile(filepath.Join(f.dir, "step-seconds"), []byte(fmt.Sprint(seconds)), 0o644)
}

func (f *fakeTablet) failInstalls(on bool) {
	path := filepath.Join(f.dir, "fail-installs")
	if on {
		os.WriteFile(path, nil, 0o644)
	} else {
		os.Remove(path)
	}
}

func (f *fakeTablet) commands() string {
	return f.run("cat /var/log/remanager-commands.log 2>/dev/null || true")
}

func (f *fakeTablet) docker(args ...string) {
	f.t.Helper()
	if out, err := exec.Command("docker", append(args, f.container)...).CombinedOutput(); err != nil {
		f.t.Fatalf("docker %v: %v: %s", args, err, out)
	}
}

// testApp embeds core directly, not through the services that each embed it, so core's promoted fields do not collide.
type testApp struct {
	*core
	*ConnectionService
	*OperationsService
	*OSService
	*ConfigService
	*SystemService
	*ShellService
	*DeviceService
	*PackagesService
	*SupportService
	*FilesService
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()
	c := &core{wails: wailsTestApp}
	s := newServices(c)
	app := &testApp{c, s.connection, s.operations, s.os, s.config, s.system, s.shell, s.device, s.packages, s.support, s.files}
	c.startup(context.Background())
	s.support.ServiceStartup(context.Background(), application.ServiceOptions{})
	t.Cleanup(func() { s.connection.shutdown(context.Background()) })
	return app
}

func connectApp(t *testing.T, tablet *fakeTablet) *testApp {
	t.Helper()
	app := newTestApp(t)
	result := app.Connect(tablet.addr, tablet.password)
	if !result.Success {
		t.Fatalf("Connect: %s (%s)", result.Message, result.Code)
	}
	return app
}

func requireDocker(t *testing.T) {
	t.Helper()
	if os.Getenv("REMANAGER_TEST_HOST") != "" {
		t.Skip("running against a real tablet")
	}
	if runtime.GOOS == "windows" {
		t.Skip("the fake tablet needs a Linux container host")
	}
}

type operationRun struct {
	result  OperationResult
	prompts []DialogRequest
	output  string
}

// An answer of "" leaves the prompt open, for a test that cancels instead.
type promptAnswerer func(DialogRequest) string

func runOp(t *testing.T, app *testApp, timeout time.Duration, answer promptAnswerer, start func(id string)) operationRun {
	t.Helper()
	id := fmt.Sprintf("op-%d", time.Now().UnixNano())
	from := emitted.mark()
	start(id)
	deadline := time.Now().Add(timeout)
	var run operationRun
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("operation %s did not finish within %v; output so far:\n%s", id, timeout, operationOutput(emitted.since(from), id))
		}
		event, next := emitted.waitFor(t, from, remaining, "op:prompt", "op:done")
		from = next
		switch data := event.Data.(type) {
		case OperationPrompt:
			if data.ID != id {
				continue
			}
			run.prompts = append(run.prompts, data.Request)
			if answer == nil {
				t.Fatalf("unexpected prompt %q", data.Request.Title)
			}
			if response := answer(data.Request); response != "" {
				app.RespondToOperation(id, response)
			}
		case OperationResult:
			if data.ID != id {
				continue
			}
			run.result = data
			run.output = operationOutput(emitted.since(0), id)
			return run
		}
	}
}

func runQuietOp(t *testing.T, app *testApp, timeout time.Duration, start func(id string)) (string, error) {
	t.Helper()
	run := runOp(t, app, timeout, nil, start)
	if !run.result.OK {
		return run.result.Value, errors.New(run.result.Error)
	}
	return run.result.Value, nil
}

func operationOutput(list []*application.CustomEvent, id string) string {
	var out strings.Builder
	for _, event := range list {
		if output, ok := event.Data.(OperationOutput); ok && output.ID == id {
			out.WriteString(output.Text)
		}
	}
	return out.String()
}

func actionLabeled(request DialogRequest, label string) string {
	for _, action := range request.Actions {
		if strings.EqualFold(action.Label, label) {
			return action.Id
		}
	}
	return ""
}

func vellumTablet(t *testing.T) (*fakeTablet, *testApp) {
	t.Helper()
	tablet := startFakeTablet(t, fakeTabletOptions{realVellum: true, osVersion: currentOSVersion, hashtabSeconds: 1})
	from := emitted.mark()
	app := connectApp(t, tablet)
	emitted.waitFor(t, from, time.Minute, "vellum:missing")

	// The bootstrap skips checksum verification when it reads local files, which only TestBootstrapVellum needs to cover.
	t.Setenv("VELLUM_BOOTSTRAP_DIR", cachedBootstrapFiles(t))
	if run := runOp(t, app, 5*time.Minute, nil, func(id string) { app.BootstrapVellum(id) }); !run.result.OK {
		t.Fatalf("bootstrap: %+v; output:\n%s", run.result, run.output)
	}
	waitForCatalog(t, app)
	return tablet, app
}

func waitForCatalog(t *testing.T, app *testApp) {
	t.Helper()
	deadline := time.Now().Add(time.Minute)
	for !app.metadata.Ready() {
		if time.Now().After(deadline) {
			t.Fatal("the catalog never loaded")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

var bootstrapFilesOnce sync.Once

// Each bootstrap otherwise asks api.github.com for the latest release, and a run with a dozen tablets hits its unauthenticated rate limit, so the files are cached a day across runs.
func cachedBootstrapFiles(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(os.TempDir(), "remanager-integration-vellum")
	var fetchErr error
	bootstrapFilesOnce.Do(func() {
		if info, err := os.Stat(filepath.Join(dir, "packages.rsa.pub")); err == nil && time.Since(info.ModTime()) < 24*time.Hour {
			return
		}
		fetchErr = fetchBootstrapFiles(dir)
	})
	if fetchErr != nil {
		t.Fatalf("download Vellum bootstrap files: %v", fetchErr)
	}
	return dir
}

func fetchBootstrapFiles(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var release struct {
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := fetchJSON("https://api.github.com/repos/vellum-dev/vellum-cli/releases/latest", &release); err != nil {
		return err
	}
	for _, asset := range release.Assets {
		if asset.Name == "bootstrap.sh" || asset.Name == "vellum-linux-arm64" || asset.Name == "vellum-linux-armv7" {
			if err := download(asset.URL, filepath.Join(dir, asset.Name)); err != nil {
				return err
			}
		}
	}
	script, err := os.ReadFile(filepath.Join(dir, "bootstrap.sh"))
	if err != nil {
		return err
	}
	apkVersion := regexp.MustCompile(`APK_TOOLS_VERSION="([^"]+)"`).FindSubmatch(script)
	if apkVersion == nil {
		return fmt.Errorf("no APK_TOOLS_VERSION in bootstrap.sh")
	}
	for _, name := range []string{"apk-aarch64", "apk-armv7"} {
		url := fmt.Sprintf("https://github.com/vellum-dev/apk-tools/releases/download/%s/%s", apkVersion[1], name)
		if err := download(url, filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return download("https://raw.githubusercontent.com/vellum-dev/vellum/main/keys/packages.rsa.pub", filepath.Join(dir, "packages.rsa.pub"))
}

func fetchJSON(url string, into any) error {
	resp, err := httputil.NewClient(30 * time.Second).Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(into)
}

func download(url, path string) error {
	resp, err := httputil.NewClient(5 * time.Minute).Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o755)
}
