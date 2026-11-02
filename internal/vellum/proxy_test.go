package vellum

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/sftp"
)

func localSFTPClient(t *testing.T) *sftp.Client {
	t.Helper()
	serverReads, clientWrites := io.Pipe()
	clientReads, serverWrites := io.Pipe()
	server, err := sftp.NewServer(struct {
		io.Reader
		io.WriteCloser
	}{serverReads, serverWrites})
	if err != nil {
		t.Fatal(err)
	}
	go server.Serve()
	client, err := sftp.NewClientPipe(clientReads, clientWrites)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		serverWrites.Close()
		client.Close()
		server.Close()
	})
	return client
}

func TestWriteFileAtomicallyReplacesTheFileWhole(t *testing.T) {
	client := localSFTPClient(t)
	remotePath := filepath.ToSlash(filepath.Join(t.TempDir(), "cache", "APKINDEX.tar.gz"))

	if err := writeFileAtomically(client, remotePath, []byte("old index")); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomically(client, remotePath, []byte("new index")); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.FromSlash(remotePath))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new index" {
		t.Errorf("file holds %q, want %q", got, "new index")
	}
	if _, err := os.Stat(filepath.FromSlash(remotePath + ".tmp")); !os.IsNotExist(err) {
		t.Errorf("temporary file left behind: %v", err)
	}
}
