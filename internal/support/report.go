package support

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"reManager/internal/httputil"
)

type ReportRequest struct {
	Description string `json:"description"`
	Email       string `json:"email,omitempty"`
	AppVersion  string `json:"app_version,omitempty"`
	HostOS      string `json:"host_os,omitempty"`
	DeviceModel string `json:"device_model,omitempty"`
	Firmware    string `json:"firmware_version,omitempty"`
	BundleURL   string `json:"bundle_url,omitempty"`
}

func SubmitReport(baseURL string, report ReportRequest) error {
	body, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	client := httputil.NewClient(30 * time.Second)
	req, err := http.NewRequest("POST", baseURL+"/api/reports", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to submit report: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("report submission failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}
