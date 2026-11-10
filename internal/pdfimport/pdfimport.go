package pdfimport

// Package pdfimport assembles the sidecar files xochitl expects alongside a PDF
// and writes the whole set to the tablet. Used by both the GUI and the CLI.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pkg/sftp"
)

// XochitlPath is scanned by xochitl; a document appears once its bundle lands here.
const XochitlPath = "/home/root/.local/share/remarkable/xochitl"

// ImportStagingPath must stay on XochitlPath's filesystem: publishing a document is a rename, which cannot cross filesystems.
const ImportStagingPath = "/home/root/.local/share/remarkable/.remanager-import"

func init() {
	// pdfcpu creates a config directory under the user's home on first use unless this is set, which a desktop app has no reason to do.
	model.ConfigPath = "disable"
}

type pdfContent struct {
	CoverPageNumber       *int           `json:"coverPageNumber,omitempty"`
	CustomZoomCenterX     int            `json:"customZoomCenterX"`
	CustomZoomCenterY     int            `json:"customZoomCenterY"`
	CustomZoomOrientation string         `json:"customZoomOrientation"`
	CustomZoomPageHeight  int            `json:"customZoomPageHeight"`
	CustomZoomPageWidth   int            `json:"customZoomPageWidth"`
	CustomZoomScale       int            `json:"customZoomScale"`
	DocumentMetadata      map[string]any `json:"documentMetadata"`
	ExtraMetadata         map[string]any `json:"extraMetadata"`
	FileType              string         `json:"fileType"`
	FontName              string         `json:"fontName"`
	FormatVersion         int            `json:"formatVersion"`
	LineHeight            int            `json:"lineHeight"`
	Margins               int            `json:"margins"`
	Orientation           string         `json:"orientation"`
	OriginalPageCount     int            `json:"originalPageCount"`
	PageCount             int            `json:"pageCount"`
	PageTags              []string       `json:"pageTags"`
	Pages                 []string       `json:"pages"`
	RedirectionPageMap    []int          `json:"redirectionPageMap"`
	TextAlignment         string         `json:"textAlignment"`
	TextScale             int            `json:"textScale"`
	Transform             map[string]any `json:"transform"`
	ZoomMode              string         `json:"zoomMode"`
}

type DocMetadata struct {
	CreatedTime    string `json:"createdTime"`
	LastModified   string `json:"lastModified"`
	LastOpened     string `json:"lastOpened"`
	LastOpenedPage int    `json:"lastOpenedPage"`
	Parent         string `json:"parent"`
	Pinned         bool   `json:"pinned"`
	Type           string `json:"type"`
	VisibleName    string `json:"visibleName"`
}

// PageCount returns 0 if the document cannot be parsed; byte-pattern counting fails because PDF 1.5 onward packs page objects into compressed object streams where neither /Type /Page nor /Count appears in the raw bytes.
func PageCount(pdfData []byte) int {
	count, err := api.PageCount(bytes.NewReader(pdfData), model.NewDefaultConfiguration())
	if err != nil {
		return 0
	}
	return count
}

// newContent gives every page its own UUID and an identity entry in the redirection map, so an inaccurate pageCount yields a document xochitl renders with missing or phantom pages.
func newContent(pageCount int, coverPageNumber *int) pdfContent {
	pages := make([]string, pageCount)
	redirection := make([]int, pageCount)
	for i := range pages {
		pages[i] = uuid.NewString()
		redirection[i] = i
	}

	return pdfContent{
		CoverPageNumber:       coverPageNumber,
		CustomZoomCenterX:     0,
		CustomZoomCenterY:     936,
		CustomZoomOrientation: "portrait",
		CustomZoomPageHeight:  1872,
		CustomZoomPageWidth:   1404,
		CustomZoomScale:       1,
		DocumentMetadata:      map[string]any{},
		ExtraMetadata:         map[string]any{},
		FileType:              "pdf",
		FontName:              "",
		FormatVersion:         1,
		LineHeight:            -1,
		Margins:               125,
		Orientation:           "portrait",
		OriginalPageCount:     pageCount,
		PageCount:             pageCount,
		PageTags:              []string{},
		Pages:                 pages,
		RedirectionPageMap:    redirection,
		TextAlignment:         "justify",
		TextScale:             1,
		Transform:             map[string]any{},
		ZoomMode:              "bestFit",
	}
}

func newMetadata(visibleName, parentID string) DocMetadata {
	nowMs := strconv.FormatInt(time.Now().UnixMilli(), 10)
	return DocMetadata{
		CreatedTime:    nowMs,
		LastModified:   nowMs,
		LastOpened:     "0",
		LastOpenedPage: 0,
		Parent:         parentID,
		Pinned:         false,
		Type:           "DocumentType",
		VisibleName:    visibleName,
	}
}

func UploadBytes(client *sftp.Client, remotePath string, data []byte) error {
	remote, err := client.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create remote file %s: %w", remotePath, err)
	}

	if _, err := remote.Write(data); err != nil {
		remote.Close()
		return fmt.Errorf("failed to write remote file %s: %w", remotePath, err)
	}
	if err := remote.Close(); err != nil {
		return fmt.Errorf("failed to write remote file %s: %w", remotePath, err)
	}
	return nil
}

// DocumentUpload stages one document outside xochitl and publishes it only once every file is on the tablet, so an interrupted import cannot leave xochitl a truncated file.
type DocumentUpload struct {
	client     *sftp.Client
	stagingDir string
	published  []string
}

// NewDocumentUpload clears what an interrupted import left in staging, since that import lost the connection it would have cleaned up with.
func NewDocumentUpload(client *sftp.Client, docID string) (*DocumentUpload, error) {
	_ = client.RemoveAll(ImportStagingPath)
	stagingDir := path.Join(ImportStagingPath, docID)
	if err := client.MkdirAll(stagingDir); err != nil {
		return nil, fmt.Errorf("failed to create staging directory %s: %w", stagingDir, err)
	}
	return &DocumentUpload{client: client, stagingDir: stagingDir}, nil
}

func (u *DocumentUpload) WriteFile(name string, data []byte) error {
	return UploadBytes(u.client, path.Join(u.stagingDir, name), data)
}

func (u *DocumentUpload) MkdirAll(dir string) error {
	return u.client.MkdirAll(path.Join(u.stagingDir, dir))
}

// Publish moves .metadata last, so xochitl never finds a .metadata without the rest of its document.
func (u *DocumentUpload) Publish() error {
	entries, err := u.client.ReadDir(u.stagingDir)
	if err != nil {
		return fmt.Errorf("failed to read staging directory %s: %w", u.stagingDir, err)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return !isMetadataFile(entries[i]) && isMetadataFile(entries[j])
	})
	for _, entry := range entries {
		destPath := path.Join(XochitlPath, entry.Name())
		if err := u.client.PosixRename(path.Join(u.stagingDir, entry.Name()), destPath); err != nil {
			return fmt.Errorf("failed to publish %s: %w", entry.Name(), err)
		}
		u.published = append(u.published, destPath)
	}
	u.removeStaging()
	return nil
}

func isMetadataFile(entry os.FileInfo) bool {
	return strings.HasSuffix(entry.Name(), ".metadata")
}

func (u *DocumentUpload) Discard() {
	for i := len(u.published) - 1; i >= 0; i-- {
		_ = u.client.RemoveAll(u.published[i])
	}
	u.removeStaging()
}

func (u *DocumentUpload) removeStaging() {
	_ = u.client.RemoveAll(u.stagingDir)
	_ = u.client.RemoveDirectory(ImportStagingPath)
}

// Upload returns the new document's UUID. An empty parentID files the document at the root rather than inside a collection.
func Upload(client *sftp.Client, pdfData []byte, visibleName, parentID string, pageCount int, coverPageNumber *int) (string, error) {
	contentData, err := json.MarshalIndent(newContent(pageCount, coverPageNumber), "", "    ")
	if err != nil {
		return "", fmt.Errorf("failed to encode .content JSON: %w", err)
	}
	metadataData, err := json.MarshalIndent(newMetadata(visibleName, parentID), "", "    ")
	if err != nil {
		return "", fmt.Errorf("failed to encode .metadata JSON: %w", err)
	}

	docID := uuid.NewString()

	upload, err := NewDocumentUpload(client, docID)
	if err != nil {
		return "", err
	}
	for _, file := range []struct {
		name string
		data []byte
	}{
		{docID + ".pdf", pdfData},
		{docID + ".content", contentData},
		{docID + ".pagedata", []byte("\n")},
		{docID + ".metadata", metadataData},
	} {
		if err := upload.WriteFile(file.name, file.data); err != nil {
			upload.Discard()
			return "", err
		}
	}
	if err := upload.Publish(); err != nil {
		upload.Discard()
		return "", err
	}

	return docID, nil
}
