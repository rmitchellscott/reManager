package support

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type BundleInfo struct {
	AppVersion    string   `json:"appVersion"`
	Timestamp     string   `json:"timestamp"`
	BundleID      string   `json:"bundleId"`
	HostOS        string   `json:"hostOS"`
	FormatVersion string   `json:"formatVersion"`
	DeviceID      string   `json:"deviceId,omitempty"`
	DeviceName    string   `json:"deviceName,omitempty"`
	Errors        []string `json:"errors,omitempty"`
}

type DeviceInfo struct {
	MachineType     string `json:"machineType"`
	FirmwareVersion string `json:"firmwareVersion"`
	Architecture    string `json:"architecture"`
}

type UpdateServiceStatus struct {
	Enabled bool `json:"enabled"`
	Running bool `json:"running"`
}

type HashtabInfo struct {
	Installed      bool   `json:"installed"`
	HashtabVersion string `json:"hashtabVersion,omitempty"`
	NeedsRebuild   bool   `json:"needsRebuild"`
}

type Generator struct {
	AppVersion        string
	HostOS            string
	LogDir            string
	DeviceID          string
	DeviceName        string
	Device            *DeviceInfo
	InstalledPackages map[string]string
	OSVer             string
	Settings          any
	UpdateService     *UpdateServiceStatus
	Hashtab           *HashtabInfo
	Timezone          string
	PartitionInfo     any
}

func GenerateBundleID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (g *Generator) GenerateToBuffer(bundleID string) ([]byte, error) {
	var buf bytes.Buffer

	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	var bundleErrors []string

	info := BundleInfo{
		AppVersion:    g.AppVersion,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		BundleID:      bundleID,
		HostOS:        g.HostOS,
		FormatVersion: "1",
		DeviceID:      g.DeviceID,
		DeviceName:    g.DeviceName,
	}

	if g.Device != nil {
		deviceJSON, _ := json.MarshalIndent(g.Device, "", "  ")
		if err := writeEntry(tw, "support-bundle/device/device_info.json", deviceJSON); err != nil {
			tw.Close()
			gzw.Close()
			return nil, err
		}
	}

	if g.InstalledPackages != nil {
		pkgsJSON, _ := json.MarshalIndent(g.InstalledPackages, "", "  ")
		if err := writeEntry(tw, "support-bundle/device/installed_packages.json", pkgsJSON); err != nil {
			tw.Close()
			gzw.Close()
			return nil, err
		}
	}

	if g.OSVer != "" {
		if err := writeEntry(tw, "support-bundle/device/osver", []byte(g.OSVer)); err != nil {
			tw.Close()
			gzw.Close()
			return nil, err
		}
	}

	if g.UpdateService != nil {
		data, _ := json.MarshalIndent(g.UpdateService, "", "  ")
		if err := writeEntry(tw, "support-bundle/device/update_service.json", data); err != nil {
			tw.Close()
			gzw.Close()
			return nil, err
		}
	}

	if g.Hashtab != nil {
		data, _ := json.MarshalIndent(g.Hashtab, "", "  ")
		if err := writeEntry(tw, "support-bundle/device/hashtab.json", data); err != nil {
			tw.Close()
			gzw.Close()
			return nil, err
		}
	}

	if g.PartitionInfo != nil {
		data, _ := json.MarshalIndent(g.PartitionInfo, "", "  ")
		if err := writeEntry(tw, "support-bundle/device/partition_info.json", data); err != nil {
			tw.Close()
			gzw.Close()
			return nil, err
		}
	}

	if g.Timezone != "" {
		if err := writeEntry(tw, "support-bundle/device/timezone", []byte(g.Timezone)); err != nil {
			tw.Close()
			gzw.Close()
			return nil, err
		}
	}

	if g.Settings != nil {
		data, _ := json.MarshalIndent(g.Settings, "", "  ")
		if err := writeEntry(tw, "support-bundle/settings.json", data); err != nil {
			tw.Close()
			gzw.Close()
			return nil, err
		}
	}

	if g.LogDir != "" {
		if err := g.addLogFiles(tw); err != nil {
			bundleErrors = append(bundleErrors, fmt.Sprintf("log files: %v", err))
		}
	}

	info.Errors = bundleErrors
	infoJSON, _ := json.MarshalIndent(info, "", "  ")
	if err := writeEntry(tw, "support-bundle/BUNDLE_INFO.json", infoJSON); err != nil {
		tw.Close()
		gzw.Close()
		return nil, err
	}

	if err := tw.Close(); err != nil {
		gzw.Close()
		return nil, fmt.Errorf("failed to finalize tar: %w", err)
	}
	if err := gzw.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize gzip: %w", err)
	}

	return buf.Bytes(), nil
}

func (g *Generator) Generate(destPath, bundleID string) error {
	data, err := g.GenerateToBuffer(bundleID)
	if err != nil {
		return err
	}
	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write bundle file: %w", err)
	}
	return nil
}

func (g *Generator) addLogFiles(tw *tar.Writer) error {
	entries, err := os.ReadDir(g.LogDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}

		filePath := filepath.Join(g.LogDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		if err := writeEntry(tw, "support-bundle/logs/"+entry.Name(), data); err != nil {
			return err
		}
	}

	if g.DeviceID != "" {
		g.addDeviceCommandLogs(tw, g.DeviceID)
	} else {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			g.addDeviceCommandLogs(tw, entry.Name())
		}
	}

	return nil
}

func (g *Generator) addDeviceCommandLogs(tw *tar.Writer, deviceID string) {
	dir := filepath.Join(g.LogDir, deviceID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		writeEntry(tw, "support-bundle/logs/commands/"+entry.Name(), data)
	}
}

func writeEntry(tw *tar.Writer, name string, data []byte) error {
	header := &tar.Header{
		Name:    name,
		Size:    int64(len(data)),
		Mode:    0644,
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write header for %s: %w", name, err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("failed to write data for %s: %w", name, err)
	}
	return nil
}

type SupportBundlePreview struct {
	Included []string `json:"included"`
	Excluded []string `json:"excluded"`
}

func GetPreview() SupportBundlePreview {
	return SupportBundlePreview{
		Included: []string{
			"Device name, type, OS version, and partition info",
			"Device timezone",
			"Auto-update service status",
			"Hashtab version",
			"Installed package names and versions",
			"reManager behavior settings",
			"reManager application and command logs",
			"reManager version and host OS type",
		},
		Excluded: []string{
			"SSH passwords or key passphrases",
			"SSH private keys",
			"Personal documents, notebooks, or files",
			"Device serial number",
			"Network configuration or WiFi passwords",
		},
	}
}
