package pdfimport

import (
	"errors"
	"io"
	"net"
	"os"
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

type recordingRenamer struct {
	sftp.FileCmder
	mu         sync.Mutex
	renamed    []string
	failSuffix string
}

func (c *recordingRenamer) PosixRename(r *sftp.Request) error {
	c.mu.Lock()
	c.renamed = append(c.renamed, r.Target)
	c.mu.Unlock()
	if c.failSuffix != "" && strings.HasSuffix(r.Target, c.failSuffix) {
		return errors.New("rename refused")
	}
	return c.FileCmder.(sftp.PosixRenameFileCmder).PosixRename(r)
}

func inMemoryTablet(t *testing.T) (*sftp.Client, *recordingWriter, *recordingRenamer) {
	t.Helper()
	handlers := sftp.InMemHandler()
	writer := &recordingWriter{FileWriter: handlers.FilePut}
	handlers.FilePut = writer
	renamer := &recordingRenamer{FileCmder: handlers.FileCmd}
	handlers.FileCmd = renamer

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
	return client, writer, renamer
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

func assertNoStagingLeft(t *testing.T, client *sftp.Client) {
	t.Helper()
	if _, err := client.Stat(ImportStagingPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging directory still exists: stat err = %v", err)
	}
}

func TestPDFUploadPublishesMetadataLast(t *testing.T) {
	client, writer, renamer := inMemoryTablet(t)

	if _, err := Upload(client, []byte("%PDF-1.4"), "Doc", "", 1, nil); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	for _, opened := range writer.opened {
		if !strings.HasPrefix(opened, ImportStagingPath+"/") {
			t.Fatalf("wrote %s outside staging", opened)
		}
	}
	if len(renamer.renamed) != 4 || !strings.HasSuffix(renamer.renamed[3], ".metadata") {
		t.Fatalf("publish order = %v, want .metadata last", renamer.renamed)
	}
	if names := xochitlEntries(t, client); len(names) != 4 {
		t.Fatalf("xochitl holds %v, want the 4 document files", names)
	}
	assertNoStagingLeft(t, client)
}

func TestFailedPDFUploadLeavesNothingBehind(t *testing.T) {
	client, writer, _ := inMemoryTablet(t)
	writer.failSuffix = ".pagedata"

	if _, err := Upload(client, []byte("%PDF-1.4"), "Doc", "", 1, nil); err == nil {
		t.Fatal("Upload succeeded with a failing write")
	}

	if names := xochitlEntries(t, client); len(names) != 0 {
		t.Fatalf("failed import left %v in xochitl", names)
	}
	assertNoStagingLeft(t, client)
}

func TestFailedPublishRemovesWhatItMoved(t *testing.T) {
	client, _, renamer := inMemoryTablet(t)
	renamer.failSuffix = ".metadata"

	if _, err := Upload(client, []byte("%PDF-1.4"), "Doc", "", 1, nil); err == nil {
		t.Fatal("Upload succeeded with a failing rename")
	}

	if names := xochitlEntries(t, client); len(names) != 0 {
		t.Fatalf("failed publish left %v in xochitl", names)
	}
	assertNoStagingLeft(t, client)
}

func TestNextImportClearsAnInterruptedUpload(t *testing.T) {
	client, _, _ := inMemoryTablet(t)

	interrupted, err := NewDocumentUpload(client, "interrupted")
	if err != nil {
		t.Fatal(err)
	}
	if err := interrupted.WriteFile("interrupted.pdf", []byte("%PDF")); err != nil {
		t.Fatal(err)
	}
	if names := xochitlEntries(t, client); len(names) != 0 {
		t.Fatalf("unpublished upload reached xochitl: %v", names)
	}

	if _, err := Upload(client, []byte("%PDF-1.4"), "Doc", "", 1, nil); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	assertNoStagingLeft(t, client)
}
