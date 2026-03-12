package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/sftp"
	"github.com/spf13/cobra"
)

const xochitlPath = "/home/root/.local/share/remarkable/xochitl"

type pdfContent struct {
	CoverPageNumber       int                    `json:"coverPageNumber"`
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

type docMetadata struct {
	CreatedTime    string `json:"createdTime"`
	LastModified   string `json:"lastModified"`
	LastOpened     string `json:"lastOpened"`
	LastOpenedPage int    `json:"lastOpenedPage"`
	Parent         string `json:"parent"`
	Pinned         bool   `json:"pinned"`
	Type           string `json:"type"`
	VisibleName    string `json:"visibleName"`
}

func importPDFCmd() *cobra.Command {
	var visibleName string
	var parentID string
	var pageCountOverride int
	var restartXochitl bool

	cmd := &cobra.Command{
		Use:   "import-pdf [pdf-path]",
		Short: "Import a PDF to remarkable, generating the proper metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pdfPath := args[0]
			pdfData, err := os.ReadFile(pdfPath)
			if err != nil {
				return fmt.Errorf("failed to read PDF: %w", err)
			}

			pageCount := pageCountOverride
			if pageCount <= 0 {
				pageCount = estimatePDFPageCount(pdfData)
			}
			if pageCount <= 0 {
				return fmt.Errorf("could not determine page count from PDF; retry with --pages")
			}

			if visibleName == "" {
				visibleName = strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))
			}

			pages := make([]string, pageCount)
			redirection := make([]int, pageCount)
			for i := 0; i < pageCount; i++ {
				pages[i] = uuid.NewString()
				redirection[i] = i
			}

			nowMs := strconv.FormatInt(time.Now().UnixMilli(), 10)
			content := pdfContent{
				CoverPageNumber:       0,
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

			metadata := docMetadata{
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
				return fmt.Errorf("failed to encode .content JSON: %w", err)
			}
			metadataData, err := json.MarshalIndent(metadata, "", "    ")
			if err != nil {
				return fmt.Errorf("failed to encode .metadata JSON: %w", err)
			}

			client, deviceType, err := connect()
			if err != nil {
				return err
			}
			defer client.Close()

			fmt.Printf("Connected to %s (%s)\n", host, deviceType)
			sftpClient, err := sftp.NewClient(client)
			if err != nil {
				return fmt.Errorf("failed to create SFTP client: %w", err)
			}
			defer sftpClient.Close()

			docID := uuid.NewString()
			base := path.Join(xochitlPath, docID)
			if err := uploadBytes(sftpClient, base+".pdf", pdfData); err != nil {
				return err
			}
			if err := uploadBytes(sftpClient, base+".content", contentData); err != nil {
				return err
			}
			if err := uploadBytes(sftpClient, base+".metadata", metadataData); err != nil {
				return err
			}
			if err := uploadBytes(sftpClient, base+".pagedata", []byte("\n")); err != nil {
				return err
			}

			fmt.Println("Upload complete.")
			fmt.Printf("Document ID: %s\n", docID)
			fmt.Printf("Uploaded files under: %s\n", xochitlPath)

			if restartXochitl {
				session, err := client.NewSession()
				if err != nil {
					return fmt.Errorf("failed to create SSH session for restart: %w", err)
				}
				defer session.Close()
				if _, err := session.CombinedOutput("systemctl restart xochitl"); err != nil {
					return fmt.Errorf("uploaded, but failed to restart xochitl: %w", err)
				}
				fmt.Println("xochitl restarted.")
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&visibleName, "name", "", "Visible document name (defaults to filename)")
	cmd.Flags().StringVar(&parentID, "parent", "", "Parent collection (default: root)")
	cmd.Flags().IntVar(&pageCountOverride, "pages", 0, "Override page count if auto-detection fails")
	cmd.Flags().BoolVar(&restartXochitl, "restart-xochitl", true, "Restart xochitl after upload")

	return cmd
}

func uploadBytes(client *sftp.Client, remotePath string, data []byte) error {
	remoteFile, err := client.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create remote file %s: %w", remotePath, err)
	}
	defer remoteFile.Close()

	if _, err := remoteFile.Write(data); err != nil {
		return fmt.Errorf("failed to write remote file %s: %w", remotePath, err)
	}

	return nil
}

func estimatePDFPageCount(pdfData []byte) int {
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
