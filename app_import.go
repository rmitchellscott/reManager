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
	"github.com/rymdport/portal/filechooser"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"reManager/internal/debug"
	"reManager/internal/epubimport"
	"reManager/internal/httputil"
	"reManager/internal/pdfimport"
	"reManager/internal/platform"
	"reManager/internal/rmdocimport"
)

const guideManifestURL = "https://raw.githubusercontent.com/rmitchellscott/remanager/main/docs/guide/guide-manifest.json"
const guideDownloadURLBase = "https://raw.githubusercontent.com/rmitchellscott/remanager/main/docs/guide/releases/"
const guideVisibleName = "reManager User Guide"

type PDFFileInfo struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	PageCount int    `json:"pageCount"`
}

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
	DocID       string `json:"docId"`
}

func (a *App) SelectPDFFile() string {
	if platform.IsRunningInFlatpak() {
		home, _ := os.UserHomeDir()
		files, err := filechooser.OpenFile("", "Select PDF", &filechooser.OpenFileOptions{
			CurrentFolder: home,
		})
		if err != nil || len(files) == 0 {
			return ""
		}
		return strings.TrimPrefix(files[0], "file://")
	}
	home, _ := os.UserHomeDir()
	p, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "Select PDF",
		DefaultDirectory: home,
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF files", Pattern: "*.pdf"},
		},
	})
	if err != nil {
		return ""
	}
	return p
}

func (a *App) SelectPDFFiles() []string {
	return a.SelectImportFiles()
}

func (a *App) SelectImportFiles() []string {
	if platform.IsRunningInFlatpak() {
		home, _ := os.UserHomeDir()
		files, err := filechooser.OpenFile("", "Select documents", &filechooser.OpenFileOptions{
			CurrentFolder: home,
			Multiple:      true,
			Filters: []*filechooser.Filter{
				{Name: "Documents", Rules: []filechooser.Rule{
					{Type: filechooser.GlobPattern, Pattern: "*.pdf"},
					{Type: filechooser.GlobPattern, Pattern: "*.epub"},
					{Type: filechooser.GlobPattern, Pattern: "*.rmdoc"},
				}},
			},
		})
		if err != nil || len(files) == 0 {
			return []string{}
		}
		for i, f := range files {
			files[i] = strings.TrimPrefix(f, "file://")
		}
		return files
	}
	home, _ := os.UserHomeDir()
	files, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "Select documents",
		DefaultDirectory: home,
		Filters: []runtime.FileFilter{
			{DisplayName: "Documents (PDF, ePub, rmdoc)", Pattern: "*.pdf;*.epub;*.rmdoc"},
		},
	})
	if err != nil {
		return []string{}
	}
	return files
}

func (a *App) GetPDFFileInfo(localPath string) (PDFFileInfo, error) {
	info, err := os.Stat(localPath)
	if err != nil {
		return PDFFileInfo{}, fmt.Errorf("failed to stat file: %w", err)
	}
	pdfData, err := os.ReadFile(localPath)
	if err != nil {
		return PDFFileInfo{}, fmt.Errorf("failed to read file: %w", err)
	}
	return PDFFileInfo{
		Path:      localPath,
		Size:      info.Size(),
		PageCount: pdfimport.EstimatePageCount(pdfData),
	}, nil
}

func (a *App) GetImportFileInfo(localPath string) (ImportFileInfo, error) {
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
			PageCount: pdfimport.EstimatePageCount(pdfData),
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

func (a *App) ImportEpubFromPath(localPath, visibleName string, restartXochitl bool) error {
	epubData, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("failed to read ePub: %w", err)
	}

	sftpClient, err := a.getSFTPClient()
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	if _, err := epubimport.Upload(sftpClient, epubData, visibleName, ""); err != nil {
		return err
	}

	if restartXochitl {
		if err := a.RestartXochitl(); err != nil {
			return fmt.Errorf("uploaded, but %w", err)
		}
	}

	return nil
}

func (a *App) ImportRmdocFromPath(localPath, visibleName string, restartXochitl bool) error {
	zipData, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("failed to read rmdoc: %w", err)
	}

	sftpClient, err := a.getSFTPClient()
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	if _, err := rmdocimport.Upload(sftpClient, zipData, visibleName); err != nil {
		return err
	}

	if restartXochitl {
		if err := a.RestartXochitl(); err != nil {
			return fmt.Errorf("uploaded, but %w", err)
		}
	}

	return nil
}

func (a *App) ImportPDFFromPath(localPath, visibleName string, restartXochitl bool, pageCountOverride int, coverPageNumber *int) error {
	pdfData, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("failed to read PDF: %w", err)
	}

	pageCount := pageCountOverride
	if pageCount <= 0 {
		pageCount = pdfimport.EstimatePageCount(pdfData)
	}
	if pageCount <= 0 {
		return fmt.Errorf("Could not determine page count from PDF.")
	}

	sftpClient, err := a.getSFTPClient()
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	if _, err := pdfimport.Upload(sftpClient, pdfData, visibleName, "", pageCount, coverPageNumber); err != nil {
		return err
	}

	if restartXochitl {
		if err := a.RestartXochitl(); err != nil {
			return fmt.Errorf("uploaded, but %w", err)
		}
	}

	return nil
}

func (a *App) fetchGuideManifest() (map[string]struct{ SHA256 string `json:"sha256"` }, error) {
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

func (a *App) findGuideOnDevice(sftpClient *sftp.Client) (docID string, err error) {
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

func (a *App) CheckUserGuide() UserGuideStatus {
	if version == "dev" {
		return UserGuideStatus{Skipped: true}
	}

	v := strings.TrimPrefix(version, "v")

	manifest, err := a.fetchGuideManifest()
	if err != nil {
		debug.Printf("[DEBUG] CheckUserGuide: manifest fetch failed: %v\n", err)
		return UserGuideStatus{Skipped: true}
	}

	entry, ok := manifest[v]
	if !ok {
		debug.Printf("[DEBUG] CheckUserGuide: version %s not in manifest\n", v)
		return UserGuideStatus{Skipped: true}
	}

	sftpClient, err := a.getSFTPClient()
	if err != nil {
		debug.Printf("[DEBUG] CheckUserGuide: sftp failed: %v\n", err)
		return UserGuideStatus{Skipped: true}
	}
	defer sftpClient.Close()

	docID, err := a.findGuideOnDevice(sftpClient)
	if err != nil {
		debug.Printf("[DEBUG] CheckUserGuide: scan failed: %v\n", err)
		return UserGuideStatus{}
	}

	if docID == "" {
		return UserGuideStatus{Installed: false, NeedsUpdate: false}
	}

	epubPath := path.Join(pdfimport.XochitlPath, docID+".epub")
	output, err := a.runCommand("sha256sum " + epubPath)
	if err != nil {
		debug.Printf("[DEBUG] CheckUserGuide: sha256sum failed: %v\n", err)
		return UserGuideStatus{Installed: true, DocID: docID}
	}

	deviceHash := strings.Fields(strings.TrimSpace(output))
	if len(deviceHash) > 0 && deviceHash[0] == entry.SHA256 {
		return UserGuideStatus{Installed: true, DocID: docID}
	}

	return UserGuideStatus{Installed: true, NeedsUpdate: true, DocID: docID}
}

func (a *App) InstallUserGuide() error {
	if version == "dev" {
		return fmt.Errorf("guide installation not available in dev builds")
	}

	v := strings.TrimPrefix(version, "v")

	downloadURL := guideDownloadURLBase + v + ".epub"
	resp, err := httputil.GetStreaming(a.ctx, downloadURL)
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

	manifest, err := a.fetchGuideManifest()
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

	sftpClient, err := a.getSFTPClient()
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	existingID, _ := a.findGuideOnDevice(sftpClient)
	if existingID != "" {
		base := path.Join(pdfimport.XochitlPath, existingID)
		for _, ext := range []string{".epub", ".content", ".metadata", ".pagedata"} {
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

	if _, err := epubimport.Upload(sftpClient, epubData, guideVisibleName, ""); err != nil {
		return err
	}

	return nil
}

func (a *App) DismissGuideOffer() error {
	if a.settingsStore == nil {
		return fmt.Errorf("settings store not initialized")
	}
	settings, err := a.settingsStore.Load()
	if err != nil {
		return err
	}
	settings.SuppressGuideOffer = true
	return a.settingsStore.Save(settings)
}

func (a *App) EnableGuideOffer() error {
	if a.settingsStore == nil {
		return fmt.Errorf("settings store not initialized")
	}
	settings, err := a.settingsStore.Load()
	if err != nil {
		return err
	}
	settings.SuppressGuideOffer = false
	return a.settingsStore.Save(settings)
}
