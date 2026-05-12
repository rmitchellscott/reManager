package pdfimport

// Package pdfimport handles building and uploading the xochitl document bundle
// for a PDF file. Shared between the GUI (app.go) and the CLI (cmd/remanager).

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/sftp"
)

// XochitlPath is the reMarkable directory that xochitl watches for documents.
const XochitlPath = "/home/root/.local/share/remarkable/xochitl"

type pdfContent struct {
	CoverPageNumber       *int                   `json:"coverPageNumber,omitempty"`
	CustomZoomCenterX     int                    `json:"customZoomCenterX"`
	CustomZoomCenterY     int                    `json:"customZoomCenterY"`
	CustomZoomOrientation string                 `json:"customZoomOrientation"`
	CustomZoomPageHeight  int                    `json:"customZoomPageHeight"`
	CustomZoomPageWidth   int                    `json:"customZoomPageWidth"`
	CustomZoomScale       int                    `json:"customZoomScale"`
	DocumentMetadata      map[string]interface{} `json:"documentMetadata"`
	ExtraMetadata         map[string]interface{} `json:"extraMetadata"`
	FileType              string                 `json:"fileType"`
	FontName              string                 `json:"fontName"`
	FormatVersion         int                    `json:"formatVersion"`
	LineHeight            int                    `json:"lineHeight"`
	Margins               int                    `json:"margins"`
	Orientation           string                 `json:"orientation"`
	OriginalPageCount     int                    `json:"originalPageCount"`
	PageCount             int                    `json:"pageCount"`
	PageTags              []string               `json:"pageTags"`
	Pages                 []string               `json:"pages"`
	RedirectionPageMap    []int                  `json:"redirectionPageMap"`
	TextAlignment         string                 `json:"textAlignment"`
	TextScale             int                    `json:"textScale"`
	Transform             map[string]interface{} `json:"transform"`
	ZoomMode              string                 `json:"zoomMode"`
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

// EstimatePageCount attempts to count PDF pages via byte-pattern matching.
// Returns 0 if it cannot determine the count.
func EstimatePageCount(pdfData []byte) int {
	pageRe := regexp.MustCompile(`/Type\s*/Page\b`)
	pagesRe := regexp.MustCompile(`/Type\s*/Pages\b`)
	countRe := regexp.MustCompile(`/Count\s+(\d+)`)

	pageHits := len(pageRe.FindAllIndex(pdfData, -1))
	pagesHits := len(pagesRe.FindAllIndex(pdfData, -1))
	if pageHits-pagesHits > 0 {
		return pageHits - pagesHits
	}

	matches := countRe.FindAllSubmatch(pdfData, -1)
	maxCount := 0
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		n, err := strconv.Atoi(string(m[1]))
		if err != nil {
			continue
		}
		if n > maxCount {
			maxCount = n
		}
	}
	return maxCount
}

// UploadBytes writes data to a new remote file over SFTP.
func UploadBytes(client *sftp.Client, remotePath string, data []byte) error {
	f, err := client.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create remote file %s: %w", remotePath, err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write remote file %s: %w", remotePath, err)
	}
	return nil
}

// Upload writes the PDF file and all xochitl sidecar files (.content, .metadata,
// .pagedata) for the given PDF data. visibleName is the display name shown in
// xochitl; parentID is the UUID of a collection, or "" for the root.
// Returns the document UUID on success.
func Upload(client *sftp.Client, pdfData []byte, visibleName, parentID string, pageCount int, coverPageNumber *int) (string, error) {
	pages := make([]string, pageCount)
	redirection := make([]int, pageCount)
	for i := 0; i < pageCount; i++ {
		pages[i] = uuid.NewString()
		redirection[i] = i
	}

	nowMs := strconv.FormatInt(time.Now().UnixMilli(), 10)

	content := pdfContent{
		CoverPageNumber:       coverPageNumber,
		CustomZoomCenterX:     0,
		CustomZoomCenterY:     936,
		CustomZoomOrientation: "portrait",
		CustomZoomPageHeight:  1872,
		CustomZoomPageWidth:   1404,
		CustomZoomScale:       1,
		DocumentMetadata:      map[string]interface{}{},
		ExtraMetadata:         map[string]interface{}{},
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
		Transform:             map[string]interface{}{},
		ZoomMode:              "bestFit",
	}

	metadata := DocMetadata{
		CreatedTime:    nowMs,
		LastModified:   nowMs,
		LastOpened:     "0",
		LastOpenedPage: 0,
		Parent:         parentID,
		Pinned:         false,
		Type:           "DocumentType",
		VisibleName:    visibleName,
	}

	contentData, err := json.MarshalIndent(content, "", "    ")
	if err != nil {
		return "", fmt.Errorf("failed to encode .content JSON: %w", err)
	}
	metadataData, err := json.MarshalIndent(metadata, "", "    ")
	if err != nil {
		return "", fmt.Errorf("failed to encode .metadata JSON: %w", err)
	}

	docID := uuid.NewString()
	base := path.Join(XochitlPath, docID)

	for _, u := range []struct {
		remote string
		data   []byte
	}{
		{base + ".pdf", pdfData},
		{base + ".content", contentData},
		{base + ".metadata", metadataData},
		{base + ".pagedata", []byte("\n")},
	} {
		if err := UploadBytes(client, u.remote, u.data); err != nil {
			return "", err
		}
	}

	return docID, nil
}
