package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"

	"reManager/internal/debug"
	"reManager/internal/epubimport"
	"reManager/internal/httputil"
	"reManager/internal/pdfimport"
	"reManager/internal/platform"
	"reManager/internal/rmdocimport"
	"reManager/internal/storage"
)

const guideManifestURL = "https://raw.githubusercontent.com/rmitchellscott/remanager/main/docs/guide/guide-manifest.json"
const guideDownloadURLBase = "https://raw.githubusercontent.com/rmitchellscott/remanager/main/docs/guide/releases/"
const guideVisibleName = "reManager User Guide"

type ImportFileInfo struct {
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	PageCount   int    `json:"pageCount"`
	FileType    string `json:"fileType"`
	VisibleName string `json:"visibleName"`
}

type UserGuideStatus struct {
	Installed   bool   `json:"installed"`
	NeedsUpdate bool   `json:"needsUpdate"`
	Skipped     bool   `json:"skipped"`
	Unseen      bool   `json:"unseen"`
	DocID       string `json:"docId"`
}

func (s *FilesService) SelectImportFiles() []string {
	if paths, ok := s.screenshotImportPaths(); ok {
		return paths
	}
	if platform.IsRunningInFlatpak() {
		home, _ := os.UserHomeDir()
		files, err := portalOpenFiles("Select documents", portalOpenOptions{
			dir:            home,
			multiple:       true,
			filterName:     "Documents",
			filterPatterns: []string{"*.pdf", "*.epub", "*.rmdoc"},
		})
		if err != nil || len(files) == 0 {
			return []string{}
		}
		return files
	}
	files, err := s.wails.Dialog.OpenFile().
		CanChooseFiles(true).
		SetTitle("Select documents").
		SetDirectory(dialogHomeDir()).
		AddFilter("Documents (PDF, EPUB, rmdoc)", "*.pdf;*.epub;*.rmdoc").
		PromptForMultipleSelection()
	if err != nil {
		return []string{}
	}
	return files
}

func (s *FilesService) GetImportFileInfo(localPath string) (ImportFileInfo, error) {
	if info, ok := s.screenshotImportFileInfo(localPath); ok {
		return info, nil
	}
	info, err := os.Stat(localPath)
	if err != nil {
		return ImportFileInfo{}, fmt.Errorf("failed to stat file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(localPath))
	switch ext {
	case ".pdf":
		pdfData, err := os.ReadFile(localPath)
		if err != nil {
			return ImportFileInfo{}, fmt.Errorf("failed to read file: %w", err)
		}
		return ImportFileInfo{
			Path:      localPath,
			Size:      info.Size(),
			PageCount: pdfimport.PageCount(pdfData),
			FileType:  "pdf",
		}, nil
	case ".rmdoc":
		zipData, err := os.ReadFile(localPath)
		if err != nil {
			return ImportFileInfo{}, fmt.Errorf("failed to read file: %w", err)
		}
		rmdocInfo, err := rmdocimport.Inspect(zipData)
		if err != nil {
			return ImportFileInfo{}, fmt.Errorf("failed to inspect rmdoc: %w", err)
		}
		return ImportFileInfo{
			Path:        localPath,
			Size:        info.Size(),
			PageCount:   rmdocInfo.PageCount,
			FileType:    "rmdoc",
			VisibleName: rmdocInfo.VisibleName,
		}, nil
	case ".epub":
		return ImportFileInfo{
			Path:     localPath,
			Size:     info.Size(),
			FileType: "epub",
		}, nil
	default:
		return ImportFileInfo{}, fmt.Errorf("unsupported file type: %s", ext)
	}
}

// The file type comes from the path's extension, as GetImportFileInfo reads it.
type ImportDocument struct {
	Path              string `json:"path"`
	VisibleName       string `json:"visibleName"`
	PageCountOverride int    `json:"pageCountOverride"`
	CoverPageNumber   *int   `json:"coverPageNumber"`
}

// A failed document does not stop the rest, so a retry sends only the ones that did not make it.
func (s *FilesService) ImportDocuments(opID string, documents []ImportDocument, rescanLibrary, restartUI bool) {
	s.runOperation(opID, "Importing documents", func(op *Operation) (any, error) {
		var sftpClient *sftp.Client
		if !s.inDemoMode() {
			client, err := s.getSFTPClient()
			if err != nil {
				return nil, err
			}
			defer client.Close()
			sftpClient = client
		}

		total := len(documents)
		imported := 0
		for index, document := range documents {
			if op.Canceled() {
				return nil, errOperationCanceled
			}
			op.Progress(InstallProgress{Component: document.VisibleName, Index: index, Total: total, Status: InstallImporting})
			if err := s.importDocument(sftpClient, document); err != nil {
				op.Progress(InstallProgress{Component: document.VisibleName, Index: index, Total: total, Status: InstallError, Message: err.Error()})
				continue
			}
			imported++
			op.Progress(InstallProgress{Component: document.VisibleName, Index: index, Total: total, Status: InstallCompleted})
		}

		var refreshErr error
		if imported > 0 {
			switch {
			case rescanLibrary:
				if err := s.config.rescanLibrary(); err != nil {
					refreshErr = fmt.Errorf("imported, but could not rescan the library: %w", err)
				}
			case restartUI:
				if err := s.config.restartXochitl(op); err != nil {
					refreshErr = fmt.Errorf("imported, but could not restart the reMarkable UI: %w", err)
				}
			}
		}

		if failed := total - imported; failed > 0 {
			return nil, fmt.Errorf("%d of %d documents could not be imported", failed, total)
		}
		return nil, refreshErr
	})
}

func (s *FilesService) importDocument(sftpClient *sftp.Client, document ImportDocument) error {
	if _, ok := s.screenshotImportFileInfo(document.Path); ok {
		return s.demoImport()
	}
	switch ext := strings.ToLower(filepath.Ext(document.Path)); ext {
	case ".pdf":
		return s.importPDF(sftpClient, document.Path, document.VisibleName, document.PageCountOverride, document.CoverPageNumber)
	case ".epub":
		return s.importEpub(sftpClient, document.Path, document.VisibleName)
	case ".rmdoc":
		return s.importRmdoc(sftpClient, document.Path, document.VisibleName)
	default:
		return fmt.Errorf("unsupported file type: %s", ext)
	}
}

func (s *FilesService) importEpub(sftpClient *sftp.Client, localPath, visibleName string) error {
	epubData, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("failed to read ePub: %w", err)
	}

	if s.inDemoMode() {
		return s.demoImport()
	}

	_, err = epubimport.Upload(sftpClient, epubData, visibleName, "")
	return err
}

func (s *FilesService) importRmdoc(sftpClient *sftp.Client, localPath, visibleName string) error {
	zipData, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("failed to read rmdoc: %w", err)
	}

	if s.inDemoMode() {
		return s.demoImport()
	}

	_, err = rmdocimport.Upload(sftpClient, zipData, visibleName)
	return err
}

func (s *FilesService) importPDF(sftpClient *sftp.Client, localPath, visibleName string, pageCountOverride int, coverPageNumber *int) error {
	pdfData, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("failed to read PDF: %w", err)
	}

	pageCount := pageCountOverride
	if pageCount <= 0 {
		pageCount = pdfimport.PageCount(pdfData)
	}
	if pageCount <= 0 {
		return fmt.Errorf("Could not determine page count from PDF.")
	}

	if s.inDemoMode() {
		return s.demoImport()
	}

	_, err = pdfimport.Upload(sftpClient, pdfData, visibleName, "", pageCount, coverPageNumber)
	return err
}

func (s *FilesService) fetchGuideManifest() (map[string]struct {
	SHA256 string `json:"sha256"`
}, error) {
	client := httputil.NewClient(10 * time.Second)
	resp, err := client.Get(guideManifestURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch guide manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("guide manifest returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read guide manifest: %w", err)
	}

	var manifest map[string]struct {
		SHA256 string `json:"sha256"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse guide manifest: %w", err)
	}
	return manifest, nil
}

func (s *FilesService) findGuideOnDevice(sftpClient *sftp.Client) (docID string, err error) {
	entries, err := sftpClient.ReadDir(pdfimport.XochitlPath)
	if err != nil {
		return "", fmt.Errorf("failed to read xochitl directory: %w", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".metadata") {
			continue
		}

		metaPath := path.Join(pdfimport.XochitlPath, name)
		f, err := sftpClient.Open(metaPath)
		if err != nil {
			continue
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			continue
		}

		var meta struct {
			VisibleName string `json:"visibleName"`
		}
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		if meta.VisibleName == guideVisibleName {
			return strings.TrimSuffix(name, ".metadata"), nil
		}
	}
	return "", nil
}

// Unseen marks a guide that is missing or stale and has not been pointed out since this version of reManager started, which is when the guide is news.
func (s *FilesService) checkUserGuide() UserGuideStatus {
	status := s.checkUserGuideOnDevice()
	if !status.Skipped && (!status.Installed || status.NeedsUpdate) {
		status.Unseen = !s.guideSeen()
	}
	return status
}

func (s *FilesService) MarkGuideSeen() error {
	if s.inDemoMode() {
		s.mu.Lock()
		s.demoGuideSeen = true
		s.mu.Unlock()
		return nil
	}
	if s.settingsStore == nil {
		return fmt.Errorf("settings store not initialized")
	}
	return s.settingsStore.Update(func(settings *storage.Settings) {
		settings.GuideSeenVersion = version
	})
}

func (s *FilesService) guideSeen() bool {
	if s.inDemoMode() {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.demoGuideSeen
	}
	if s.settingsStore == nil {
		return true
	}
	settings, err := s.settingsStore.Load()
	if err != nil {
		return true
	}
	return settings.GuideSeenVersion == version
}

func (s *FilesService) checkUserGuideOnDevice() UserGuideStatus {
	if s.inDemoMode() {
		return s.demoGuideStatus()
	}
	if version == "dev" {
		return UserGuideStatus{Skipped: true}
	}

	v := strings.TrimPrefix(version, "v")

	manifest, err := s.fetchGuideManifest()
	if err != nil {
		debug.Printf("[DEBUG] checkUserGuideOnDevice: manifest fetch failed: %v\n", err)
		return UserGuideStatus{Skipped: true}
	}

	entry, ok := manifest[v]
	if !ok {
		debug.Printf("[DEBUG] checkUserGuideOnDevice: version %s not in manifest\n", v)
		return UserGuideStatus{Skipped: true}
	}

	sftpClient, err := s.getSFTPClient()
	if err != nil {
		debug.Printf("[DEBUG] checkUserGuideOnDevice: sftp failed: %v\n", err)
		return UserGuideStatus{Skipped: true}
	}
	defer sftpClient.Close()

	docID, err := s.findGuideOnDevice(sftpClient)
	if err != nil {
		debug.Printf("[DEBUG] checkUserGuideOnDevice: scan failed: %v\n", err)
		return UserGuideStatus{}
	}

	if docID == "" {
		return UserGuideStatus{Installed: false, NeedsUpdate: false}
	}

	epubPath := path.Join(pdfimport.XochitlPath, docID+".epub")
	output, err := s.runCommand("sha256sum " + epubPath)
	if err != nil {
		debug.Printf("[DEBUG] checkUserGuideOnDevice: sha256sum failed: %v\n", err)
		return UserGuideStatus{Installed: true, DocID: docID}
	}

	deviceHash := strings.Fields(strings.TrimSpace(output))
	if len(deviceHash) > 0 && deviceHash[0] == entry.SHA256 {
		return UserGuideStatus{Installed: true, DocID: docID}
	}

	return UserGuideStatus{Installed: true, NeedsUpdate: true, DocID: docID}
}

func (s *FilesService) InstallUserGuide(opID string) {
	s.runOperation(opID, "Installing the reManager guide", func(op *Operation) (any, error) {
		return nil, s.installUserGuide()
	})
}

func (s *FilesService) installUserGuide() error {
	if s.inDemoMode() {
		return s.demoInstallGuide()
	}
	if version == "dev" {
		return fmt.Errorf("guide installation not available in dev builds")
	}

	v := strings.TrimPrefix(version, "v")

	downloadURL := guideDownloadURLBase + v + ".epub"
	resp, err := httputil.GetStreaming(s.ctx, downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download guide: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("guide download returned HTTP %d", resp.StatusCode)
	}

	epubData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read guide data: %w", err)
	}

	manifest, err := s.fetchGuideManifest()
	if err != nil {
		return fmt.Errorf("failed to verify guide: %w", err)
	}
	entry, ok := manifest[v]
	if !ok {
		return fmt.Errorf("no guide available for version %s", v)
	}

	h := sha256.Sum256(epubData)
	if fmt.Sprintf("%x", h) != entry.SHA256 {
		return fmt.Errorf("guide checksum mismatch")
	}

	sftpClient, err := s.getSFTPClient()
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	// The old guide goes only once the new one is written, so a failed upload leaves the user with the guide they had.
	existingID, _ := s.findGuideOnDevice(sftpClient)
	if _, err := epubimport.Upload(sftpClient, epubData, guideVisibleName, ""); err != nil {
		return err
	}

	if existingID != "" {
		base := path.Join(pdfimport.XochitlPath, existingID)
		for _, ext := range []string{".metadata", ".epub", ".content", ".pagedata"} {
			sftpClient.Remove(base + ext)
		}
		thumbDir := base + ".thumbnails"
		if entries, err := sftpClient.ReadDir(thumbDir); err == nil {
			for _, e := range entries {
				sftpClient.Remove(path.Join(thumbDir, e.Name()))
			}
			sftpClient.RemoveDirectory(thumbDir)
		}
		cacheDir := base + ".cache"
		if entries, err := sftpClient.ReadDir(cacheDir); err == nil {
			for _, e := range entries {
				sftpClient.Remove(path.Join(cacheDir, e.Name()))
			}
			sftpClient.RemoveDirectory(cacheDir)
		}
	}

	return nil
}
