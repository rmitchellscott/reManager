package backup

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"golang.org/x/crypto/ssh"
)

type Manager struct {
	SSHClient  *ssh.Client
	CancelCh   <-chan struct{}
	ProgressFn func(Progress)
	DeviceName string
	DeviceID   string
	MachineID  string
}

type BackupMetadata struct {
	DeviceName string `json:"deviceName"`
	DeviceID   string `json:"deviceId"`
	MachineID  string `json:"machineId,omitempty"`
	CreatedAt  string `json:"createdAt"`
	Version    string `json:"version"`
}

type Phase string

const (
	PhaseScanning    Phase = "scanning"
	PhaseDownloading Phase = "downloading"
	PhaseUploading   Phase = "uploading"
	PhaseComplete    Phase = "complete"
	PhaseStoppingUI  Phase = "stopping-ui"
	PhaseStartingUI  Phase = "starting-ui"
)

type Progress struct {
	Phase      Phase   `json:"phase"`
	FilePath   string  `json:"filePath"`
	FilesDone  int     `json:"filesDone"`
	FilesTotal int     `json:"filesTotal"`
	BytesDone  int64   `json:"bytesDone"`
	BytesTotal int64   `json:"bytesTotal"`
	Percentage float64 `json:"percentage"`
	Message    string  `json:"message"`
}

func (m *Manager) isCanceled() bool {
	select {
	case <-m.CancelCh:
		return true
	default:
		return false
	}
}

func (m *Manager) emitProgress(p Progress) {
	if m.ProgressFn != nil {
		m.ProgressFn(p)
	}
}

func (m *Manager) countRemoteFiles(sources []string) (int, error) {
	session, err := m.SSHClient.NewSession()
	if err != nil {
		return 0, fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	cmd := fmt.Sprintf("find %s -type f 2>/dev/null | wc -l", strings.Join(sources, " "))
	output, err := session.Output(cmd)
	if err != nil {
		return 0, fmt.Errorf("failed to count files: %w", err)
	}

	var count int
	_, err = fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &count)
	if err != nil {
		return 0, fmt.Errorf("failed to parse file count: %w", err)
	}

	return count, nil
}

// A backup that does not finish removes its archive, so a canceled or failed run never leaves a well-formed file that holds only part of the tablet.
func (m *Manager) CreateBackup(destPath string, sources []string) (err error) {
	m.emitProgress(Progress{
		Phase:      PhaseScanning,
		Message:    "Scanning files on your reMarkable…",
		Percentage: 0,
	})

	totalFiles, err := m.countRemoteFiles(sources)
	if err != nil {
		return fmt.Errorf("failed to scan files: %w", err)
	}

	if m.isCanceled() {
		return fmt.Errorf("operation canceled")
	}

	m.emitProgress(Progress{
		Phase:      PhaseScanning,
		Message:    fmt.Sprintf("Found %d files", totalFiles),
		FilesTotal: totalFiles,
		Percentage: 0,
	})

	cmd := fmt.Sprintf("tar -czf - %s", strings.Join(sources, " "))

	session, err := m.SSHClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout: %w", err)
	}

	if err := session.Start(cmd); err != nil {
		return fmt.Errorf("failed to start tar command: %w", err)
	}

	m.emitProgress(Progress{
		Phase:      PhaseDownloading,
		Message:    "Downloading from your reMarkable…",
		FilesTotal: totalFiles,
		Percentage: 0,
	})

	archiveFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create archive: %w", err)
	}
	defer func() {
		if err != nil {
			archiveFile.Close()
			os.Remove(destPath)
		}
	}()

	zstdWriter, err := zstd.NewWriter(archiveFile, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return fmt.Errorf("failed to create zstd writer: %w", err)
	}
	defer zstdWriter.Close()

	tarWriter := tar.NewWriter(zstdWriter)
	defer tarWriter.Close()

	metadata := BackupMetadata{
		DeviceName: m.DeviceName,
		DeviceID:   m.DeviceID,
		MachineID:  m.MachineID,
		CreatedAt:  time.Now().Format(time.RFC3339),
		Version:    "1",
	}
	metadataJSON, _ := json.Marshal(metadata)
	metadataHeader := &tar.Header{
		Name:    "remarkable/BACKUP_INFO.json",
		Size:    int64(len(metadataJSON)),
		Mode:    0644,
		ModTime: time.Now(),
	}
	if err := tarWriter.WriteHeader(metadataHeader); err != nil {
		return fmt.Errorf("failed to write metadata header: %w", err)
	}
	if _, err := tarWriter.Write(metadataJSON); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	gzipReader, err := gzip.NewReader(stdout)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	// Stream: device tar (gzip compressed) → decompress → re-tar → zstd compress → local file
	var bytesDone int64
	var filesDone int

	for {
		if m.isCanceled() {
			session.Signal(ssh.SIGTERM)
			return fmt.Errorf("operation canceled")
		}

		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar entry: %w", err)
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header: %w", err)
		}

		if !header.FileInfo().IsDir() {
			written, err := io.Copy(tarWriter, tarReader)
			if err != nil {
				return fmt.Errorf("failed to copy file data: %w", err)
			}

			bytesDone += written
			filesDone++

			progress := (float64(filesDone) / float64(totalFiles)) * 100.0
			if progress > 100 {
				progress = 100
			}
			m.emitProgress(Progress{
				Phase:      PhaseDownloading,
				FilesDone:  filesDone,
				FilesTotal: totalFiles,
				BytesDone:  bytesDone,
				Message:    fmt.Sprintf("Downloaded %d/%d files", filesDone, totalFiles),
				Percentage: progress,
			})
		}
	}

	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("failed to finish archive: %w", err)
	}
	if err := zstdWriter.Close(); err != nil {
		return fmt.Errorf("failed to compress archive: %w", err)
	}
	if err := archiveFile.Close(); err != nil {
		return fmt.Errorf("failed to write archive: %w", err)
	}

	if err := session.Wait(); err != nil {
		return fmt.Errorf("tar command failed: %w", err)
	}

	m.emitProgress(Progress{
		Phase:      PhaseComplete,
		Message:    "Backup complete",
		FilesDone:  filesDone,
		FilesTotal: totalFiles,
		BytesDone:  bytesDone,
		Percentage: 100,
	})

	return nil
}

func (m *Manager) RestoreBackup(archivePath string) error {
	m.emitProgress(Progress{
		Phase:      PhaseScanning,
		Message:    "Scanning backup archive…",
		Percentage: 0,
	})

	totalFiles, err := m.countArchiveFiles(archivePath)
	if err != nil {
		return fmt.Errorf("failed to scan archive: %w", err)
	}

	if m.isCanceled() {
		return fmt.Errorf("operation canceled")
	}

	m.emitProgress(Progress{
		Phase:      PhaseScanning,
		Message:    fmt.Sprintf("Found %d files", totalFiles),
		FilesTotal: totalFiles,
		Percentage: 0,
	})

	archiveFile, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer archiveFile.Close()

	zstdReader, err := zstd.NewReader(archiveFile)
	if err != nil {
		return fmt.Errorf("failed to create zstd reader (corrupted or invalid format): %w", err)
	}
	defer zstdReader.Close()

	tarReader := tar.NewReader(zstdReader)

	session, err := m.SSHClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin: %w", err)
	}

	m.emitProgress(Progress{
		Phase:      PhaseUploading,
		Message:    "Uploading to your reMarkable…",
		FilesTotal: totalFiles,
		Percentage: 0,
	})

	cmd := "tar -xzf - -C /"
	if err := session.Start(cmd); err != nil {
		return fmt.Errorf("failed to start tar command: %w", err)
	}

	gzipWriter, err := gzip.NewWriterLevel(stdin, 1)
	if err != nil {
		return fmt.Errorf("failed to create gzip writer: %w", err)
	}

	tarWriter := tar.NewWriter(gzipWriter)

	var bytesDone int64
	var filesDone int

	for {
		if m.isCanceled() {
			session.Signal(ssh.SIGTERM)
			return fmt.Errorf("operation canceled")
		}

		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar entry: %w", err)
		}

		if header.Name == "remarkable/BACKUP_INFO.json" {
			continue
		}

		target, err := m.restoreTarget(header)
		if err != nil {
			return err
		}
		header.Name = target

		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header: %w", err)
		}

		if !header.FileInfo().IsDir() {
			written, err := io.Copy(tarWriter, tarReader)
			if err != nil {
				return fmt.Errorf("failed to copy file data: %w", err)
			}

			bytesDone += written
			filesDone++

			progress := (float64(filesDone) / float64(totalFiles)) * 100.0
			if progress > 100 {
				progress = 100
			}
			m.emitProgress(Progress{
				Phase:      PhaseUploading,
				FilesDone:  filesDone,
				FilesTotal: totalFiles,
				BytesDone:  bytesDone,
				Message:    fmt.Sprintf("Uploaded %d/%d files", filesDone, totalFiles),
				Percentage: progress,
			})
		}
	}

	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("failed to close tar writer: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}
	stdin.Close()

	if err := session.Wait(); err != nil {
		return fmt.Errorf("tar extract failed: %w", err)
	}

	m.emitProgress(Progress{
		Phase:      PhaseComplete,
		Message:    "Restore complete.",
		FilesDone:  filesDone,
		FilesTotal: totalFiles,
		BytesDone:  bytesDone,
		Percentage: 100,
	})

	return nil
}

func (m *Manager) countArchiveFiles(archivePath string) (int, error) {
	archiveFile, err := os.Open(archivePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open archive: %w", err)
	}
	defer archiveFile.Close()

	zstdReader, err := zstd.NewReader(archiveFile)
	if err != nil {
		return 0, fmt.Errorf("failed to create zstd reader: %w", err)
	}
	defer zstdReader.Close()

	tarReader := tar.NewReader(zstdReader)

	var count int
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("failed to read tar entry: %w", err)
		}
		if header.Name == "remarkable/BACKUP_INFO.json" {
			continue
		}
		if _, err := m.restoreTarget(header); err != nil {
			return 0, err
		}
		if !header.FileInfo().IsDir() {
			count++
		}
	}

	return count, nil
}

var restorableRoots = []string{
	"/home/root/.local/share/remarkable/xochitl",
	"/home/root/.config/remarkable",
}

func (m *Manager) restoreTarget(header *tar.Header) (string, error) {
	mode := header.FileInfo().Mode()
	if header.Typeflag == tar.TypeLink || (!mode.IsRegular() && !mode.IsDir()) {
		return "", fmt.Errorf("backup entry %q is not a file or folder", header.Name)
	}
	target := path.Clean("/" + header.Name)
	for _, root := range restorableRoots {
		if target == root || strings.HasPrefix(target, root+"/") {
			return strings.TrimPrefix(target, "/"), nil
		}
	}
	return "", fmt.Errorf("backup entry %q is outside the reMarkable's documents and settings", header.Name)
}

func ReadBackupMetadata(archivePath string) (*BackupMetadata, error) {
	archiveFile, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open archive: %w", err)
	}
	defer archiveFile.Close()

	zstdReader, err := zstd.NewReader(archiveFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create zstd reader: %w", err)
	}
	defer zstdReader.Close()

	tarReader := tar.NewReader(zstdReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar entry: %w", err)
		}

		if header.Name == "remarkable/BACKUP_INFO.json" {
			data, err := io.ReadAll(tarReader)
			if err != nil {
				return nil, fmt.Errorf("failed to read metadata: %w", err)
			}
			var metadata BackupMetadata
			if err := json.Unmarshal(data, &metadata); err != nil {
				return nil, fmt.Errorf("failed to parse metadata: %w", err)
			}
			return &metadata, nil
		}
	}

	return nil, nil
}
