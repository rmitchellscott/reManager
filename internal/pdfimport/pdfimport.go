package pdfimport

// Package pdfimport assembles the sidecar files xochitl expects alongside a PDF
// and writes the whole set to the tablet. Used by both the GUI and the CLI.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pkg/sftp"
)

// XochitlPath is scanned by xochitl; a document appears once its bundle lands here.
const XochitlPath = "/home/root/.local/share/remarkable/xochitl"

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

// DocumentUpload records what one import has written to the xochitl directory, so a failed import can remove it again rather than leave xochitl a document with missing or truncated files.
type DocumentUpload struct {
	client  *sftp.Client
	created []string
}

func NewDocumentUpload(client *sftp.Client) *DocumentUpload {
	return &DocumentUpload{client: client}
}

func (u *DocumentUpload) WriteFile(remotePath string, data []byte) error {
	u.created = append(u.created, remotePath)
	return UploadBytes(u.client, remotePath, data)
}

// MkdirAll records only the directories it creates, so Discard never removes one that was already on the tablet.
func (u *DocumentUpload) MkdirAll(dir string) error {
	topMissing := ""
	for d := dir; d != XochitlPath && d != "/" && d != "."; d = path.Dir(d) {
		_, err := u.client.Stat(d)
		if !errors.Is(err, os.ErrNotExist) {
			break
		}
		topMissing = d
	}
	if topMissing != "" {
		u.created = append(u.created, topMissing)
	}
	return u.client.MkdirAll(dir)
}

func (u *DocumentUpload) Discard() {
	for i := len(u.created) - 1; i >= 0; i-- {
		_ = u.client.RemoveAll(u.created[i])
	}
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
	base := path.Join(XochitlPath, docID)

	upload := NewDocumentUpload(client)
	for _, file := range []struct {
		remotePath string
		data       []byte
	}{
		{base + ".pdf", pdfData},
		{base + ".content", contentData},
		{base + ".pagedata", []byte("\n")},
		{base + ".metadata", metadataData},
	} {
		if err := upload.WriteFile(file.remotePath, file.data); err != nil {
			upload.Discard()
			return "", err
		}
	}

	return docID, nil
}
