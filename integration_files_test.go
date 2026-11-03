//go:build integration

package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"

	"reManager/internal/backup"
	"reManager/internal/pdfimport"
)

const (
	transferTestDir = "/home/root/it-transfers"
	configDir       = "/home/root/.config/remarkable"
	configPath      = configDir + "/xochitl.conf"
)

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func writeLocalFile(t *testing.T, path string, data []byte) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return sha256Hex(data)
}

func writeRandomFile(t *testing.T, path string, size int) string {
	t.Helper()
	data := make([]byte, size)
	rand.Read(data)
	return writeLocalFile(t, path, data)
}

func localFileHash(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256Hex(data)
}

func tabletFileHash(tablet *fakeTablet, remotePath string) string {
	tablet.t.Helper()
	return strings.Fields(tablet.run(fmt.Sprintf("sha256sum '%s'", remotePath)))[0]
}

func tabletPathExists(tablet *fakeTablet, remotePath string) bool {
	tablet.t.Helper()
	return strings.TrimSpace(tablet.run(fmt.Sprintf("test -e '%s' && echo yes || echo no", remotePath))) == "yes"
}

func tabletTreeHashes(tablet *fakeTablet, root string) map[string]string {
	tablet.t.Helper()
	hashes := map[string]string{}
	out := tablet.run(fmt.Sprintf("cd '%s' && find . -type f -exec sha256sum {} +", root))
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		digest, name, found := strings.Cut(line, "  ")
		if found {
			hashes[strings.TrimPrefix(name, "./")] = digest
		}
	}
	return hashes
}

func localTreeHashes(t *testing.T, root string) map[string]string {
	t.Helper()
	hashes := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.Type().IsRegular() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		hashes[filepath.ToSlash(rel)] = localFileHash(t, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hashes
}

func assertSameTree(t *testing.T, got, want map[string]string) {
	t.Helper()
	if len(want) == 0 {
		t.Fatal("the expected tree is empty")
	}
	for name, digest := range want {
		if got[name] != digest {
			t.Errorf("%s: sha256 %q, want %q", name, got[name], digest)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("unexpected file %s", name)
		}
	}
}

func waitForTransfersToSettle(t *testing.T, app *testApp, timeout time.Duration) TransferSnapshot {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		snapshot := app.GetTransfers()
		if snapshot.ActiveCount == 0 {
			return snapshot
		}
		if time.Now().After(deadline) {
			t.Fatalf("transfers still active after %v: %+v", timeout, snapshot.Transfers)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func transferNamed(t *testing.T, snapshot TransferSnapshot, name string) TransferView {
	t.Helper()
	for _, view := range snapshot.Transfers {
		if view.Name == name {
			return view
		}
	}
	t.Fatalf("no transfer named %q in %+v", name, snapshot.Transfers)
	return TransferView{}
}

func waitForTransferRunning(t *testing.T, app *testApp, name string) TransferView {
	t.Helper()
	deadline := time.Now().Add(time.Minute)
	for time.Now().Before(deadline) {
		for _, view := range app.GetTransfers().Transfers {
			if view.Name != name {
				continue
			}
			switch view.State {
			case TransferRunning:
				if view.BytesDone > 0 {
					return view
				}
			case TransferDone, TransferFailed, TransferCanceled:
				t.Fatalf("%s ended as %s before it could be canceled mid-transfer", name, view.State)
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s never started moving bytes", name)
	return TransferView{}
}

func assertNoUploadLeftovers(t *testing.T, tablet *fakeTablet, dir string, names ...string) {
	t.Helper()
	if leftovers := strings.TrimSpace(tablet.run(fmt.Sprintf("find '%s' -name '*%s'", dir, uploadTempSuffix))); leftovers != "" {
		t.Errorf("temporary upload files left on the tablet:\n%s", leftovers)
	}
	for _, name := range names {
		if tabletPathExists(tablet, dir+"/"+name) {
			t.Errorf("a canceled upload left %s/%s on the tablet", dir, name)
		}
	}
}

// A tablet reached by address has no device ID until the connect dialog saves it, and the ID is what backups, bundles and command logs are filed under.
func connectSavedTablet(t *testing.T, tablet *fakeTablet, name string) (*testApp, string) {
	t.Helper()
	app := connectApp(t, tablet)
	id, err := app.SaveDevice("", name, tablet.addr, "password", tablet.password, "", "", "")
	if err != nil {
		t.Fatalf("SaveDevice: %v", err)
	}
	t.Cleanup(func() { app.DeleteSavedDevice(id) })
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.connectedDeviceID != id {
		t.Fatalf("connected device ID %q after saving, want %q", app.connectedDeviceID, id)
	}
	return app, id
}

func rootMountMode(tablet *fakeTablet) string {
	tablet.t.Helper()
	fields := strings.Fields(tablet.run("grep ' / ' /proc/mounts | head -1"))
	if len(fields) < 4 {
		tablet.t.Fatalf("no root mount in /proc/mounts: %v", fields)
	}
	return strings.Split(fields[3], ",")[0]
}

func TestFilesBrowseAndManage(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectApp(t, tablet)

	root := "/home/root/it-files"
	for _, dir := range []string{root, root + "/notes"} {
		if err := app.CreateDirectory(dir); err != nil {
			t.Fatalf("CreateDirectory(%s): %v", dir, err)
		}
	}
	if err := app.CreateDirectory(root); err == nil {
		t.Error("CreateDirectory succeeded on a folder that already exists")
	}
	tablet.run("cd /home/root/it-files && printf hello > Readme.txt && mkdir -p Full/inner && printf x > Full/inner/file")

	entries, err := app.ListDirectory(root)
	if err != nil {
		t.Fatalf("ListDirectory: %v", err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	if want := []string{"Full", "notes", "Readme.txt"}; !slices.Equal(names, want) {
		t.Fatalf("ListDirectory names %v, want %v (folders first, case-insensitive)", names, want)
	}
	readme := entries[2]
	if readme.IsDir || readme.Size != 5 || readme.Path != root+"/Readme.txt" {
		t.Errorf("Readme.txt listed as %+v", readme)
	}
	if !entries[0].IsDir || !entries[1].IsDir {
		t.Errorf("folders not marked as folders: %+v", entries[:2])
	}

	home, err := app.ListDirectory("")
	if err != nil {
		t.Fatalf("ListDirectory(\"\"): %v", err)
	}
	if !slices.ContainsFunc(home, func(entry FileInfo) bool { return entry.Path == root }) {
		t.Errorf("an empty path did not list /home/root: %+v", home)
	}

	if err := app.RenamePath(root+"/Readme.txt", root+"/notes/renamed.txt"); err != nil {
		t.Fatalf("RenamePath file: %v", err)
	}
	if err := app.RenamePath(root+"/notes", root+"/archive"); err != nil {
		t.Fatalf("RenamePath folder: %v", err)
	}
	if got := tablet.run("cat /home/root/it-files/archive/renamed.txt"); got != "hello" {
		t.Errorf("renamed file holds %q", got)
	}
	if tabletPathExists(tablet, root+"/Readme.txt") || tabletPathExists(tablet, root+"/notes") {
		t.Error("the old names still exist after the rename")
	}

	for _, target := range []string{root + "/archive/renamed.txt", root + "/archive", root + "/Full"} {
		if err := app.DeletePath(target); err != nil {
			t.Fatalf("DeletePath(%s): %v", target, err)
		}
	}
	entries, err = app.ListDirectory(root)
	if err != nil {
		t.Fatalf("ListDirectory after deletes: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("folder not empty after deleting everything: %+v", entries)
	}
	if err := app.DeletePath(root + "/missing"); err == nil {
		t.Error("DeletePath succeeded on a path that does not exist")
	}
}

// On a Paper Pro the root filesystem is read-only, and a change outside /home/root has to remount it writable and put it back afterward.
func TestFilesSystemPathRestoresReadOnlyRoot(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectApp(t, tablet)

	from := emitted.mark()
	if err := app.CreateDirectory("/opt/remanager-it"); err != nil {
		t.Fatalf("CreateDirectory on the root filesystem: %v", err)
	}
	if failures := emitted.count(from, "filesystem:restore-error"); failures > 0 {
		t.Errorf("%d filesystem:restore-error events for a restore that succeeded", failures)
	}
	if !tabletPathExists(tablet, "/opt/remanager-it") {
		t.Fatal("the folder was not created")
	}
	if mode := rootMountMode(tablet); mode != "ro" {
		t.Fatalf("root is %s after the change, want ro", mode)
	}

	// A file held open for writing makes the read-only remount fail with EBUSY.
	tablet.run("mount -o remount,rw / && (setsid sh -c 'echo $$ > /run/it-holder.pid; exec 3>>/var/tmp/it-holder; exec sleep 300' </dev/null >/dev/null 2>&1 &) && sleep 0.5")
	from = emitted.mark()
	if err := app.RenamePath("/opt/remanager-it", "/opt/remanager-it-renamed"); err != nil {
		t.Fatalf("RenamePath: %v", err)
	}
	if !tabletPathExists(tablet, "/opt/remanager-it-renamed") {
		t.Error("the rename did not happen")
	}
	event, _ := emitted.waitFor(t, from, 30*time.Second, "filesystem:restore-error")
	t.Logf("filesystem:restore-error: %v", event.Data)
	if mode := rootMountMode(tablet); mode != "rw" {
		t.Fatalf("root is %s while a writer holds it, so the failure was not provoked", mode)
	}

	tablet.run("kill $(cat /run/it-holder.pid); while kill -0 $(cat /run/it-holder.pid) 2>/dev/null; do sleep 0.1; done")
	if err := app.RetryRestoreFilesystem(); err != nil {
		t.Fatalf("RetryRestoreFilesystem: %v", err)
	}
	if mode := rootMountMode(tablet); mode != "ro" {
		t.Errorf("root is %s after the retry, want ro", mode)
	}
}

func TestFilesUploadSeveralFiles(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectApp(t, tablet)
	if err := app.CreateDirectory(transferTestDir); err != nil {
		t.Fatal(err)
	}

	local := t.TempDir()
	sizes := map[string]int{
		"notes with space.txt": 1234,
		"photo.png":            3 << 20,
		"split-upload.bin":     splitThresholdBytes + 8<<20,
	}
	hashes := map[string]string{}
	var paths []string
	for name, size := range sizes {
		path := filepath.Join(local, name)
		hashes[name] = writeRandomFile(t, path, size)
		paths = append(paths, path)
	}

	from := emitted.mark()
	app.UploadFilesFromPaths(paths, transferTestDir+"/")
	snapshot := waitForTransfersToSettle(t, app, 2*time.Minute)

	for name, size := range sizes {
		view := transferNamed(t, snapshot, name)
		if view.State != TransferDone || view.Kind != TransferUpload {
			t.Errorf("%s: %s %s (%s)", name, view.Kind, view.State, view.Error)
		}
		if view.BytesDone != int64(size) || view.BytesTotal != int64(size) {
			t.Errorf("%s: %d of %d bytes, want %d", name, view.BytesDone, view.BytesTotal, size)
		}
		if got := tabletFileHash(tablet, transferTestDir+"/"+name); got != hashes[name] {
			t.Errorf("%s on the tablet has sha256 %s, want %s", name, got, hashes[name])
		}
	}
	if finished := emitted.count(from, "transfers:finished"); finished != len(sizes) {
		t.Errorf("%d transfers:finished events, want %d", finished, len(sizes))
	}
	assertNoUploadLeftovers(t, tablet, transferTestDir)
}

func TestFilesUploadFolder(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectApp(t, tablet)
	if err := app.CreateDirectory(transferTestDir); err != nil {
		t.Fatal(err)
	}

	tree := filepath.Join(t.TempDir(), "Project")
	writeRandomFile(t, filepath.Join(tree, "a.txt"), 100)
	writeRandomFile(t, filepath.Join(tree, "sub", "b.bin"), 2<<20)
	writeRandomFile(t, filepath.Join(tree, "sub", "deeper", "c.txt"), 10)
	writeRandomFile(t, filepath.Join(tree, "sub", "deeper", "d e.txt"), 4096)
	if err := os.MkdirAll(filepath.Join(tree, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.txt", filepath.Join(tree, "link-to-a")); err != nil {
		t.Fatal(err)
	}

	app.UploadFilesFromPaths([]string{tree}, transferTestDir+"/")
	snapshot := waitForTransfersToSettle(t, app, 2*time.Minute)

	view := transferNamed(t, snapshot, "Project")
	if view.State != TransferDone || !view.IsDir {
		t.Fatalf("folder upload %s (dir=%t): %s", view.State, view.IsDir, view.Error)
	}
	if view.FilesTotal != 5 || view.FilesDone != 5 || len(view.FailedFiles) > 0 {
		t.Errorf("folder upload counted %d of %d files, failed %v; want 5 of 5", view.FilesDone, view.FilesTotal, view.FailedFiles)
	}

	remote := transferTestDir + "/Project"
	assertSameTree(t, tabletTreeHashes(tablet, remote), localTreeHashes(t, tree))
	if !tabletPathExists(tablet, remote+"/empty") {
		t.Error("the empty folder was not created")
	}
	if target := strings.TrimSpace(tablet.run("readlink " + remote + "/link-to-a")); target != "a.txt" {
		t.Errorf("symlink points at %q, want a.txt", target)
	}
	assertNoUploadLeftovers(t, tablet, remote)
}

// DownloadFile and DownloadFolder ask for the destination in a native dialog and then enqueue, so the tests enqueue with the destination the dialog would have returned.
func TestFilesDownload(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectApp(t, tablet)

	remote := "/home/root/it-downloads"
	tablet.run("mkdir -p " + remote + "/Folder/sub/empty && cd " + remote +
		" && head -c 5000000 /dev/urandom > big.bin && head -c 1000 /dev/urandom > Folder/a.bin" +
		" && head -c 300000 /dev/urandom > 'Folder/sub/b c.bin' && ln -s a.bin Folder/link")

	local := t.TempDir()
	manager := app.transferManager()
	manager.enqueue(TransferDownload, "big.bin", filepath.Join(local, "big.bin"), remote+"/big.bin", false)
	manager.enqueue(TransferDownload, "Folder", local, remote+"/Folder", true)
	snapshot := waitForTransfersToSettle(t, app, 2*time.Minute)

	for _, name := range []string{"big.bin", "Folder"} {
		if view := transferNamed(t, snapshot, name); view.State != TransferDone || len(view.FailedFiles) > 0 {
			t.Errorf("%s: %s %s, failed %v", name, view.State, view.Error, view.FailedFiles)
		}
	}
	if got, want := localFileHash(t, filepath.Join(local, "big.bin")), tabletFileHash(tablet, remote+"/big.bin"); got != want {
		t.Errorf("big.bin downloaded with sha256 %s, want %s", got, want)
	}
	assertSameTree(t, localTreeHashes(t, filepath.Join(local, "Folder")), tabletTreeHashes(tablet, remote+"/Folder"))
	if target, err := os.Readlink(filepath.Join(local, "Folder", "link")); err != nil || target != "a.bin" {
		t.Errorf("symlink downloaded as %q (%v), want a.bin", target, err)
	}
	if info, err := os.Stat(filepath.Join(local, "Folder", "sub", "empty")); err != nil || !info.IsDir() {
		t.Errorf("the empty folder was not downloaded: %v", err)
	}
}

func TestFilesTransferPanelCancelAndClear(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectApp(t, tablet)
	if err := app.CreateDirectory(transferTestDir); err != nil {
		t.Fatal(err)
	}

	local := t.TempDir()
	small := filepath.Join(local, "small.txt")
	writeRandomFile(t, small, 100)
	large := filepath.Join(local, "large-1.bin")
	file, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	file.Truncate(512 << 20)
	file.Close()
	largeNames := []string{"large-2.bin", "large-3.bin", "large-4.bin", "large-5.bin"}
	var largeCopies []string
	for _, name := range largeNames {
		path := filepath.Join(local, name)
		if err := os.Link(large, path); err != nil {
			t.Fatal(err)
		}
		largeCopies = append(largeCopies, path)
	}

	app.UploadFilesFromPaths([]string{small}, transferTestDir+"/")
	if view := transferNamed(t, waitForTransfersToSettle(t, app, time.Minute), "small.txt"); view.State != TransferDone {
		t.Fatalf("small.txt: %s %s", view.State, view.Error)
	}

	t.Run("cancel one", func(t *testing.T) {
		app.UploadFilesFromPaths([]string{large}, transferTestDir+"/")
		running := waitForTransferRunning(t, app, "large-1.bin")
		app.CancelTransfer(running.ID)
		view := transferNamed(t, waitForTransfersToSettle(t, app, time.Minute), "large-1.bin")
		if view.State != TransferCanceled {
			t.Fatalf("large-1.bin: %s, want canceled", view.State)
		}
		if view.BytesDone >= view.BytesTotal {
			t.Errorf("cancel landed after all %d bytes were sent", view.BytesTotal)
		}
		assertNoUploadLeftovers(t, tablet, transferTestDir, "large-1.bin")
	})

	t.Run("cancel all", func(t *testing.T) {
		// Four uploads against three workers leaves one still queued.
		app.UploadFilesFromPaths(largeCopies, transferTestDir+"/")
		waitForTransferRunning(t, app, "large-2.bin")
		app.CancelAllTransfers()
		snapshot := waitForTransfersToSettle(t, app, time.Minute)
		for _, name := range largeNames {
			if view := transferNamed(t, snapshot, name); view.State != TransferCanceled {
				t.Errorf("%s: %s, want canceled", name, view.State)
			}
		}
		if view := transferNamed(t, snapshot, "small.txt"); view.State != TransferDone {
			t.Errorf("canceling all turned the finished small.txt into %s", view.State)
		}
		assertNoUploadLeftovers(t, tablet, transferTestDir, largeNames...)
		if !tabletPathExists(tablet, transferTestDir+"/small.txt") {
			t.Error("canceling all removed a finished upload")
		}
	})

	t.Run("clear completed", func(t *testing.T) {
		if count := len(app.GetTransfers().Transfers); count != 6 {
			t.Fatalf("%d transfers listed before clearing, want 6", count)
		}
		app.ClearCompletedTransfers()
		if snapshot := app.GetTransfers(); len(snapshot.Transfers) != 0 || snapshot.ActiveCount != 0 {
			t.Errorf("transfers left after clearing: %+v", snapshot)
		}
	})
}

func prepareXochitlDir(tablet *fakeTablet) {
	tablet.run("mkdir -p " + pdfimport.XochitlPath)
}

type xochitlDocument struct {
	id       string
	metadata map[string]any
}

func xochitlDocuments(t *testing.T, tablet *fakeTablet) []xochitlDocument {
	t.Helper()
	out := tablet.run("cd " + pdfimport.XochitlPath + ` && for f in *.metadata; do [ -e "$f" ] || continue; printf '%s\t' "${f%.metadata}"; tr -d '\n' < "$f"; echo; done`)
	var documents []xochitlDocument
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		id, raw, found := strings.Cut(line, "\t")
		if !found {
			continue
		}
		document := xochitlDocument{id: id}
		if err := json.Unmarshal([]byte(raw), &document.metadata); err != nil {
			t.Fatalf("%s.metadata is not JSON: %v: %s", id, err, raw)
		}
		documents = append(documents, document)
	}
	return documents
}

func xochitlDocumentNamed(t *testing.T, tablet *fakeTablet, visibleName string) xochitlDocument {
	t.Helper()
	for _, document := range xochitlDocuments(t, tablet) {
		if document.metadata["visibleName"] == visibleName {
			return document
		}
	}
	t.Fatalf("no document named %q in %s:\n%s", visibleName, pdfimport.XochitlPath, tablet.run("ls -la "+pdfimport.XochitlPath))
	return xochitlDocument{}
}

func xochitlContent(t *testing.T, tablet *fakeTablet, id string) map[string]any {
	t.Helper()
	var content map[string]any
	raw := tablet.run(fmt.Sprintf("cat %s/%s.content", pdfimport.XochitlPath, id))
	if err := json.Unmarshal([]byte(raw), &content); err != nil {
		t.Fatalf("%s.content is not JSON: %v: %s", id, err, raw)
	}
	return content
}

func assertDocumentMetadata(t *testing.T, document xochitlDocument) {
	t.Helper()
	if document.metadata["type"] != "DocumentType" || document.metadata["parent"] != "" {
		t.Errorf("%s.metadata: type %v, parent %v; want DocumentType at the root", document.id, document.metadata["type"], document.metadata["parent"])
	}
}

func runningXochitlPID(tablet *fakeTablet) string {
	tablet.t.Helper()
	return strings.TrimSpace(tablet.run("pidof xochitl || true"))
}

// minimalPDF has a correct cross-reference table, so pdfcpu reads its page count rather than falling back to repair.
func minimalPDF(pages int) []byte {
	var objects []string
	var kids []string
	for i := 0; i < pages; i++ {
		kids = append(kids, fmt.Sprintf("%d 0 R", i+3))
	}
	objects = append(objects,
		"<< /Type /Catalog /Pages 2 0 R >>",
		fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), pages))
	for i := 0; i < pages; i++ {
		objects = append(objects, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>")
	}

	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, object := range objects {
		offsets[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return out.Bytes()
}

type zipEntry struct {
	name   string
	data   []byte
	stored bool
}

func zipArchive(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		if entry.stored {
			header.Method = zip.Store
		}
		part, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		part.Write(entry.data)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func minimalEPUB(t *testing.T, title string) []byte {
	t.Helper()
	return zipArchive(t, []zipEntry{
		{name: "mimetype", data: []byte("application/epub+zip"), stored: true},
		{name: "META-INF/container.xml", data: []byte(`<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`)},
		{name: "OEBPS/content.opf", data: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="id">urn:uuid:4f1e7a52-8a44-4c55-9c1b-3d2f0e6b7a10</dc:identifier>
    <dc:title>` + title + `</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">2026-01-01T00:00:00Z</meta>
  </metadata>
  <manifest>
    <item id="chapter" href="chapter.xhtml" media-type="application/xhtml+xml"/>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
  </manifest>
  <spine><itemref idref="chapter"/></spine>
</package>`)},
		{name: "OEBPS/nav.xhtml", data: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><head><title>Contents</title></head>
<body><nav epub:type="toc"><ol><li><a href="chapter.xhtml">Chapter</a></li></ol></nav></body></html>`)},
		{name: "OEBPS/chapter.xhtml", data: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>Chapter</title></head><body><p>Hello from the integration suite.</p></body></html>`)},
	})
}

const rmdocSourceID = "0b6d3b1e-5f6a-4c3e-9d2b-7a8e1f0c4d21"

// An rmdoc is the xochitl files of one document zipped with the document's UUID in every name; writers commonly omit folder entries.
func rmdocEntries(pageFolders ...string) []zipEntry {
	entries := []zipEntry{
		{name: rmdocSourceID + ".metadata", data: []byte(`{"createdTime":"1700000000000","lastModified":"1700000000000","lastOpened":"0","lastOpenedPage":0,"parent":"","pinned":false,"type":"DocumentType","visibleName":"Exported Notebook"}`)},
		{name: rmdocSourceID + ".content", data: []byte(`{"fileType":"notebook","formatVersion":2,"pageCount":2,"cPages":{"pages":[{"id":"page-one"},{"id":"page-two"}]}}`)},
	}
	for _, folder := range pageFolders {
		extension := ".rm"
		if folder == ".highlights" {
			extension = ".json"
		}
		for _, page := range []string{"page-one", "page-two"} {
			data := make([]byte, 2048)
			rand.Read(data)
			entries = append(entries, zipEntry{name: rmdocSourceID + folder + "/" + page + extension, data: data})
		}
	}
	return entries
}

func importDocuments(t *testing.T, app *testApp, documents []ImportDocument, rescanLibrary, restartUI bool) error {
	t.Helper()
	_, err := runQuietOp(t, app, 2*time.Minute, func(id string) { app.ImportDocuments(id, documents, rescanLibrary, restartUI) })
	return err
}

func TestImportPDF(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectApp(t, tablet)
	prepareXochitlDir(tablet)
	local := t.TempDir()

	t.Run("single, restarting the UI", func(t *testing.T) {
		pdf := minimalPDF(3)
		path := filepath.Join(local, "Single.pdf")
		digest := writeLocalFile(t, path, pdf)

		info, err := app.GetImportFileInfo(path)
		if err != nil {
			t.Fatalf("GetImportFileInfo: %v", err)
		}
		if info.FileType != "pdf" || info.PageCount != 3 || info.Size != int64(len(pdf)) {
			t.Fatalf("GetImportFileInfo: %+v, want a 3-page pdf", info)
		}

		pid := runningXochitlPID(tablet)
		firstPageCover := 0
		if err := importDocuments(t, app, []ImportDocument{{Path: path, VisibleName: "IT Single", CoverPageNumber: &firstPageCover}}, false, true); err != nil {
			t.Fatalf("ImportDocuments: %v", err)
		}

		document := xochitlDocumentNamed(t, tablet, "IT Single")
		assertDocumentMetadata(t, document)
		base := pdfimport.XochitlPath + "/" + document.id
		if got := tabletFileHash(tablet, base+".pdf"); got != digest {
			t.Errorf("%s.pdf differs from the source", document.id)
		}
		if !tabletPathExists(tablet, base+".pagedata") {
			t.Errorf("%s.pagedata missing", document.id)
		}
		content := xochitlContent(t, tablet, document.id)
		pages, _ := content["pages"].([]any)
		if content["fileType"] != "pdf" || content["pageCount"] != 3.0 || len(pages) != 3 || content["coverPageNumber"] != 0.0 {
			t.Errorf("%s.content: fileType %v, pageCount %v, %d pages, coverPageNumber %v", document.id, content["fileType"], content["pageCount"], len(pages), content["coverPageNumber"])
		}
		if now := runningXochitlPID(tablet); now == "" || now == pid {
			t.Errorf("xochitl pid %q before, %q after; want a new running process", pid, now)
		}
	})

	t.Run("several, without restarting", func(t *testing.T) {
		pid := runningXochitlPID(tablet)
		pageCounts := map[string]int{"IT Two Pages": 2, "IT Five Pages": 5}
		lastPageCover := -1
		for name, pages := range pageCounts {
			path := filepath.Join(local, name+".pdf")
			writeLocalFile(t, path, minimalPDF(pages))
			if err := importDocuments(t, app, []ImportDocument{{Path: path, VisibleName: name, CoverPageNumber: &lastPageCover}}, false, false); err != nil {
				t.Fatalf("ImportDocuments(%s): %v", name, err)
			}
		}
		for name, pages := range pageCounts {
			document := xochitlDocumentNamed(t, tablet, name)
			if content := xochitlContent(t, tablet, document.id); content["pageCount"] != float64(pages) {
				t.Errorf("%s: pageCount %v, want %d", name, content["pageCount"], pages)
			}
		}
		if len(xochitlDocuments(t, tablet)) != 3 {
			t.Errorf("%d documents on the tablet, want 3", len(xochitlDocuments(t, tablet)))
		}
		if now := runningXochitlPID(tablet); now != pid {
			t.Errorf("xochitl pid %q before, %q after; want it left running without being asked", pid, now)
		}
	})

	t.Run("not a PDF", func(t *testing.T) {
		path := filepath.Join(local, "broken.pdf")
		writeLocalFile(t, path, []byte("not a pdf"))
		if err := importDocuments(t, app, []ImportDocument{{Path: path, VisibleName: "IT Broken"}}, false, false); err == nil {
			t.Error("ImportDocuments accepted a file with no readable page count")
		}
	})
}

func TestImportEpub(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectApp(t, tablet)
	prepareXochitlDir(tablet)

	path := filepath.Join(t.TempDir(), "Book.epub")
	digest := writeLocalFile(t, path, minimalEPUB(t, "Integration Book"))
	info, err := app.GetImportFileInfo(path)
	if err != nil || info.FileType != "epub" {
		t.Fatalf("GetImportFileInfo: %+v, %v", info, err)
	}

	pid := runningXochitlPID(tablet)
	if err := importDocuments(t, app, []ImportDocument{{Path: path, VisibleName: "IT Book"}}, false, true); err != nil {
		t.Fatalf("ImportDocuments: %v", err)
	}
	document := xochitlDocumentNamed(t, tablet, "IT Book")
	assertDocumentMetadata(t, document)
	if got := tabletFileHash(tablet, pdfimport.XochitlPath+"/"+document.id+".epub"); got != digest {
		t.Errorf("%s.epub differs from the source", document.id)
	}
	if content := xochitlContent(t, tablet, document.id); content["fileType"] != "epub" {
		t.Errorf("%s.content fileType %v, want epub", document.id, content["fileType"])
	}
	if now := runningXochitlPID(tablet); now == "" || now == pid {
		t.Errorf("xochitl pid %q before, %q after; want a new running process", pid, now)
	}
}

func TestImportRmdoc(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectApp(t, tablet)
	prepareXochitlDir(tablet)
	local := t.TempDir()

	importRmdoc := func(t *testing.T, fileName, visibleName string, entries []zipEntry) xochitlDocument {
		t.Helper()
		path := filepath.Join(local, fileName)
		writeLocalFile(t, path, zipArchive(t, entries))

		info, err := app.GetImportFileInfo(path)
		if err != nil {
			t.Fatalf("GetImportFileInfo: %v", err)
		}
		if info.FileType != "rmdoc" || info.VisibleName != "Exported Notebook" || info.PageCount != 2 {
			t.Errorf("GetImportFileInfo: %+v", info)
		}

		pid := runningXochitlPID(tablet)
		if err := importDocuments(t, app, []ImportDocument{{Path: path, VisibleName: visibleName}}, false, false); err != nil {
			t.Fatalf("ImportDocuments: %v", err)
		}
		if now := runningXochitlPID(tablet); now != pid {
			t.Errorf("xochitl pid %q before, %q after; want it left running without being asked", pid, now)
		}

		document := xochitlDocumentNamed(t, tablet, visibleName)
		if document.id == rmdocSourceID {
			t.Error("the import kept the source UUID; a second import would overwrite the first")
		}
		assertDocumentMetadata(t, document)
		if document.metadata["lastModified"] == "1700000000000" {
			t.Error("lastModified was not updated")
		}
		for _, entry := range entries {
			remote := pdfimport.XochitlPath + "/" + strings.Replace(entry.name, rmdocSourceID, document.id, 1)
			if strings.HasSuffix(entry.name, ".metadata") {
				continue
			}
			if !tabletPathExists(tablet, remote) {
				t.Errorf("%s missing on the tablet", remote)
			} else if got := tabletFileHash(tablet, remote); got != sha256Hex(entry.data) {
				t.Errorf("%s differs from the archive", remote)
			}
		}
		return document
	}

	t.Run("pages", func(t *testing.T) {
		importRmdoc(t, "Notebook.rmdoc", "IT Notebook", rmdocEntries(""))
	})

	t.Run("pages and highlights", func(t *testing.T) {
		importRmdoc(t, "Highlighted.rmdoc", "IT Highlighted", rmdocEntries("", ".highlights"))
	})
}

func startFakeXoviBroker(tablet *fakeTablet) {
	tablet.run(`mkdir -p /etc/systemd/system/xochitl.service.d /home/root/xovi
printf 'Environment="LD_PRELOAD=/home/root/xovi/xovi.so"\n' > /etc/systemd/system/xochitl.service.d/xovi.conf
systemctl restart xochitl
mkfifo /run/xovi-mb /run/xovi-mb-out
(setsid bash -c 'while :; do read -r request < /run/xovi-mb; echo "$request" >> /run/xovi-mb.requests; echo ok > /run/xovi-mb-out; done' </dev/null >/dev/null 2>&1 &)`)
}

func TestImportRescanLibrary(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectApp(t, tablet)

	rescan := func(id string) { app.RescanLibrary(id) }
	if _, err := runQuietOp(t, app, time.Minute, rescan); err == nil || !strings.Contains(err.Error(), "mods are not running") {
		t.Errorf("RescanLibrary without xovi: %v, want mods are not running", err)
	}

	startFakeXoviBroker(tablet)
	if _, err := runQuietOp(t, app, time.Minute, rescan); err != nil {
		t.Fatalf("RescanLibrary: %v", err)
	}
	if requests := tablet.run("cat /run/xovi-mb.requests"); strings.TrimSpace(requests) != ">erescanLibrary:" {
		t.Errorf("the message broker received %q", requests)
	}
}

func TestConfigEditor(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectApp(t, tablet)

	original, err := app.ReadConfigFile()
	if err != nil {
		t.Fatalf("ReadConfigFile: %v", err)
	}
	if original != tablet.run("cat "+configPath) {
		t.Fatalf("ReadConfigFile returned %q", original)
	}

	edited := "[General]\nSleepScreenPath=/usr/share/remarkable/suspended.png\nwifion=true\n"
	writeConfig := func(content string) error {
		_, err := runQuietOp(t, app, time.Minute, func(id string) { app.WriteConfigFile(id, content) })
		return err
	}
	restoreBackup := func(name string) error {
		_, err := runQuietOp(t, app, time.Minute, func(id string) { app.RestoreConfigBackup(id, name) })
		return err
	}
	if err := writeConfig(edited); err != nil {
		t.Fatalf("WriteConfigFile: %v", err)
	}
	if got := tablet.run("cat " + configPath); got != edited {
		t.Fatalf("xochitl.conf holds %q after saving", got)
	}

	backupName, err := runQuietOp(t, app, time.Minute, func(id string) { app.BackupConfigFile(id) })
	if err != nil {
		t.Fatalf("BackupConfigFile: %v", err)
	}
	if !strings.HasPrefix(backupName, "xochitl.conf.backup-") {
		t.Errorf("backup named %q", backupName)
	}

	if err := writeConfig("[General]\nwifion=false\n"); err != nil {
		t.Fatalf("WriteConfigFile: %v", err)
	}
	if err := writeConfig("[General\nwifion=true\n"); err == nil {
		t.Error("WriteConfigFile accepted an unclosed section")
	}
	if got := tablet.run("cat " + configPath); got != "[General]\nwifion=false\n" {
		t.Errorf("a rejected save changed xochitl.conf to %q", got)
	}

	backups, err := app.ListConfigBackups()
	if err != nil {
		t.Fatalf("ListConfigBackups: %v", err)
	}
	if len(backups) != 1 || backups[0].Name != backupName || backups[0].Size != int64(len(edited)) {
		t.Fatalf("ListConfigBackups: %+v, want %s of %d bytes", backups, backupName, len(edited))
	}

	if err := restoreBackup(backupName); err != nil {
		t.Fatalf("RestoreConfigBackup: %v", err)
	}
	restored, err := app.ReadConfigFile()
	if err != nil || restored != edited {
		t.Errorf("after restoring, ReadConfigFile: %q, %v; want %q", restored, err, edited)
	}
	if err := restoreBackup("../../../etc/passwd"); err == nil {
		t.Error("RestoreConfigBackup accepted a name outside the backups")
	}
	if leftovers := strings.TrimSpace(tablet.run("ls " + configDir)); leftovers != "xochitl.conf\n"+backupName && leftovers != backupName+"\nxochitl.conf" {
		t.Errorf("config folder holds:\n%s", leftovers)
	}
}

func shellOutputSince(from int) string {
	var out strings.Builder
	for _, event := range emitted.since(from) {
		if text, ok := event.Data.(string); ok && event.Name == "shell:output" {
			out.WriteString(text)
		}
	}
	return out.String()
}

func waitForShellOutput(t *testing.T, from int, want string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !strings.Contains(shellOutputSince(from), want) {
		if time.Now().After(deadline) {
			t.Fatalf("shell output never contained %q:\n%s", want, shellOutputSince(from))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func shellActive(app *testApp) bool {
	app.shellMu.Lock()
	defer app.shellMu.Unlock()
	return app.shellActive
}

func TestTerminalShell(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectApp(t, tablet)

	from := emitted.mark()
	if err := app.StartShell(24, 80); err != nil {
		t.Fatalf("StartShell: %v", err)
	}
	t.Cleanup(app.StopShell)
	if !shellActive(app) {
		t.Error("the shell is not active with a shell running")
	}
	if err := app.StartShell(24, 80); err == nil {
		t.Error("a second StartShell succeeded while one is running")
	}

	if err := app.WriteToShell("echo it-$((6*7))\n"); err != nil {
		t.Fatalf("WriteToShell: %v", err)
	}
	waitForShellOutput(t, from, "it-42")

	if err := app.ResizeShell(40, 132); err != nil {
		t.Fatalf("ResizeShell: %v", err)
	}
	app.WriteToShell("stty size\n")
	waitForShellOutput(t, from, "40 132")

	app.StopShell()
	emitted.waitFor(t, from, 10*time.Second, "shell:stopped")
	if shellActive(app) {
		t.Error("the shell is still active after StopShell")
	}
	if err := app.WriteToShell("echo late\n"); err == nil {
		t.Error("WriteToShell succeeded after the shell stopped")
	}
}

func arrangeTabletDocuments(tablet *fakeTablet) {
	tablet.run("mkdir -p " + pdfimport.XochitlPath + "/nb-1111 && cd " + pdfimport.XochitlPath +
		` && printf '{"visibleName":"Notebook","type":"DocumentType","parent":""}' > nb-1111.metadata` +
		` && printf '{"fileType":"notebook"}' > nb-1111.content` +
		" && head -c 70000 /dev/urandom > nb-1111/page-a.rm && head -c 90000 /dev/urandom > nb-1111/page-b.rm" +
		` && printf '{"visibleName":"Paper","type":"DocumentType","parent":""}' > pdf-2222.metadata` +
		" && head -c 2000000 /dev/urandom > pdf-2222.pdf" +
		" && printf '[General]\\nwifion=true\\n' > " + configPath)
}

func backupArchiveHashes(t *testing.T, archivePath string) map[string]string {
	t.Helper()
	file, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoder, err := zstd.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.Close()
	hashes := map[string]string{}
	reader := tar.NewReader(decoder)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return hashes
		}
		if err != nil {
			t.Fatalf("read %s: %v", archivePath, err)
		}
		if header.Typeflag == tar.TypeReg {
			data, _ := io.ReadAll(reader)
			hashes[header.Name] = sha256Hex(data)
		}
	}
}

func TestBackupCreateAndRestore(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app, deviceID := connectSavedTablet(t, tablet, "IT Backup Tablet")
	arrangeTabletDocuments(tablet)
	documents := tabletTreeHashes(tablet, pdfimport.XochitlPath)
	configHash := tabletFileHash(tablet, configPath)

	archive := filepath.Join(t.TempDir(), "remarkable-backup-it.tar.zst")
	if path, err := runQuietOp(t, app, 2*time.Minute, func(id string) { app.CreateDeviceBackup(id, archive) }); err != nil {
		t.Fatalf("backup failed: %v", err)
	} else if path != archive {
		t.Errorf("backup finished with path %q, want %q", path, archive)
	}

	metadata, err := backup.ReadBackupMetadata(archive)
	if err != nil || metadata == nil {
		t.Fatalf("ReadBackupMetadata: %+v, %v", metadata, err)
	}
	if metadata.DeviceID != deviceID || metadata.DeviceName != "IT Backup Tablet" {
		t.Errorf("backup records device %q named %q, want %q named IT Backup Tablet", metadata.DeviceID, metadata.DeviceName, deviceID)
	}

	entries := backupArchiveHashes(t, archive)
	for name, digest := range documents {
		if got := entries[strings.TrimPrefix(pdfimport.XochitlPath, "/")+"/"+name]; got != digest {
			t.Errorf("document %s: archived sha256 %q, want %q", name, got, digest)
		}
	}
	if got := entries[strings.TrimPrefix(configPath, "/")]; got != configHash {
		t.Errorf("xochitl.conf: archived sha256 %q, want %q", got, configHash)
	}

	tablet.run("rm -rf " + pdfimport.XochitlPath + "/* && printf '[General]\\nwifion=false\\n' > " + configPath)
	pid := runningXochitlPID(tablet)

	from := emitted.mark()
	if _, err := runQuietOp(t, app, 2*time.Minute, func(id string) { app.RestoreDeviceBackup(id, archive) }); err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	assertSameTree(t, tabletTreeHashes(tablet, pdfimport.XochitlPath), documents)
	if got := tabletFileHash(tablet, configPath); got != configHash {
		t.Error("xochitl.conf was not restored")
	}

	var phases []backup.Phase
	for _, event := range emitted.since(from) {
		if progress, ok := event.Data.(OperationProgress); ok && progress.Backup != nil && !slices.Contains(phases, progress.Backup.Phase) {
			phases = append(phases, progress.Backup.Phase)
		}
	}
	if !slices.Contains(phases, backup.PhaseStoppingUI) || !slices.Contains(phases, backup.PhaseStartingUI) {
		t.Errorf("restore phases %v, want the UI stopped and started around the upload", phases)
	}
	if now := runningXochitlPID(tablet); now == "" || now == pid {
		t.Errorf("xochitl pid %q before, %q after the restore; want it stopped and started again", pid, now)
	}
}

func TestBackupCancelLargeBackup(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectApp(t, tablet)
	tablet.run("mkdir -p " + pdfimport.XochitlPath + " && cd " + pdfimport.XochitlPath +
		" && for i in $(seq 200); do head -c 1048576 /dev/urandom > doc-$i.pdf; done")

	archive := filepath.Join(t.TempDir(), "remarkable-backup-canceled.tar.zst")
	id := fmt.Sprintf("op-%d", time.Now().UnixNano())
	next := emitted.mark()
	app.CreateDeviceBackup(id, archive)
	var result OperationResult
	canceled := false
	for {
		event, after := emitted.waitFor(t, next, 2*time.Minute, "op:progress", "op:done")
		next = after
		if done, ok := event.Data.(OperationResult); ok && done.ID == id {
			result = done
			break
		}
		progress, ok := event.Data.(OperationProgress)
		if canceled || !ok || progress.ID != id || progress.Backup == nil {
			continue
		}
		if progress.Backup.Phase == backup.PhaseDownloading && progress.Backup.FilesDone > 0 {
			app.CancelOperation(id)
			canceled = true
		}
	}
	if !canceled {
		t.Fatalf("the backup ended before it could be canceled: %+v", result)
	}
	if result.OK || !result.Canceled {
		t.Errorf("canceled backup finished with %+v, want a cancellation", result)
	}

	if _, err := os.Stat(archive); err == nil {
		files := len(backupArchiveHashes(t, archive))
		t.Errorf("a canceled backup left %s on disk, a readable archive of %d of 201 files that looks like a finished backup", filepath.Base(archive), files)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		app.backupMu.Lock()
		active := app.backupActive
		app.backupMu.Unlock()
		if !active {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the backup slot is still held after the cancellation")
		}
		time.Sleep(50 * time.Millisecond)
	}
	for strings.TrimSpace(tablet.run("pidof tar || true")) != "" {
		if time.Now().After(deadline) {
			t.Error("tar is still running on the tablet after the cancellation")
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
}
