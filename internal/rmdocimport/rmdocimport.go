package rmdocimport

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/sftp"

	"reManager/internal/pdfimport"
)

type RmdocInfo struct {
	VisibleName  string `json:"visibleName"`
	PageCount    int    `json:"pageCount"`
	OriginalUUID string `json:"originalUUID"`
}

type rmdocMetadata struct {
	CreatedTime    string `json:"createdTime"`
	LastModified   string `json:"lastModified"`
	LastOpened     string `json:"lastOpened"`
	LastOpenedPage int    `json:"lastOpenedPage"`
	Parent         string `json:"parent"`
	Pinned         bool   `json:"pinned"`
	Type           string `json:"type"`
	VisibleName    string `json:"visibleName"`
}

type rmdocContent struct {
	PageCount int `json:"pageCount"`
}

func discoverUUID(zr *zip.Reader) (string, error) {
	for _, f := range zr.File {
		name := path.Base(f.Name)
		if strings.HasSuffix(name, ".metadata") && !strings.Contains(f.Name, "/") {
			return strings.TrimSuffix(name, ".metadata"), nil
		}
	}
	return "", fmt.Errorf("no .metadata file found at root level of rmdoc archive")
}

func readZipEntry(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			buf := new(bytes.Buffer)
			if _, err := buf.ReadFrom(rc); err != nil {
				return nil, err
			}
			return buf.Bytes(), nil
		}
	}
	return nil, fmt.Errorf("entry %q not found in archive", name)
}

func Inspect(zipData []byte) (RmdocInfo, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return RmdocInfo{}, fmt.Errorf("failed to open zip: %w", err)
	}

	origUUID, err := discoverUUID(zr)
	if err != nil {
		return RmdocInfo{}, err
	}

	metaBytes, err := readZipEntry(zr, origUUID+".metadata")
	if err != nil {
		return RmdocInfo{}, fmt.Errorf("failed to read .metadata: %w", err)
	}
	var meta rmdocMetadata
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return RmdocInfo{}, fmt.Errorf("failed to parse .metadata: %w", err)
	}

	var pageCount int
	contentBytes, err := readZipEntry(zr, origUUID+".content")
	if err == nil {
		var content rmdocContent
		if err := json.Unmarshal(contentBytes, &content); err == nil {
			pageCount = content.PageCount
		}
	}

	return RmdocInfo{
		VisibleName:  meta.VisibleName,
		PageCount:    pageCount,
		OriginalUUID: origUUID,
	}, nil
}

func Upload(client *sftp.Client, zipData []byte, visibleName string) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return "", fmt.Errorf("failed to open zip: %w", err)
	}

	origUUID, err := discoverUUID(zr)
	if err != nil {
		return "", err
	}

	newUUID := uuid.NewString()
	nowMs := strconv.FormatInt(time.Now().UnixMilli(), 10)

	dirCreated := false

	for _, f := range zr.File {
		newName := strings.Replace(f.Name, origUUID, newUUID, 1)
		remotePath := path.Join(pdfimport.XochitlPath, newName)

		if f.FileInfo().IsDir() {
			if err := client.MkdirAll(remotePath); err != nil {
				return "", fmt.Errorf("failed to create directory %s: %w", remotePath, err)
			}
			continue
		}

		if !dirCreated && strings.Contains(newName, "/") {
			dir := path.Dir(remotePath)
			if err := client.MkdirAll(dir); err != nil {
				return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
			dirCreated = true
		}

		data, err := readZipEntry(zr, f.Name)
		if err != nil {
			return "", fmt.Errorf("failed to read %s from archive: %w", f.Name, err)
		}

		if strings.HasSuffix(f.Name, ".metadata") && !strings.Contains(f.Name, "/") {
			var meta map[string]interface{}
			if err := json.Unmarshal(data, &meta); err == nil {
				meta["visibleName"] = visibleName
				meta["lastModified"] = nowMs
				if updated, err := json.MarshalIndent(meta, "", "    "); err == nil {
					data = updated
				}
			}
		}

		if err := pdfimport.UploadBytes(client, remotePath, data); err != nil {
			return "", err
		}
	}

	return newUUID, nil
}
