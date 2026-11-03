package pdfimport

import (
	"errors"
	"io"
	"net"
	"os"
	"path"
	"strings"
	"sync"
	"testing"

	"github.com/pkg/sftp"
)

type recordingWriter struct {
	sftp.FileWriter
	mu         sync.Mutex
	opened     []string
	failSuffix string
}

func (w *recordingWriter) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	w.mu.Lock()
	w.opened = append(w.opened, r.Filepath)
	w.mu.Unlock()
	if w.failSuffix != "" && strings.HasSuffix(r.Filepath, w.failSuffix) {
		return nil, errors.New("disk full")
	}
	return w.FileWriter.Filewrite(r)
}

func inMemoryTablet(t *testing.T) (*sftp.Client, *recordingWriter) {
	t.Helper()
	handlers := sftp.InMemHandler()
	writer := &recordingWriter{FileWriter: handlers.FilePut}
	handlers.FilePut = writer

	serverConn, clientConn := net.Pipe()
	server := sftp.NewRequestServer(serverConn, handlers)
	go server.Serve()
	t.Cleanup(func() { server.Close() })

	client, err := sftp.NewClientPipe(clientConn, clientConn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	if err := client.MkdirAll(XochitlPath); err != nil {
		t.Fatal(err)
	}
	return client, writer
}

func xochitlEntries(t *testing.T, client *sftp.Client) []string {
	t.Helper()
	entries, err := client.ReadDir(XochitlPath)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestPDFUploadWritesMetadataLast(t *testing.T) {
	client, writer := inMemoryTablet(t)

	if _, err := Upload(client, []byte("%PDF-1.4"), "Doc", "", 1, nil); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	if len(writer.opened) != 4 || !strings.HasSuffix(writer.opened[3], ".metadata") {
		t.Fatalf("write order = %v, want .metadata last", writer.opened)
	}
}

func TestFailedPDFUploadLeavesNothingBehind(t *testing.T) {
	client, writer := inMemoryTablet(t)
	writer.failSuffix = ".pagedata"

	if _, err := Upload(client, []byte("%PDF-1.4"), "Doc", "", 1, nil); err == nil {
		t.Fatal("Upload succeeded with a failing write")
	}

	if names := xochitlEntries(t, client); len(names) != 0 {
		t.Fatalf("failed import left %v in xochitl", names)
	}
}

func TestDiscardKeepsDirectoriesThatAlreadyExisted(t *testing.T) {
	client, _ := inMemoryTablet(t)
	existing := path.Join(XochitlPath, "existing.thumbnails")
	if err := client.Mkdir(existing); err != nil {
		t.Fatal(err)
	}

	upload := NewDocumentUpload(client)
	if err := upload.MkdirAll(existing); err != nil {
		t.Fatal(err)
	}
	if err := upload.MkdirAll(path.Join(XochitlPath, "new", "pages")); err != nil {
		t.Fatal(err)
	}
	if err := upload.WriteFile(path.Join(XochitlPath, "new", "pages", "a.rm"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	upload.Discard()

	if _, err := client.Stat(existing); err != nil {
		t.Fatalf("Discard removed a directory it did not create: %v", err)
	}
	if _, err := client.Stat(path.Join(XochitlPath, "new")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Discard left the directory it created: stat err = %v", err)
	}
}
