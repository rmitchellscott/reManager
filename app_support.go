package main

import (
	"fmt"
	"os"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"reManager/internal/platform"
	"reManager/internal/storage"
	"reManager/internal/support"
)

func (a *App) DeleteAllLogs() error {
	if a.logger == nil {
		return nil
	}
	err := a.logger.DeleteAllCommandLogs()
	if err != nil {
		a.logger.LogEvent("APP", "failed to delete command logs: "+err.Error())
		return err
	}
	a.logger.LogEvent("APP", "all command logs deleted by user")
	return nil
}

func (a *App) GetSupportBundlePreview() support.SupportBundlePreview {
	return support.GetPreview()
}

func (a *App) buildGenerator(deviceID string, isConnectedDevice bool, deviceName string) *support.Generator {
	var logDir string
	if a.logger != nil {
		logDir = a.logger.GetLogDir()
	}

	env := runtime.Environment(a.ctx)
	hostOS := env.Platform
	if hostOS == "darwin" {
		hostOS = "macos"
	}
	if platform.IsRunningInFlatpak() {
		hostOS += " (Flatpak)"
	}

	gen := &support.Generator{
		AppVersion: version,
		HostOS:     hostOS,
		LogDir:     logDir,
		DeviceID:   deviceID,
		DeviceName: deviceName,
	}

	settings := a.GetSettings()
	gen.Settings = settings.BehaviorSettings

	if isConnectedDevice {
		deviceInfo := a.GetDeviceInfo()
		if len(deviceInfo) > 0 {
			gen.Device = &support.DeviceInfo{
				MachineType:     deviceInfo["machine"],
				FirmwareVersion: deviceInfo["firmware"],
			}
		}

		if a.vellumClient != nil {
			if versions, err := a.vellumClient.ListInstalledWithVersions(); err == nil {
				gen.InstalledPackages = versions
			}
		}

		osState := a.GetOSVersionState()
		if osState.StoredVersion != "" {
			gen.OSVer = osState.StoredVersion
		}

		updateStatus := a.GetUpdateServiceStatus()
		gen.UpdateService = &support.UpdateServiceStatus{
			Enabled: updateStatus.Enabled,
			Running: updateStatus.Running,
		}

		hashtab := a.CheckHashtabVersion()
		gen.Hashtab = &support.HashtabInfo{
			Installed:      hashtab.Installed,
			HashtabVersion: hashtab.HashtabVersion,
			NeedsRebuild:   hashtab.NeedsRebuild,
		}

		if partInfo, err := a.GetPartitionInfo(); err == nil {
			gen.PartitionInfo = partInfo
		}

		if tz, err := a.GetDeviceTimezone(); err == nil {
			gen.Timezone = tz
		}
	} else if deviceID != "" && a.deviceInfoCache != nil {
		if cached, _ := a.deviceInfoCache.Get(deviceID); cached != nil {
			gen.Device = &support.DeviceInfo{
				MachineType:     cached.MachineType,
				FirmwareVersion: cached.FirmwareVersion,
			}
			gen.InstalledPackages = cached.InstalledPackages
			gen.OSVer = cached.OSVersion
			gen.UpdateService = &support.UpdateServiceStatus{
				Enabled: cached.UpdateServiceEnabled,
				Running: cached.UpdateServiceRunning,
			}
			gen.Hashtab = &support.HashtabInfo{
				Installed:      cached.HashtabInstalled,
				HashtabVersion: cached.HashtabVersion,
				NeedsRebuild:   cached.HashtabNeedsRebuild,
			}
			gen.PartitionInfo = cached.PartitionInfo
			gen.Timezone = cached.Timezone
		}
	}

	return gen
}

func (a *App) GenerateSupportBundle(deviceID string) {
	go func() {
		a.mu.Lock()
		if deviceID == "" {
			deviceID = a.connectedDeviceID
		}
		isConnectedDevice := deviceID == a.connectedDeviceID && a.client != nil
		a.mu.Unlock()

		var deviceName string
		if deviceID != "" && a.deviceStore != nil {
			if d, err := a.deviceStore.Get(deviceID); err == nil {
				deviceName = d.Name
			}
		}

		runtime.EventsEmit(a.ctx, "support-bundle:progress", map[string]string{
			"phase":   "generating",
			"message": "Preparing support bundle...",
		})

		filename := fmt.Sprintf("remanager-support-%s.tar.gz", time.Now().Format("20060102-150405"))
		destPath, err := saveFileDialog(a.ctx, "Save Support Bundle", filename, "")
		if err != nil || destPath == "" {
			if err != nil {
				runtime.EventsEmit(a.ctx, "support-bundle:error", map[string]string{
					"message": err.Error(),
				})
			}
			return
		}

		a.mu.Lock()
		if a.supportBundleID == "" {
			a.supportBundleID = support.GenerateBundleID()
		}
		bundleID := a.supportBundleID
		a.mu.Unlock()

		runtime.EventsEmit(a.ctx, "support-bundle:progress", map[string]string{
			"phase":   "collecting",
			"message": "Collecting device information...",
		})

		gen := a.buildGenerator(deviceID, isConnectedDevice, deviceName)

		if err := gen.Generate(destPath, bundleID); err != nil {
			runtime.EventsEmit(a.ctx, "support-bundle:error", map[string]string{
				"message": fmt.Sprintf("Failed to generate bundle: %v", err),
			})
			return
		}

		if a.logger != nil {
			a.logger.LogEvent("APP", "support bundle generated: "+bundleID)
		}

		runtime.EventsEmit(a.ctx, "support-bundle:complete", map[string]string{
			"path":     destPath,
			"bundleId": bundleID,
		})
	}()
}

func (a *App) UploadSupportBundle(deviceID string) {
	go func() {
		a.mu.Lock()
		if deviceID == "" {
			deviceID = a.connectedDeviceID
		}
		isConnectedDevice := deviceID == a.connectedDeviceID && a.client != nil
		a.mu.Unlock()

		var deviceName string
		if deviceID != "" && a.deviceStore != nil {
			if d, err := a.deviceStore.Get(deviceID); err == nil {
				deviceName = d.Name
			}
		}

		runtime.EventsEmit(a.ctx, "support-bundle:progress", map[string]string{
			"phase":   "generating",
			"message": "Generating support bundle...",
		})

		bundleID := support.GenerateBundleID()
		gen := a.buildGenerator(deviceID, isConnectedDevice, deviceName)

		data, err := gen.GenerateToBuffer(bundleID)
		if err != nil {
			runtime.EventsEmit(a.ctx, "support-bundle:upload-error", map[string]string{
				"message": fmt.Sprintf("Failed to generate bundle: %v", err),
			})
			return
		}

		tmpFile, err := os.CreateTemp("", "remanager-bundle-*.tar.gz")
		if err != nil {
			runtime.EventsEmit(a.ctx, "support-bundle:upload-error", map[string]string{
				"message": fmt.Sprintf("Failed to create temp file: %v", err),
			})
			return
		}
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)

		if _, err := tmpFile.Write(data); err != nil {
			tmpFile.Close()
			runtime.EventsEmit(a.ctx, "support-bundle:upload-error", map[string]string{
				"message": fmt.Sprintf("Failed to write temp file: %v", err),
			})
			return
		}
		tmpFile.Close()

		runtime.EventsEmit(a.ctx, "support-bundle:progress", map[string]string{
			"phase":   "uploading",
			"message": "Uploading support bundle...",
		})

		client := support.NewUploadClient(support.GetSupportURL())
		result, err := client.Upload(tmpPath)
		if err != nil {
			runtime.EventsEmit(a.ctx, "support-bundle:upload-error", map[string]string{
				"message": fmt.Sprintf("Upload failed: %v", err),
			})
			return
		}

		if a.bundleStore != nil {
			a.bundleStore.Add(storage.BundleRecord{
				ID:         bundleID,
				RemoteID:   result.ID,
				URL:        result.URL,
				DeviceName: deviceName,
				DeviceID:   deviceID,
				UploadedAt: time.Now().Unix(),
			}, result.DeleteToken)
		}

		if a.logger != nil {
			a.logger.LogEvent("APP", "support bundle uploaded: "+result.ID)
		}

		runtime.EventsEmit(a.ctx, "support-bundle:upload-complete", map[string]string{
			"url":      result.URL,
			"remoteId": result.ID,
		})
	}()
}

func (a *App) AppendSupportBundle(remoteID, deviceID string) {
	go func() {
		if a.bundleStore == nil {
			runtime.EventsEmit(a.ctx, "support-bundle:append-error", map[string]string{
				"message": "Bundle store not available",
			})
			return
		}

		deleteToken, err := a.bundleStore.GetDeleteToken(remoteID)
		if err != nil {
			runtime.EventsEmit(a.ctx, "support-bundle:append-error", map[string]string{
				"message": "Could not retrieve bundle credentials",
			})
			return
		}

		a.mu.Lock()
		if deviceID == "" {
			deviceID = a.connectedDeviceID
		}
		isConnectedDevice := deviceID == a.connectedDeviceID && a.client != nil
		a.mu.Unlock()

		var deviceName string
		if deviceID != "" && a.deviceStore != nil {
			if d, err := a.deviceStore.Get(deviceID); err == nil {
				deviceName = d.Name
			}
		}

		runtime.EventsEmit(a.ctx, "support-bundle:progress", map[string]string{
			"phase":   "generating",
			"message": "Generating support bundle...",
		})

		bundleID := support.GenerateBundleID()
		gen := a.buildGenerator(deviceID, isConnectedDevice, deviceName)

		data, err := gen.GenerateToBuffer(bundleID)
		if err != nil {
			runtime.EventsEmit(a.ctx, "support-bundle:append-error", map[string]string{
				"message": fmt.Sprintf("Failed to generate bundle: %v", err),
			})
			return
		}

		tmpFile, err := os.CreateTemp("", "remanager-bundle-*.tar.gz")
		if err != nil {
			runtime.EventsEmit(a.ctx, "support-bundle:append-error", map[string]string{
				"message": fmt.Sprintf("Failed to create temp file: %v", err),
			})
			return
		}
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)

		if _, err := tmpFile.Write(data); err != nil {
			tmpFile.Close()
			runtime.EventsEmit(a.ctx, "support-bundle:append-error", map[string]string{
				"message": fmt.Sprintf("Failed to write temp file: %v", err),
			})
			return
		}
		tmpFile.Close()

		runtime.EventsEmit(a.ctx, "support-bundle:progress", map[string]string{
			"phase":   "uploading",
			"message": "Appending to support bundle...",
		})

		client := support.NewUploadClient(support.GetSupportURL())
		_, err = client.Append(tmpPath, remoteID, deleteToken)
		if err != nil {
			runtime.EventsEmit(a.ctx, "support-bundle:append-error", map[string]string{
				"message": fmt.Sprintf("Append failed: %v", err),
			})
			return
		}

		now := time.Now().Unix()
		a.bundleStore.UpdateLastAppended(remoteID, now)

		if a.logger != nil {
			a.logger.LogEvent("APP", "support bundle appended: "+remoteID)
		}

		runtime.EventsEmit(a.ctx, "support-bundle:append-complete", map[string]string{
			"remoteId": remoteID,
		})
	}()
}

func (a *App) DeleteSupportBundle(remoteID string) error {
	if a.bundleStore == nil {
		return fmt.Errorf("bundle store not available")
	}

	deleteToken, err := a.bundleStore.GetDeleteToken(remoteID)
	if err != nil {
		return fmt.Errorf("could not retrieve bundle credentials")
	}

	client := support.NewUploadClient(support.GetSupportURL())
	if err := client.Delete(remoteID, deleteToken); err != nil {
		return err
	}

	return a.bundleStore.Remove(remoteID)
}

func (a *App) GetBundleHistory() []storage.BundleRecord {
	if a.bundleStore == nil {
		return []storage.BundleRecord{}
	}
	records, err := a.bundleStore.GetAll()
	if err != nil {
		return []storage.BundleRecord{}
	}
	return records
}

func (a *App) CopyToClipboard(text string) {
	runtime.ClipboardSetText(a.ctx, text)
}

func (a *App) SubmitProblemReport(description, email, bundleURL, deviceID string) error {
	env := runtime.Environment(a.ctx)
	hostOS := env.Platform
	if hostOS == "darwin" {
		hostOS = "macOS"
	}
	if platform.IsRunningInFlatpak() {
		hostOS += " (Flatpak)"
	}

	a.mu.Lock()
	deviceInfo := make(map[string]string)
	isConnectedDevice := deviceID != "" && deviceID == a.connectedDeviceID && a.client != nil
	a.mu.Unlock()

	if isConnectedDevice {
		deviceInfo = a.GetDeviceInfo()
	} else if deviceID != "" && a.deviceInfoCache != nil {
		if cached, _ := a.deviceInfoCache.Get(deviceID); cached != nil {
			deviceInfo["machine"] = cached.MachineType
			deviceInfo["firmware"] = cached.FirmwareVersion
		}
	}

	report := support.ReportRequest{
		Description: description,
		Email:       email,
		AppVersion:  version,
		HostOS:      hostOS,
		DeviceModel: deviceInfo["machine"],
		Firmware:    deviceInfo["firmware"],
		BundleURL:   bundleURL,
	}

	return support.SubmitReport(support.GetSupportURL(), report)
}

func (a *App) GetSupportBundleSessionID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.supportBundleID
}

func (a *App) ResetSupportBundleSession() {
	a.mu.Lock()
	a.supportBundleID = ""
	a.mu.Unlock()
}
