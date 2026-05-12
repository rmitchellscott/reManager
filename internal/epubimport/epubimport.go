package epubimport

import (
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/sftp"

	"reManager/internal/pdfimport"
)

type epubContent struct {
	DummyDocument         bool                   `json:"dummyDocument"`
	ExtraMetadata         map[string]interface{} `json:"extraMetadata"`
	FileType              string                 `json:"fileType"`
	FontName              string                 `json:"fontName"`
	FormatVersion         int                    `json:"formatVersion"`
	LineHeight            int                    `json:"lineHeight"`
	Margins               int                    `json:"margins"`
	Orientation           string                 `json:"orientation"`
	PageCount             int                    `json:"pageCount"`
	Pages                 []string               `json:"pages"`
	PageTags              []string               `json:"pageTags"`
	RedirectionPageMap    []int                  `json:"redirectionPageMap"`
	TextAlignment         string                 `json:"textAlignment"`
	TextScale             int                    `json:"textScale"`
	DocumentMetadata      map[string]interface{} `json:"documentMetadata"`
	CustomZoomCenterX     int                    `json:"customZoomCenterX"`
	CustomZoomCenterY     int                    `json:"customZoomCenterY"`
	CustomZoomOrientation string                 `json:"customZoomOrientation"`
	CustomZoomPageHeight  int                    `json:"customZoomPageHeight"`
	CustomZoomPageWidth   int                    `json:"customZoomPageWidth"`
	CustomZoomScale       int                    `json:"customZoomScale"`
	ZoomMode              string                 `json:"zoomMode"`
}

func Upload(client *sftp.Client, epubData []byte, visibleName, parentID string) (string, error) {
	nowMs := strconv.FormatInt(time.Now().UnixMilli(), 10)

	content := epubContent{
		DummyDocument:         false,
		ExtraMetadata:         map[string]interface{}{},
		FileType:              "epub",
		FontName:              "",
		FormatVersion:         1,
		LineHeight:            -1,
		Margins:               180,
		Orientation:           "portrait",
		PageCount:             0,
		Pages:                 []string{},
		PageTags:              []string{},
		RedirectionPageMap:    []int{},
		TextAlignment:         "justify",
		TextScale:             1,
		DocumentMetadata:      map[string]interface{}{},
		CustomZoomCenterX:     0,
		CustomZoomCenterY:     936,
		CustomZoomOrientation: "portrait",
		CustomZoomPageHeight:  1872,
		CustomZoomPageWidth:   1404,
		CustomZoomScale:       1,
		ZoomMode:              "bestFit",
	}

	metadata := pdfimport.DocMetadata{
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
	base := path.Join(pdfimport.XochitlPath, docID)

	for _, u := range []struct {
		remote string
		data   []byte
	}{
		{base + ".epub", epubData},
		{base + ".content", contentData},
		{base + ".metadata", metadataData},
		{base + ".pagedata", []byte("\n")},
	} {
		if err := pdfimport.UploadBytes(client, u.remote, u.data); err != nil {
			return "", err
		}
	}

	return docID, nil
}
