package rmdocimport

import (
	"archive/zip"
	"bytes"
	"net"
	"path"
	"testing"

	"github.com/pkg/sftp"

	"reManager/internal/pdfimport"
)

const docUUID = "0b2b3f1e-4c55-4b8e-9d2a-2f6c2b0d7a10"

func TestBelongsToDocument(t *testing.T) {
	accepted := []string{
		docUUID + ".metadata",
		docUUID + ".content",
		docUUID + "/",
		docUUID + "/page.rm",
	}
	for _, name := range accepted {
		if !belongsToDocument(name, docUUID) {
			t.Errorf("rejected %q", name)
		}
	}

	rejected := []string{
		docUUID + "/../../../../etc/profile.d/x.sh",
		"../" + docUUID + ".metadata",
		"/" + docUUID + ".metadata",
		docUUID + `\..\x`,
		docUUID + "//page.rm",
		"payload.sh",
		"other.metadata",
	}
	for _, name := range rejected {
		if belongsToDocument(name, docUUID) {
			t.Errorf("accepted %q", name)
		}
	}
}

func TestBelongsToDocumentWithDotDotUUID(t *testing.T) {
	if belongsToDocument("../etc/profile", "..") {
		t.Fatal("a .. document UUID let a parent path through")
	}
}

func TestUploadCreatesEveryFolderWithoutDirectoryEntries(t *testing.T) {
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	for _, name := range []string{
		docUUID + ".content",
		docUUID + "/page.rm",
		docUUID + ".highlights/page.json",
		docUUID + ".metadata",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte("{}"))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	serverConn, clientConn := net.Pipe()
	server := sftp.NewRequestServer(serverConn, sftp.InMemHandler())
	go server.Serve()
	t.Cleanup(func() { server.Close() })
	client, err := sftp.NewClientPipe(clientConn, clientConn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	if err := client.MkdirAll(pdfimport.XochitlPath); err != nil {
		t.Fatal(err)
	}

	newUUID, err := Upload(client, archive.Bytes(), "Doc")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	for _, name := range []string{newUUID + "/page.rm", newUUID + ".highlights/page.json", newUUID + ".metadata"} {
		if _, err := client.Stat(path.Join(pdfimport.XochitlPath, name)); err != nil {
			t.Errorf("%s was not written: %v", name, err)
		}
	}
}
