package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Manager struct {
	Ctx        context.Context
	SftpClient *sftp.Client
	SSHClient  *ssh.Client
	CancelCh   chan struct{}
	ProgressFn func(Progress)
	DeviceName string
	DeviceID   string
}

type BackupMetadata struct {
	DeviceName string `json:"deviceName"`
	DeviceID   string `json:"deviceId"`
	CreatedAt  string `json:"createdAt"`
	Version    string `json:"version"`
}

type Progress struct {
	Phase      string  `json:"phase"`
	FilePath   string  `json:"filePath"`
	FilesDone  int     `json:"filesDone"`
	FilesTotal int     `json:"filesTotal"`
	BytesDone  int64   `json:"bytesDone"`
	BytesTotal int64   `json:"bytesTotal"`
	Percentage float64 `json:"percentage"`
	Message    string  `json:"message"`
}

func (m *Manager) Cancel() {
	if m.CancelCh != nil {
		close(m.CancelCh)
	}
}

func (m *Manager) isCancelled() bool {
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

func (m *Manager) CreateBackup(destPath string, sources []string) error {
	m.emitProgress(Progress{
		Phase:      "scanning",
		Message:    "Scanning files on device...",
		Percentage: 0,
	})

	// First pass: count files on device
	totalFiles, err := m.countRemoteFiles(sources)
	if err != nil {
		return fmt.Errorf("failed to scan files: %w", err)
	}

	if m.isCancelled() {
		return fmt.Errorf("operation cancelled")
	}

	m.emitProgress(Progress{
		Phase:      "scanning",
		Message:    fmt.Sprintf("Found %d files", totalFiles),
		FilesTotal: totalFiles,
		Percentage: 0,
	})

	// Build tar command to run on device with gzip compression
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
		Phase:      "downloading",
		Message:    "Downloading from device...",
		FilesTotal: totalFiles,
		Percentage: 0,
	})

	// Create local zstd archive
	archiveFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create archive: %w", err)
	}
	defer archiveFile.Close()

	zstdWriter, err := zstd.NewWriter(archiveFile, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return fmt.Errorf("failed to create zstd writer: %w", err)
	}
	defer zstdWriter.Close()

	tarWriter := tar.NewWriter(zstdWriter)
	defer tarWriter.Close()

	// Write metadata as first entry in archive
	metadata := BackupMetadata{
		DeviceName: m.DeviceName,
		DeviceID:   m.DeviceID,
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

	// Read compressed tar from device
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
		if m.isCancelled() {
			session.Signal(ssh.SIGTERM)
			return fmt.Errorf("operation cancelled")
		}

		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar entry: %w", err)
		}

		// Remap paths to archive format
		// /home/root/.local/share/remarkable/xochitl/... → remarkable/documents/...
		// /home/root/.config/remarkable/... → remarkable/configs/...
		if strings.HasPrefix(header.Name, "/home/root/.local/share/remarkable/xochitl/") {
			header.Name = "remarkable/documents/" + strings.TrimPrefix(header.Name, "/home/root/.local/share/remarkable/xochitl/")
		} else if strings.HasPrefix(header.Name, "/home/root/.config/remarkable/") {
			header.Name = "remarkable/configs/" + strings.TrimPrefix(header.Name, "/home/root/.config/remarkable/")
		}

		// Write header to output tar
		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header: %w", err)
		}

		// Copy file data if not a directory
		if !header.FileInfo().IsDir() {
			written, err := io.Copy(tarWriter, tarReader)
			if err != nil {
				return fmt.Errorf("failed to copy file data: %w", err)
			}

			bytesDone += written
			filesDone++

			// Calculate progress: 5% for scan, 95% for download
			progress := (float64(filesDone) / float64(totalFiles)) * 100.0
			if progress > 100 {
				progress = 100
			}
			m.emitProgress(Progress{
				Phase:      "downloading",
				FilesDone:  filesDone,
				FilesTotal: totalFiles,
				BytesDone:  bytesDone,
				Message:    fmt.Sprintf("Downloaded %d/%d files", filesDone, totalFiles),
				Percentage: progress,
			})
		}
	}

	tarWriter.Close()
	zstdWriter.Close()
	archiveFile.Close()

	if err := session.Wait(); err != nil {
		return fmt.Errorf("tar command failed: %w", err)
	}

	m.emitProgress(Progress{
		Phase:      "complete",
		Message:    "Backup complete!",
		FilesDone:  filesDone,
		FilesTotal: totalFiles,
		BytesDone:  bytesDone,
		Percentage: 100,
	})

	return nil
}

func (m *Manager) RestoreBackup(archivePath string) error {
	m.emitProgress(Progress{
		Phase:      "scanning",
		Message:    "Scanning backup archive...",
		Percentage: 0,
	})

	// First pass: count files in archive
	totalFiles, err := m.countArchiveFiles(archivePath)
	if err != nil {
		return fmt.Errorf("failed to scan archive: %w", err)
	}

	if m.isCancelled() {
		return fmt.Errorf("operation cancelled")
	}

	m.emitProgress(Progress{
		Phase:      "scanning",
		Message:    fmt.Sprintf("Found %d files", totalFiles),
		FilesTotal: totalFiles,
		Percentage: 0,
	})

	// Second pass: actual restore
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
		Phase:      "uploading",
		Message:    "Uploading to device...",
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
		if m.isCancelled() {
			session.Signal(ssh.SIGTERM)
			return fmt.Errorf("operation cancelled")
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

		header.Name = m.mapArchivePathToDevicePath(header.Name)

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
				Phase:      "uploading",
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

	// Wait for remote command to finish
	if err := session.Wait(); err != nil {
		return fmt.Errorf("tar extract failed: %w", err)
	}

	m.emitProgress(Progress{
		Phase:      "complete",
		Message:    "Restore complete! Please reboot your device for changes to take effect.",
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
		if !header.FileInfo().IsDir() && header.Name != "remarkable/BACKUP_INFO.json" {
			count++
		}
	}

	return count, nil
}

func (m *Manager) mapArchivePathToDevicePath(archivePath string) string {
	// Archive has: "remarkable/documents/..." or "remarkable/configs/..."
	// Need to map to: "/home/root/.local/share/remarkable/xochitl/..." etc

	if strings.HasPrefix(archivePath, "remarkable/documents/") {
		return "/home/root/.local/share/remarkable/xochitl/" +
			strings.TrimPrefix(archivePath, "remarkable/documents/")
	}
	if strings.HasPrefix(archivePath, "remarkable/configs/") {
		return "/home/root/.config/remarkable/" +
			strings.TrimPrefix(archivePath, "remarkable/configs/")
	}
	return archivePath
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
