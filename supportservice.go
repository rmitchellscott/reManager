package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"reManager/internal/debug"
	"reManager/internal/platform"
	"reManager/internal/storage"
	"reManager/internal/support"
)

type SupportService struct {
	*core
	system   *SystemService
	os       *OSService
	device   *DeviceService
	packages *PackagesService

	bundleStore     *storage.BundleStore
	supportBundleID string
}

func (s *SupportService) ServiceStartup(context.Context, application.ServiceOptions) error {
	bundleStore, err := storage.NewBundleStore()
	if err != nil {
		fmt.Printf("Warning: could not initialize bundle store: %v\n", err)
	}
	s.bundleStore = bundleStore
	return nil
}

func (s *SupportService) DeleteAllLogs() error {
	if s.logger == nil {
		return nil
	}
	err := s.logger.DeleteAllCommandLogs()
	if err != nil {
		s.logger.LogEvent("APP", "failed to delete command logs: "+err.Error())
		return err
	}
	s.logger.LogEvent("APP", "all command logs deleted by user")
	return nil
}

func (s *SupportService) GetSupportBundlePreview() support.SupportBundlePreview {
	return support.GetPreview()
}

func (s *SupportService) buildGenerator(deviceID string, isConnectedDevice bool, deviceName string) *support.Generator {
	var logDir string
	if s.logger != nil {
		logDir = s.logger.GetLogDir()
	}

	host := s.detectHostPlatform()

	gen := &support.Generator{
		AppVersion:          version,
		HostOS:              host.OS,
		HostOSVersion:       host.Version,
		HostModel:           host.Model,
		DistributionChannel: platform.DistributionChannel(),
		LogDir:              logDir,
		DeviceID:            deviceID,
		DeviceName:          deviceName,
	}

	settings := s.system.GetSettings()
	gen.Settings = settings.BehaviorSettings

	if isConnectedDevice {
		deviceInfo := s.device.GetDeviceInfo()
		if len(deviceInfo) > 0 {
			gen.Device = &support.DeviceInfo{
				MachineType:     deviceInfo["machine"],
				FirmwareVersion: deviceInfo["firmware"],
			}
		}

		if vc := s.getVellumClient(); vc != nil {
			if versions, err := vc.ListInstalledWithVersions(); err == nil {
				gen.InstalledPackages = versions
			}
		}

		osState := s.packages.getOSVersionState()
		if osState.StoredVersion != "" {
			gen.OSVer = osState.StoredVersion
		}

		if updateStatus, err := s.device.GetUpdateServiceStatus(); err == nil {
			gen.UpdateService = &support.UpdateServiceStatus{
				Enabled: updateStatus.Enabled,
				Running: updateStatus.Running,
			}
		}

		if hashtab, err := s.device.CheckHashtabVersion(); err == nil {
			gen.Hashtab = &support.HashtabInfo{
				Installed:      hashtab.Installed,
				HashtabVersion: hashtab.HashtabVersion,
				NeedsRebuild:   hashtab.NeedsRebuild,
			}
		}

		if xochitlStatus, err := s.device.GetXochitlStatus(); err == nil {
			_, launcherctlInstalled := gen.InstalledPackages["launcherctl"]
			gen.Launcher = &support.LauncherInfo{
				Installed: launcherctlInstalled,
				Current:   xochitlStatus.CurrentLauncher,
				Active:    xochitlStatus.ActiveLauncher,
			}
		}

		if partInfo, err := s.os.GetPartitionInfo(); err == nil {
			gen.PartitionInfo = partInfo
		}

		if tz, err := s.device.getDeviceTimezone(); err == nil {
			gen.Timezone = tz
		}

		if sftpClient, err := s.getSFTPClient(); err == nil {
			xoviTree := support.BuildDirTree(sftpClient, support.XoviDir)
			gen.XoviTree = &xoviTree
			sftpClient.Close()
		}
	} else if deviceID != "" && s.deviceInfoCache != nil {
		if cached, _ := s.deviceInfoCache.Get(deviceID); cached != nil {
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
			gen.Launcher = &support.LauncherInfo{
				Installed: cached.LauncherctlInstalled,
				Current:   cached.CurrentLauncher,
				Active:    cached.ActiveLauncher,
			}
			gen.PartitionInfo = cached.PartitionInfo
			gen.Timezone = cached.Timezone
		}
	}

	return gen
}

// A saved bundle has a Path, an uploaded one a URL.
type SupportBundleResult struct {
	Path     string `json:"path,omitempty"`
	BundleID string `json:"bundleId,omitempty"`
	URL      string `json:"url,omitempty"`
	RemoteID string `json:"remoteId,omitempty"`
	// False when the upload worked but its delete token was not stored, so the bundle cannot be updated or deleted from this device.
	SavedOnDevice bool `json:"savedOnDevice"`
}

func (s *SupportService) GenerateSupportBundle(opID, deviceID string) {
	s.runOperation(opID, "Generating a support bundle", func(op *Operation) (any, error) {
		if s.inDemoMode() && s.takeDemoFailure() {
			return nil, errors.New(demoFailureMessage)
		}
		s.mu.Lock()
		if deviceID == "" {
			deviceID = s.connectedDeviceID
		}
		isConnectedDevice := deviceID == s.connectedDeviceID && s.client != nil
		s.mu.Unlock()

		var deviceName string
		if deviceID != "" && s.deviceStore != nil {
			if d, err := s.deviceStore.Get(deviceID); err == nil {
				deviceName = d.Name
			}
		}

		op.Progress(InstallProgress{Status: InstallGenerating, Message: "Preparing support bundle…"})

		filename := fmt.Sprintf("remanager-support-%s.tar.gz", time.Now().Format("20060102-150405"))
		destPath, err := saveFileDialog("Save support bundle", filename, "")
		if err != nil {
			return nil, err
		}
		if destPath == "" {
			return nil, errOperationCanceled
		}

		s.mu.Lock()
		if s.supportBundleID == "" {
			s.supportBundleID = support.GenerateBundleID()
		}
		bundleID := s.supportBundleID
		s.mu.Unlock()

		op.Progress(InstallProgress{Status: InstallCollecting, Message: "Collecting information from your reMarkable…"})

		gen := s.buildGenerator(deviceID, isConnectedDevice, deviceName)

		if err := gen.Generate(destPath, bundleID); err != nil {
			return nil, fmt.Errorf("Could not generate the bundle: %v", err)
		}

		if s.logger != nil {
			s.logger.LogEvent("APP", "support bundle generated: "+bundleID)
		}

		shareFile(destPath)
		return SupportBundleResult{Path: destPath, BundleID: bundleID}, nil
	})
}

func (s *SupportService) UploadSupportBundle(opID, deviceID string) {
	s.runOperation(opID, "Uploading a support bundle", func(op *Operation) (any, error) {
		if s.inDemoMode() && s.takeDemoFailure() {
			return nil, errors.New(demoFailureMessage)
		}
		if !support.UploadAvailable() {
			return nil, support.ErrNoSupportService
		}
		s.mu.Lock()
		if deviceID == "" {
			deviceID = s.connectedDeviceID
		}
		isConnectedDevice := deviceID == s.connectedDeviceID && s.client != nil
		s.mu.Unlock()

		var deviceName string
		if deviceID != "" && s.deviceStore != nil {
			if d, err := s.deviceStore.Get(deviceID); err == nil {
				deviceName = d.Name
			}
		}

		op.Progress(InstallProgress{Status: InstallGenerating, Message: "Generating support bundle…"})

		bundleID := support.GenerateBundleID()
		gen := s.buildGenerator(deviceID, isConnectedDevice, deviceName)

		data, err := gen.GenerateToBuffer(bundleID)
		if err != nil {
			return nil, fmt.Errorf("Could not generate the bundle: %v", err)
		}

		tmpFile, err := os.CreateTemp("", "remanager-bundle-*.tar.gz")
		if err != nil {
			return nil, fmt.Errorf("Could not write a temporary file: %v", err)
		}
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)

		if _, err := tmpFile.Write(data); err != nil {
			tmpFile.Close()
			return nil, fmt.Errorf("Could not write a temporary file: %v", err)
		}
		tmpFile.Close()

		op.Progress(InstallProgress{Status: InstallUploading, Message: "Uploading support bundle…"})

		client := support.NewUploadClient(support.GetSupportURL())
		result, err := client.Upload(tmpPath)
		if err != nil {
			return nil, fmt.Errorf("Upload failed: %v", err)
		}

		savedOnDevice := false
		if s.bundleStore != nil {
			err := s.bundleStore.Add(storage.BundleRecord{
				ID:         bundleID,
				RemoteID:   result.ID,
				URL:        result.URL,
				DeviceName: deviceName,
				DeviceID:   deviceID,
				UploadedAt: time.Now().Unix(),
			}, result.DeleteToken)
			if err != nil {
				debug.Printf("[DEBUG] support bundle %s uploaded but not saved: %v\n", result.ID, err)
			}
			savedOnDevice = err == nil
		}

		if s.logger != nil {
			s.logger.LogEvent("APP", "support bundle uploaded: "+result.ID)
		}

		return SupportBundleResult{URL: result.URL, RemoteID: result.ID, SavedOnDevice: savedOnDevice}, nil
	})
}

func (s *SupportService) AppendSupportBundle(opID, remoteID, deviceID string) {
	s.runOperation(opID, "Updating a support bundle", func(op *Operation) (any, error) {
		if s.inDemoMode() && s.takeDemoFailure() {
			return nil, errors.New(demoFailureMessage)
		}
		if !support.UploadAvailable() {
			return nil, support.ErrNoSupportService
		}
		if s.bundleStore == nil {
			return nil, errors.New("Could not open saved bundles.")
		}

		deleteToken, err := s.bundleStore.GetDeleteToken(remoteID)
		if err != nil {
			return nil, errors.New("Could not retrieve the bundle's credentials.")
		}

		s.mu.Lock()
		if deviceID == "" {
			deviceID = s.connectedDeviceID
		}
		isConnectedDevice := deviceID == s.connectedDeviceID && s.client != nil
		s.mu.Unlock()

		var deviceName string
		if deviceID != "" && s.deviceStore != nil {
			if d, err := s.deviceStore.Get(deviceID); err == nil {
				deviceName = d.Name
			}
		}

		op.Progress(InstallProgress{Status: InstallGenerating, Message: "Generating support bundle…"})

		bundleID := support.GenerateBundleID()
		gen := s.buildGenerator(deviceID, isConnectedDevice, deviceName)

		data, err := gen.GenerateToBuffer(bundleID)
		if err != nil {
			return nil, fmt.Errorf("Could not generate the bundle: %v", err)
		}

		tmpFile, err := os.CreateTemp("", "remanager-bundle-*.tar.gz")
		if err != nil {
			return nil, fmt.Errorf("Could not write a temporary file: %v", err)
		}
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)

		if _, err := tmpFile.Write(data); err != nil {
			tmpFile.Close()
			return nil, fmt.Errorf("Could not write a temporary file: %v", err)
		}
		tmpFile.Close()

		op.Progress(InstallProgress{Status: InstallUploading, Message: "Updating support bundle…"})

		client := support.NewUploadClient(support.GetSupportURL())
		if _, err := client.Append(tmpPath, remoteID, deleteToken); err != nil {
			return nil, fmt.Errorf("Could not update the bundle: %v", err)
		}

		now := time.Now().Unix()
		if err := s.bundleStore.UpdateLastAppended(remoteID, now); err != nil {
			debug.Printf("[DEBUG] support bundle %s appended but its new expiry was not saved: %v\n", remoteID, err)
		}

		if s.logger != nil {
			s.logger.LogEvent("APP", "support bundle appended: "+remoteID)
		}

		return nil, nil
	})
}

func (s *SupportService) DeleteSupportBundle(remoteID string) error {
	if !support.UploadAvailable() {
		return support.ErrNoSupportService
	}
	if s.bundleStore == nil {
		return fmt.Errorf("bundle store not available")
	}

	deleteToken, err := s.bundleStore.GetDeleteToken(remoteID)
	if err != nil {
		return fmt.Errorf("could not retrieve bundle credentials")
	}

	client := support.NewUploadClient(support.GetSupportURL())
	if err := client.Delete(remoteID, deleteToken); err != nil {
		return err
	}

	return s.bundleStore.Remove(remoteID)
}

func (s *SupportService) GetBundleHistory() []storage.BundleRecord {
	if s.bundleStore == nil {
		return []storage.BundleRecord{}
	}
	records, err := s.bundleStore.GetAll()
	if err != nil {
		return []storage.BundleRecord{}
	}
	return records
}

func (s *SupportService) CopyToClipboard(text string) {
	s.wails.Clipboard.SetText(text)
}

func (s *SupportService) SubmitProblemReport(description, email, bundleURL, deviceID string) error {
	if !support.UploadAvailable() {
		return support.ErrNoSupportService
	}
	hostOS := s.wails.Env.Info().OS
	if hostOS == "darwin" {
		hostOS = "macOS"
	}

	s.mu.Lock()
	deviceInfo := make(map[string]string)
	isConnectedDevice := deviceID != "" && deviceID == s.connectedDeviceID && (s.client != nil || s.demo)
	demo := s.demo
	s.mu.Unlock()

	// A report filed from demo mode is still a real message, and support needs to know the device in it was fabricated.
	if demo {
		hostOS += " (demo mode)"
	}

	if isConnectedDevice {
		deviceInfo = s.device.GetDeviceInfo()
	} else if deviceID != "" && s.deviceInfoCache != nil {
		if cached, _ := s.deviceInfoCache.Get(deviceID); cached != nil {
			deviceInfo["machine"] = cached.MachineType
			deviceInfo["firmware"] = cached.FirmwareVersion
		}
	}

	report := support.ReportRequest{
		Description:         description,
		Email:               email,
		AppVersion:          version,
		HostOS:              hostOS,
		DistributionChannel: platform.DistributionChannel(),
		DeviceModel:         deviceInfo["machine"],
		Firmware:            deviceInfo["firmware"],
		BundleURL:           bundleURL,
	}

	return support.SubmitReport(support.GetSupportURL(), report)
}
