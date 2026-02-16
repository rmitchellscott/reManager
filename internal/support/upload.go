package support

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type UploadResponse struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	DeleteToken string `json:"delete_token"`
	CreatedAt   string `json:"created_at"`
}

type AppendResponse struct {
	ID        string `json:"id"`
	UpdatedAt string `json:"updated_at"`
	Size      int64  `json:"size"`
}

type UploadClient struct {
	BaseURL    string
	httpClient *http.Client
}

func GetSupportURL() string {
	if url := os.Getenv("REMANAGER_SUPPORT_URL"); url != "" {
		return url
	}
	return "https://support.remanager.io"
}

func NewUploadClient(baseURL string) *UploadClient {
	return &UploadClient{
		BaseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}
}

func (c *UploadClient) Upload(bundlePath string) (*UploadResponse, error) {
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		part, err := writer.CreateFormFile("file", filepath.Base(bundlePath))
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		f, err := os.Open(bundlePath)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		defer f.Close()
		if _, err := io.Copy(part, f); err != nil {
			pw.CloseWithError(err)
			return
		}
		writer.Close()
		pw.Close()
	}()

	req, err := http.NewRequest("POST", c.BaseURL+"/api/bundles", pr)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("upload failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result UploadResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse upload response: %w", err)
	}
	return &result, nil
}

func (c *UploadClient) Append(bundlePath, remoteID, deleteToken string) (*AppendResponse, error) {
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		part, err := writer.CreateFormFile("file", filepath.Base(bundlePath))
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		f, err := os.Open(bundlePath)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		defer f.Close()
		if _, err := io.Copy(part, f); err != nil {
			pw.CloseWithError(err)
			return
		}
		writer.Close()
		pw.Close()
	}()

	req, err := http.NewRequest("POST", c.BaseURL+"/api/bundles/"+remoteID+"/append", pr)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+deleteToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("append failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("append failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result AppendResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse append response: %w", err)
	}
	return &result, nil
}

func (c *UploadClient) Delete(remoteID, deleteToken string) error {
	req, err := http.NewRequest("DELETE", c.BaseURL+"/api/bundles/"+remoteID, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+deleteToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed (HTTP %d): %s", resp.StatusCode, string(body))
	}
	return nil
}
