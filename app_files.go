package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"github.com/skratchdot/open-golang/open"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"

	"reManager/internal/backup"
	apperrors "reManager/internal/errors"
	"reManager/internal/logger"
	"reManager/internal/platform"

	"github.com/rymdport/portal/openuri"
)

type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"isDir"`
	ModTime int64  `json:"modTime"`
	Mode    string `json:"mode"`
}

type cmdLogWriter struct {
	cmdLog *logger.CommandLog
}

func (w *cmdLogWriter) Write(p []byte) (n int, err error) {
	w.cmdLog.Write(string(p))
	return len(p), nil
}

func (a *App) ListDirectory(dirPath string) ([]FileInfo, error) {
	if dirPath == "" {
		dirPath = "/home/root"
	}

	sftpClient, err := a.getSFTPClient()
	if err != nil {
		return nil, err
	}
	defer sftpClient.Close()

	entries, err := sftpClient.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var files []FileInfo
	for _, entry := range entries {
		fullPath := path.Join(dirPath, entry.Name())
		if dirPath == "/" {
			fullPath = "/" + entry.Name()
		}
		files = append(files, FileInfo{
			Name:    entry.Name(),
			Path:    fullPath,
			Size:    entry.Size(),
			IsDir:   entry.IsDir(),
			ModTime: entry.ModTime().Unix(),
			Mode:    entry.Mode().String(),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	return files, nil
}

func (a *App) DownloadFile(remotePath string) {
	go func() {
		client := a.getClient()

		if client == nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]interface{}{
				"message": "Not connected.",
				"code":    apperrors.ErrHostDown,
			})
			return
		}

		filename := path.Base(remotePath)
		var lastDir string
		if a.settingsStore != nil {
			if settings, err := a.settingsStore.Load(); err == nil {
				lastDir = settings.LastDownloadDir
			}
		}
		localPath, err := saveFileDialog(a.ctx, "Save File", filename, lastDir)
		if err != nil || localPath == "" {
			return
		}
		if a.settingsStore != nil {
			if settings, err := a.settingsStore.Load(); err == nil {
				settings.LastDownloadDir = filepath.Dir(localPath)
				_ = a.settingsStore.Save(settings)
			}
		}

		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			ue := apperrors.Classify(err)
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]interface{}{
				"message": ue.Message,
				"code":    ue.Code,
			})
			return
		}
		defer sftpClient.Close()

		remoteFile, err := sftpClient.Open(remotePath)
		if err != nil {
			ue := apperrors.Classify(err)
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]interface{}{
				"message": ue.Message,
				"code":    ue.Code,
			})
			return
		}
		defer remoteFile.Close()

		stat, err := remoteFile.Stat()
		if err != nil {
			ue := apperrors.Classify(err)
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]interface{}{
				"message": ue.Message,
				"code":    ue.Code,
			})
			return
		}
		totalBytes := stat.Size()

		localFile, err := os.Create(localPath)
		if err != nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
				"message": fmt.Sprintf("Failed to create local file: %v", err),
			})
			return
		}
		defer localFile.Close()

		buffer := make([]byte, 32*1024)
		var transferred int64

		for {
			n, err := remoteFile.Read(buffer)
			if n > 0 {
				_, writeErr := localFile.Write(buffer[:n])
				if writeErr != nil {
					runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
						"message": fmt.Sprintf("Failed to write to local file: %v", writeErr),
					})
					return
				}
				transferred += int64(n)

				var percentage float64
				if totalBytes > 0 {
					percentage = float64(transferred) / float64(totalBytes) * 100
				}
				runtime.EventsEmit(a.ctx, "filebrowser:progress", TransferProgress{
					Filename:   filename,
					BytesSent:  transferred,
					TotalBytes: totalBytes,
					Percentage: percentage,
					Status:     "downloading",
				})
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
					"message": fmt.Sprintf("Failed to read remote file: %v", err),
				})
				return
			}
		}

		runtime.EventsEmit(a.ctx, "filebrowser:progress", TransferProgress{
			Filename:   filename,
			BytesSent:  totalBytes,
			TotalBytes: totalBytes,
			Percentage: 100,
			Status:     "downloading",
		})

		runtime.EventsEmit(a.ctx, "filebrowser:download-complete", map[string]string{
			"path": remotePath,
		})
	}()
}

func (a *App) DownloadFolder(remotePath string) {
	go func() {
		client := a.getClient()

		if client == nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]interface{}{
				"message": "Not connected.",
				"code":    apperrors.ErrHostDown,
			})
			return
		}

		folderName := path.Base(remotePath)
		var lastDir string
		if a.settingsStore != nil {
			if settings, err := a.settingsStore.Load(); err == nil {
				lastDir = settings.LastDownloadDir
			}
		}
		localDir, err := openDirectoryDialog(a.ctx, "Save Folder To", lastDir)
		if err != nil || localDir == "" {
			return
		}
		if a.settingsStore != nil {
			if settings, err := a.settingsStore.Load(); err == nil {
				settings.LastDownloadDir = localDir
				_ = a.settingsStore.Save(settings)
			}
		}
		localBasePath := filepath.Join(localDir, folderName)

		a.folderTransferMu.Lock()
		a.folderTransferCancelCh = make(chan struct{})
		cancelCh := a.folderTransferCancelCh
		a.folderTransferMu.Unlock()

		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			ue := apperrors.Classify(err)
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]interface{}{
				"message": ue.Message,
				"code":    ue.Code,
			})
			return
		}
		defer sftpClient.Close()

		filesTotal, bytesTotal, err := a.countRemoteFolder(sftpClient, remotePath)
		if err != nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
				"message": fmt.Sprintf("Failed to scan folder: %v", err),
			})
			return
		}

		var filesDone int
		var bytesDone int64
		var failedFiles []string

		err = a.downloadFolderRecursive(sftpClient, remotePath, localBasePath, &filesDone, &bytesDone, filesTotal, bytesTotal, &failedFiles, cancelCh)

		select {
		case <-cancelCh:
			runtime.EventsEmit(a.ctx, "filebrowser:folder-download-complete", map[string]interface{}{
				"cancelled": true,
			})
			return
		default:
		}

		if err != nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
				"message": fmt.Sprintf("Download failed: %v", err),
			})
			return
		}

		runtime.EventsEmit(a.ctx, "filebrowser:folder-download-complete", map[string]interface{}{
			"path":        remotePath,
			"failedFiles": failedFiles,
		})
	}()
}

func (a *App) countRemoteFolder(sftpClient *sftp.Client, remotePath string) (int, int64, error) {
	var filesTotal int
	var bytesTotal int64

	walker := sftpClient.Walk(remotePath)
	for walker.Step() {
		if err := walker.Err(); err != nil {
			continue
		}
		info := walker.Stat()
		if info.Mode()&os.ModeSymlink != 0 {
			filesTotal++
		} else if !info.IsDir() {
			filesTotal++
			bytesTotal += info.Size()
		}
	}

	return filesTotal, bytesTotal, nil
}

func (a *App) downloadFolderRecursive(sftpClient *sftp.Client, remotePath, localPath string, filesDone *int, bytesDone *int64, filesTotal int, bytesTotal int64, failedFiles *[]string, cancelCh chan struct{}) error {
	select {
	case <-cancelCh:
		return fmt.Errorf("cancelled")
	default:
	}

	info, err := sftpClient.Lstat(remotePath)
	if err != nil {
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		target, err := sftpClient.ReadLink(remotePath)
		if err != nil {
			*failedFiles = append(*failedFiles, remotePath)
			return nil
		}
		if err := os.Symlink(target, localPath); err != nil {
			*failedFiles = append(*failedFiles, remotePath)
		}
		*filesDone++
		a.emitFolderProgress(path.Base(remotePath), *filesDone, filesTotal, *bytesDone, bytesTotal, "downloading", true)
		return nil
	}

	if info.IsDir() {
		if err := os.MkdirAll(localPath, 0755); err != nil {
			return err
		}

		entries, err := sftpClient.ReadDir(remotePath)
		if err != nil {
			return err
		}

		for _, entry := range entries {
			remoteEntryPath := path.Join(remotePath, entry.Name())
			localEntryPath := filepath.Join(localPath, entry.Name())

			if err := a.downloadFolderRecursive(sftpClient, remoteEntryPath, localEntryPath, filesDone, bytesDone, filesTotal, bytesTotal, failedFiles, cancelCh); err != nil {
				return err
			}
		}
		return nil
	}

	remoteFile, err := sftpClient.Open(remotePath)
	if err != nil {
		*failedFiles = append(*failedFiles, remotePath)
		return nil
	}
	defer remoteFile.Close()

	localFile, err := os.Create(localPath)
	if err != nil {
		*failedFiles = append(*failedFiles, remotePath)
		return nil
	}
	defer localFile.Close()

	buffer := make([]byte, 32*1024)
	for {
		select {
		case <-cancelCh:
			return fmt.Errorf("cancelled")
		default:
		}

		n, err := remoteFile.Read(buffer)
		if n > 0 {
			if _, writeErr := localFile.Write(buffer[:n]); writeErr != nil {
				*failedFiles = append(*failedFiles, remotePath)
				return nil
			}
			*bytesDone += int64(n)
			a.emitFolderProgress(path.Base(remotePath), *filesDone, filesTotal, *bytesDone, bytesTotal, "downloading", true)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			*failedFiles = append(*failedFiles, remotePath)
			return nil
		}
	}

	*filesDone++
	a.emitFolderProgress(path.Base(remotePath), *filesDone, filesTotal, *bytesDone, bytesTotal, "downloading", true)
	return nil
}

func (a *App) emitFolderProgress(currentFile string, filesDone, filesTotal int, bytesDone, bytesTotal int64, status string, containsFolder bool) {
	var percentage float64
	if bytesTotal > 0 {
		percentage = float64(bytesDone) / float64(bytesTotal) * 100
	}
	runtime.EventsEmit(a.ctx, "filebrowser:folder-progress", FolderTransferProgress{
		CurrentFile:    currentFile,
		FilesDone:      filesDone,
		FilesTotal:     filesTotal,
		BytesDone:      bytesDone,
		BytesTotal:     bytesTotal,
		Percentage:     percentage,
		Status:         status,
		ContainsFolder: containsFolder,
	})
}

func (a *App) SelectFilesForUpload() []string {
	var lastDir string
	if a.settingsStore != nil {
		if settings, err := a.settingsStore.Load(); err == nil {
			lastDir = settings.LastUploadDir
		}
	}

	files, err := openMultipleFilesDialog(a.ctx, "Select files to upload", lastDir)
	if err != nil || len(files) == 0 {
		return []string{}
	}

	if a.settingsStore != nil {
		if settings, err := a.settingsStore.Load(); err == nil {
			settings.LastUploadDir = filepath.Dir(files[0])
			_ = a.settingsStore.Save(settings)
		}
	}

	return files
}

func (a *App) UploadFolder(remotePath string) {
	go func() {
		client := a.getClient()

		if client == nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
				"message": "Not connected",
			})
			return
		}

		localDir, err := openDirectoryDialog(a.ctx, "Select Folder to Upload", "")
		if err != nil || localDir == "" {
			return
		}

		a.folderTransferMu.Lock()
		a.folderTransferCancelCh = make(chan struct{})
		cancelCh := a.folderTransferCancelCh
		a.folderTransferMu.Unlock()

		folderName := filepath.Base(localDir)
		destPath := remotePath
		if strings.HasSuffix(remotePath, "/") || remotePath == "" {
			destPath = path.Join(remotePath, folderName)
		}

		needsWritable := isSystemPath(destPath)
		if needsWritable {
			if err := a.makeFilesystemWritable(client); err != nil {
				runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
					"message": fmt.Sprintf("Failed to prepare filesystem: %v", err),
				})
				return
			}
			defer a.restoreFilesystemDeferred(client)
		}

		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
				"message": fmt.Sprintf("Failed to create SFTP client: %v", err),
			})
			return
		}
		defer sftpClient.Close()

		filesTotal, bytesTotal, err := a.countLocalFolder(localDir)
		if err != nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
				"message": fmt.Sprintf("Failed to scan folder: %v", err),
			})
			return
		}

		var filesDone int
		var bytesDone int64
		var failedFiles []string

		err = a.uploadFolderRecursive(sftpClient, localDir, destPath, &filesDone, &bytesDone, filesTotal, bytesTotal, &failedFiles, cancelCh)

		select {
		case <-cancelCh:
			runtime.EventsEmit(a.ctx, "filebrowser:folder-upload-complete", map[string]interface{}{
				"cancelled": true,
			})
			return
		default:
		}

		if err != nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
				"message": fmt.Sprintf("Upload failed: %v", err),
			})
			return
		}

		runtime.EventsEmit(a.ctx, "filebrowser:folder-upload-complete", map[string]interface{}{
			"path":        destPath,
			"failedFiles": failedFiles,
		})
	}()
}

func (a *App) countLocalFolder(localPath string) (int, int64, error) {
	var filesTotal int
	var bytesTotal int64

	err := filepath.WalkDir(localPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			filesTotal++
		} else if !d.IsDir() {
			filesTotal++
			bytesTotal += info.Size()
		}
		return nil
	})

	return filesTotal, bytesTotal, err
}

func (a *App) uploadFolderRecursive(sftpClient *sftp.Client, localPath, remotePath string, filesDone *int, bytesDone *int64, filesTotal int, bytesTotal int64, failedFiles *[]string, cancelCh chan struct{}) error {
	select {
	case <-cancelCh:
		return fmt.Errorf("cancelled")
	default:
	}

	info, err := os.Lstat(localPath)
	if err != nil {
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(localPath)
		if err != nil {
			*failedFiles = append(*failedFiles, localPath)
			return nil
		}
		if err := sftpClient.Symlink(target, remotePath); err != nil {
			*failedFiles = append(*failedFiles, localPath)
		}
		*filesDone++
		a.emitFolderProgress(filepath.Base(localPath), *filesDone, filesTotal, *bytesDone, bytesTotal, "uploading", true)
		return nil
	}

	if info.IsDir() {
		if err := sftpClient.MkdirAll(remotePath); err != nil {
			return err
		}

		entries, err := os.ReadDir(localPath)
		if err != nil {
			return err
		}

		for _, entry := range entries {
			localEntryPath := filepath.Join(localPath, entry.Name())
			remoteEntryPath := path.Join(remotePath, entry.Name())

			if err := a.uploadFolderRecursive(sftpClient, localEntryPath, remoteEntryPath, filesDone, bytesDone, filesTotal, bytesTotal, failedFiles, cancelCh); err != nil {
				return err
			}
		}
		return nil
	}

	localFile, err := os.Open(localPath)
	if err != nil {
		*failedFiles = append(*failedFiles, localPath)
		return nil
	}
	defer localFile.Close()

	remoteFile, err := sftpClient.Create(remotePath)
	if err != nil {
		*failedFiles = append(*failedFiles, localPath)
		return nil
	}
	defer remoteFile.Close()

	buffer := make([]byte, 32*1024)
	for {
		select {
		case <-cancelCh:
			return fmt.Errorf("cancelled")
		default:
		}

		n, err := localFile.Read(buffer)
		if n > 0 {
			if _, writeErr := remoteFile.Write(buffer[:n]); writeErr != nil {
				*failedFiles = append(*failedFiles, localPath)
				return nil
			}
			*bytesDone += int64(n)
			a.emitFolderProgress(filepath.Base(localPath), *filesDone, filesTotal, *bytesDone, bytesTotal, "uploading", true)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			*failedFiles = append(*failedFiles, localPath)
			return nil
		}
	}

	*filesDone++
	a.emitFolderProgress(filepath.Base(localPath), *filesDone, filesTotal, *bytesDone, bytesTotal, "uploading", true)
	return nil
}

func (a *App) CancelFolderTransfer() {
	a.folderTransferMu.Lock()
	defer a.folderTransferMu.Unlock()
	if a.folderTransferCancelCh != nil {
		close(a.folderTransferCancelCh)
		a.folderTransferCancelCh = nil
	}
}

func (a *App) UploadFilesFromPaths(localPaths []string, remotePath string) {
	go func() {
		client := a.getClient()

		if client == nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
				"message": "Not connected",
			})
			return
		}

		if len(localPaths) == 0 {
			return
		}

		var hasFolder bool
		for _, localPath := range localPaths {
			info, err := os.Stat(localPath)
			if err == nil && info.IsDir() {
				hasFolder = true
				break
			}
		}

		if hasFolder || len(localPaths) > 1 {
			a.uploadMixedPaths(client, localPaths, remotePath, hasFolder)
			return
		}

		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
				"message": fmt.Sprintf("Failed to create SFTP client: %v", err),
			})
			return
		}
		defer sftpClient.Close()

		for _, localPath := range localPaths {
			if err := a.uploadSingleFile(client, sftpClient, localPath, remotePath); err != nil {
				runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
					"message": err.Error(),
				})
			}
		}

		runtime.EventsEmit(a.ctx, "filebrowser:upload-complete", map[string]string{
			"path": remotePath,
		})
	}()
}

func (a *App) uploadMixedPaths(client *ssh.Client, localPaths []string, remotePath string, containsFolder bool) {
	a.folderTransferMu.Lock()
	a.folderTransferCancelCh = make(chan struct{})
	cancelCh := a.folderTransferCancelCh
	a.folderTransferMu.Unlock()

	needsWritable := isSystemPath(remotePath)
	if needsWritable {
		if err := a.makeFilesystemWritable(client); err != nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
				"message": fmt.Sprintf("Failed to prepare filesystem: %v", err),
			})
			return
		}
		defer a.restoreFilesystemDeferred(client)
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
			"message": fmt.Sprintf("Failed to create SFTP client: %v", err),
		})
		return
	}
	defer sftpClient.Close()

	var filesTotal int
	var bytesTotal int64
	for _, localPath := range localPaths {
		info, err := os.Stat(localPath)
		if err != nil {
			continue
		}
		if info.IsDir() {
			f, b, _ := a.countLocalFolder(localPath)
			filesTotal += f
			bytesTotal += b
		} else {
			filesTotal++
			bytesTotal += info.Size()
		}
	}

	var filesDone int
	var bytesDone int64
	var failedFiles []string

	for _, localPath := range localPaths {
		select {
		case <-cancelCh:
			runtime.EventsEmit(a.ctx, "filebrowser:folder-upload-complete", map[string]interface{}{
				"cancelled": true,
			})
			return
		default:
		}

		info, err := os.Stat(localPath)
		if err != nil {
			failedFiles = append(failedFiles, localPath)
			continue
		}

		if info.IsDir() {
			folderName := filepath.Base(localPath)
			destPath := path.Join(remotePath, folderName)
			if err := a.uploadFolderRecursive(sftpClient, localPath, destPath, &filesDone, &bytesDone, filesTotal, bytesTotal, &failedFiles, cancelCh); err != nil {
				continue
			}
		} else {
			destPath := path.Join(remotePath, info.Name())
			localFile, err := os.Open(localPath)
			if err != nil {
				failedFiles = append(failedFiles, localPath)
				continue
			}

			remoteFile, err := sftpClient.Create(destPath)
			if err != nil {
				localFile.Close()
				failedFiles = append(failedFiles, localPath)
				continue
			}

			buffer := make([]byte, 32*1024)
			for {
				n, err := localFile.Read(buffer)
				if n > 0 {
					if _, writeErr := remoteFile.Write(buffer[:n]); writeErr != nil {
						failedFiles = append(failedFiles, localPath)
						break
					}
					bytesDone += int64(n)
					a.emitFolderProgress(info.Name(), filesDone, filesTotal, bytesDone, bytesTotal, "uploading", containsFolder)
				}
				if err == io.EOF {
					filesDone++
					a.emitFolderProgress(info.Name(), filesDone, filesTotal, bytesDone, bytesTotal, "uploading", containsFolder)
					break
				}
				if err != nil {
					failedFiles = append(failedFiles, localPath)
					break
				}
			}
			localFile.Close()
			remoteFile.Close()
		}
	}

	runtime.EventsEmit(a.ctx, "filebrowser:folder-upload-complete", map[string]interface{}{
		"path":        remotePath,
		"failedFiles": failedFiles,
	})
}

func (a *App) uploadSingleFile(client *ssh.Client, sftpClient *sftp.Client, localPath string, remotePath string) error {
	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file %s: %v", localPath, err)
	}
	defer localFile.Close()

	stat, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat local file: %v", err)
	}

	if stat.IsDir() {
		return nil
	}

	totalBytes := stat.Size()
	filename := stat.Name()

	destPath := remotePath
	if strings.HasSuffix(remotePath, "/") || remotePath == "" {
		destPath = path.Join(remotePath, filename)
	}

	if isSystemPath(destPath) {
		if err := a.makeFilesystemWritable(client); err != nil {
			return fmt.Errorf("failed to prepare filesystem: %v", err)
		}
		defer a.restoreFilesystemDeferred(client)
	}

	remoteFile, err := sftpClient.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create remote file: %v", err)
	}
	defer remoteFile.Close()

	buffer := make([]byte, 32*1024)
	var transferred int64

	for {
		n, err := localFile.Read(buffer)
		if n > 0 {
			_, writeErr := remoteFile.Write(buffer[:n])
			if writeErr != nil {
				return fmt.Errorf("failed to write to remote file: %v", writeErr)
			}
			transferred += int64(n)

			var percentage float64
			if totalBytes > 0 {
				percentage = float64(transferred) / float64(totalBytes) * 100
			}
			runtime.EventsEmit(a.ctx, "filebrowser:progress", TransferProgress{
				Filename:   filename,
				BytesSent:  transferred,
				TotalBytes: totalBytes,
				Percentage: percentage,
				Status:     "uploading",
			})
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read local file: %v", err)
		}
	}

	runtime.EventsEmit(a.ctx, "filebrowser:progress", TransferProgress{
		Filename:   filename,
		BytesSent:  totalBytes,
		TotalBytes: totalBytes,
		Percentage: 100,
		Status:     "uploading",
	})

	return nil
}

func (a *App) DeletePath(path string) error {
	client := a.getClient()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	if isSystemPath(path) {
		if err := a.makeFilesystemWritable(client); err != nil {
			return err
		}
		defer a.restoreFilesystemDeferred(client)
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	stat, err := sftpClient.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat path: %w", err)
	}

	if stat.IsDir() {
		entries, err := sftpClient.ReadDir(path)
		if err != nil {
			return fmt.Errorf("failed to read directory: %w", err)
		}
		if len(entries) > 0 {
			session, err := client.NewSession()
			if err != nil {
				return fmt.Errorf("failed to create SSH session: %w", err)
			}
			defer session.Close()
			_, err = session.CombinedOutput(fmt.Sprintf("rm -rf %q", path))
			if err != nil {
				return fmt.Errorf("failed to delete directory: %w", err)
			}
		} else {
			err = sftpClient.RemoveDirectory(path)
			if err != nil {
				return fmt.Errorf("failed to remove directory: %w", err)
			}
		}
	} else {
		err = sftpClient.Remove(path)
		if err != nil {
			return fmt.Errorf("failed to delete file: %w", err)
		}
	}

	return nil
}

func (a *App) RenamePath(oldPath, newPath string) error {
	client := a.getClient()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	if isSystemPath(oldPath) || isSystemPath(newPath) {
		if err := a.makeFilesystemWritable(client); err != nil {
			return err
		}
		defer a.restoreFilesystemDeferred(client)
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	err = sftpClient.Rename(oldPath, newPath)
	if err != nil {
		return fmt.Errorf("failed to rename: %w", err)
	}

	return nil
}

func (a *App) CreateDirectory(path string) error {
	client := a.getClient()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	if isSystemPath(path) {
		if err := a.makeFilesystemWritable(client); err != nil {
			return err
		}
		defer a.restoreFilesystemDeferred(client)
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	err = sftpClient.Mkdir(path)
	if err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	return nil
}

func (a *App) SelectBackupFile() string {
	a.mu.Lock()
	deviceID := a.connectedDeviceID
	a.mu.Unlock()

	rawDeviceName := ""
	if savedDevice, err := a.deviceStore.Get(deviceID); err == nil && savedDevice.Name != "" {
		rawDeviceName = savedDevice.Name
	}
	deviceName := "device"
	if rawDeviceName != "" {
		deviceName = sanitizeFilename(rawDeviceName)
	}

	timestamp := time.Now().Format("2006-01-02-150405")
	defaultName := fmt.Sprintf("remarkable-backup-%s-%s.tar.zst", deviceName, timestamp)
	destPath, err := saveFileDialog(a.ctx, "Save Backup", defaultName, "")
	if err != nil || destPath == "" {
		return ""
	}
	return destPath
}

func (a *App) CreateDeviceBackup(destPath string) {
	go func() {
		a.mu.Lock()
		client := a.client
		deviceID := a.connectedDeviceID
		a.mu.Unlock()

		if client == nil {
			runtime.EventsEmit(a.ctx, "backup:error", map[string]string{
				"message": "Not connected to device",
			})
			return
		}

		rawDeviceName := ""
		if savedDevice, err := a.deviceStore.Get(deviceID); err == nil && savedDevice.Name != "" {
			rawDeviceName = savedDevice.Name
		}

		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			runtime.EventsEmit(a.ctx, "backup:error", map[string]string{
				"message": fmt.Sprintf("Failed to create SFTP client: %v", err),
			})
			return
		}
		defer sftpClient.Close()

		a.backupMu.Lock()
		a.backupCancelCh = make(chan struct{})
		cancelCh := a.backupCancelCh
		a.backupMu.Unlock()

		manager := backup.Manager{
			Ctx:        a.ctx,
			SftpClient: sftpClient,
			SSHClient:  client,
			CancelCh:   cancelCh,
			ProgressFn: func(p backup.Progress) {
				runtime.EventsEmit(a.ctx, "backup:progress", p)
			},
			DeviceName: rawDeviceName,
			DeviceID:   deviceID,
		}

		sources := []string{
			"/home/root/.local/share/remarkable/xochitl",
			"/home/root/.config/remarkable",
		}

		err = manager.CreateBackup(destPath, sources)

		a.backupMu.Lock()
		a.backupCancelCh = nil
		a.backupMu.Unlock()

		if err != nil {
			runtime.EventsEmit(a.ctx, "backup:error", map[string]string{
				"message": err.Error(),
			})
		} else {
			runtime.EventsEmit(a.ctx, "backup:complete", map[string]string{
				"message": "Backup completed successfully",
				"path":    destPath,
			})
		}
	}()
}

func (a *App) SelectRestoreFile() string {
	archivePath, err := openFileDialog(a.ctx, "Select Backup to Restore", "")
	if err != nil || archivePath == "" {
		return ""
	}

	metadata, err := backup.ReadBackupMetadata(archivePath)
	if err == nil && metadata != nil && metadata.DeviceID != "" {
		a.mu.Lock()
		currentDeviceID := a.connectedDeviceID
		a.mu.Unlock()

		if currentDeviceID != "" && metadata.DeviceID != currentDeviceID {
			currentDeviceName := ""
			if savedDevice, err := a.deviceStore.Get(currentDeviceID); err == nil {
				currentDeviceName = savedDevice.Name
			}
			runtime.EventsEmit(a.ctx, "restore:device-mismatch", map[string]string{
				"backupDevice":  metadata.DeviceName,
				"currentDevice": currentDeviceName,
			})
		}
	}

	return archivePath
}

func (a *App) RestoreDeviceBackup(archivePath string) {
	go func() {
		client := a.getClient()

		if client == nil {
			runtime.EventsEmit(a.ctx, "restore:error", map[string]string{
				"message": "Not connected to device",
			})
			return
		}

		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			runtime.EventsEmit(a.ctx, "restore:error", map[string]string{
				"message": fmt.Sprintf("Failed to create SFTP client: %v", err),
			})
			return
		}
		defer sftpClient.Close()

		a.backupMu.Lock()
		a.backupCancelCh = make(chan struct{})
		cancelCh := a.backupCancelCh
		a.backupMu.Unlock()

		manager := backup.Manager{
			Ctx:        a.ctx,
			SftpClient: sftpClient,
			SSHClient:  client,
			CancelCh:   cancelCh,
			ProgressFn: func(p backup.Progress) {
				runtime.EventsEmit(a.ctx, "restore:progress", p)
			},
		}

		err = manager.RestoreBackup(archivePath)

		a.backupMu.Lock()
		a.backupCancelCh = nil
		a.backupMu.Unlock()

		if err != nil {
			runtime.EventsEmit(a.ctx, "restore:error", map[string]string{
				"message": err.Error(),
			})
		} else {
			runtime.EventsEmit(a.ctx, "restore:complete", map[string]string{
				"message": "Restore completed successfully! Please reboot your device for changes to take effect.",
			})
		}
	}()
}

func (a *App) CancelBackup() {
	a.backupMu.Lock()
	defer a.backupMu.Unlock()

	if a.backupCancelCh != nil {
		close(a.backupCancelCh)
		a.backupCancelCh = nil
	}
}

func (a *App) RevealInFileManager(path string) {
	dir := filepath.Dir(path)

	if platform.IsRunningInFlatpak() {
		file, err := os.Open(dir)
		if err != nil {
			open.Start(dir)
			return
		}
		defer file.Close()

		if err := openuri.OpenDirectory("", file.Fd(), nil); err != nil {
			open.Start(dir)
		}
		return
	}

	open.Start(dir)
}

