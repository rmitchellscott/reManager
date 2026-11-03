package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"golang.org/x/crypto/ssh"
)

func deviceTarStream(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(content)), Mode: 0644, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func fakeDevice(t *testing.T, fileCount string, tarStream []byte, tarExitStatus uint32) *ssh.Client {
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

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		_, channels, requests, err := ssh.NewServerConn(conn, config)
		if err != nil {
			return
		}
		go ssh.DiscardRequests(requests)
		for newChannel := range channels {
			channel, channelRequests, err := newChannel.Accept()
			if err != nil {
				continue
			}
			go func() {
				defer channel.Close()
				for req := range channelRequests {
					if req.Type != "exec" {
						req.Reply(false, nil)
						continue
					}
					req.Reply(true, nil)
					command := string(req.Payload[4:])
					exitStatus := uint32(0)
					if strings.HasPrefix(command, "find ") {
						io.WriteString(channel, fileCount+"\n")
					} else {
						channel.Write(tarStream)
						exitStatus = tarExitStatus
					}
					status := make([]byte, 4)
					binary.BigEndian.PutUint32(status, exitStatus)
					channel.SendRequest("exit-status", false, status)
					return
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
	t.Cleanup(func() { client.Close() })
	return client
}

func TestBackupWritesTheDeviceIdentityAndACompleteArchive(t *testing.T) {
	stream := deviceTarStream(t, map[string]string{"/home/root/.config/remarkable/xochitl.conf": "[General]\n"})
	dest := filepath.Join(t.TempDir(), "backup.tar.zst")
	manager := Manager{
		SSHClient:  fakeDevice(t, "1", stream, 0),
		CancelCh:   make(chan struct{}),
		DeviceName: "Paper Pro",
		DeviceID:   "saved-id",
		MachineID:  "machine-id",
	}

	if err := manager.CreateBackup(dest, []string{"/home/root/.config/remarkable"}); err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	metadata, err := ReadBackupMetadata(dest)
	if err != nil {
		t.Fatalf("ReadBackupMetadata: %v", err)
	}
	if metadata.DeviceName != "Paper Pro" || metadata.DeviceID != "saved-id" || metadata.MachineID != "machine-id" {
		t.Fatalf("metadata = %+v, want the device identity", metadata)
	}

	archive, err := os.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	decoder, err := zstd.NewReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.Close()
	var names []string
	reader := tar.NewReader(decoder)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("archive does not read to the end: %v", err)
		}
		names = append(names, header.Name)
	}
	if len(names) != 2 {
		t.Fatalf("archive entries = %v, want the metadata and one file", names)
	}
}

func TestFailedBackupRemovesItsArchive(t *testing.T) {
	stream := deviceTarStream(t, map[string]string{"/home/root/.config/remarkable/xochitl.conf": "[General]\n"})
	cases := map[string]*Manager{
		"truncated stream":  {SSHClient: fakeDevice(t, "1", stream[:len(stream)/2], 0), CancelCh: make(chan struct{})},
		"tar exits nonzero": {SSHClient: fakeDevice(t, "1", stream, 2), CancelCh: make(chan struct{})},
	}
	for name, manager := range cases {
		t.Run(name, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "backup.tar.zst")
			if err := manager.CreateBackup(dest, []string{"/home/root/.config/remarkable"}); err == nil {
				t.Fatal("CreateBackup succeeded")
			}
			if _, err := os.Stat(dest); !os.IsNotExist(err) {
				t.Fatalf("partial archive left behind: stat err = %v", err)
			}
		})
	}
}

func TestCanceledBackupRemovesItsArchive(t *testing.T) {
	stream := deviceTarStream(t, map[string]string{"/home/root/.config/remarkable/xochitl.conf": "[General]\n"})
	canceled := make(chan struct{})
	dest := filepath.Join(t.TempDir(), "backup.tar.zst")
	manager := Manager{
		SSHClient: fakeDevice(t, "1", stream, 0),
		CancelCh:  canceled,
		ProgressFn: func(p Progress) {
			if p.Phase == "downloading" && p.FilesDone == 0 {
				select {
				case <-canceled:
				default:
					close(canceled)
				}
			}
		},
	}

	if err := manager.CreateBackup(dest, []string{"/home/root/.config/remarkable"}); err == nil {
		t.Fatal("canceled backup reported success")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("partial archive left behind: stat err = %v", err)
	}
}

func TestRestoreTargetKeepsEntriesUnderTheDeviceRoots(t *testing.T) {
	m := &Manager{}
	accepted := map[string]string{
		"home/root/.local/share/remarkable/xochitl/abc.metadata": "home/root/.local/share/remarkable/xochitl/abc.metadata",
		"home/root/.config/remarkable/xochitl.conf":              "home/root/.config/remarkable/xochitl.conf",
	}
	for name, want := range accepted {
		got, err := m.restoreTarget(&tar.Header{Name: name, Typeflag: tar.TypeReg})
		if err != nil {
			t.Errorf("%q: %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("%q: got %q, want %q", name, got, want)
		}
	}

	rejected := []string{
		"/etc/profile.d/x.sh",
		"etc/shadow",
		"home/root/.config/remarkable/../../../../etc/shadow",
		"remarkable/documents/../../../../../../etc/shadow",
		"remarkable/documents/abc/1.rm",
		"remarkable/configs/xochitl.conf",
		"home/root/.config/remarkable-other/x",
		"home/root/.ssh/authorized_keys",
	}
	for _, name := range rejected {
		if _, err := m.restoreTarget(&tar.Header{Name: name, Typeflag: tar.TypeReg}); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
}

func TestRestoreTargetRejectsLinks(t *testing.T) {
	m := &Manager{}
	for _, typeflag := range []byte{tar.TypeSymlink, tar.TypeLink, tar.TypeChar} {
		header := &tar.Header{Name: "home/root/.config/remarkable/x", Typeflag: typeflag, Linkname: "/etc/shadow"}
		if _, err := m.restoreTarget(header); err == nil {
			t.Errorf("accepted type %q", typeflag)
		}
	}
}
