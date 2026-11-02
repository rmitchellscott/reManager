package main

import (
	"bytes"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"gopkg.in/ini.v1"

	"reManager/internal/debug"
	"reManager/internal/executor"
)

type ConfigService struct {
	*core
}

type BackupInfo struct {
	Name      string `json:"name"`
	Timestamp int64  `json:"timestamp"`
	Size      int64  `json:"size"`
}

func (s *ConfigService) ReadConfigFile() (string, error) {
	sftpClient, err := s.getSFTPClient()
	if err != nil {
		return "", err
	}
	defer sftpClient.Close()

	file, err := sftpClient.Open("/home/root/.config/remarkable/xochitl.conf")
	if err != nil {
		return "", fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("failed to read config file: %w", err)
	}

	return string(content), nil
}

func (s *ConfigService) WriteConfigFile(opID, content string) {
	s.runOperation(opID, "Saving the configuration", func(op *Operation) (any, error) {
		return nil, s.writeConfigFile(content)
	})
}

func (s *ConfigService) writeConfigFile(content string) error {
	_, err := ini.Load([]byte(content))
	if err != nil {
		return fmt.Errorf("invalid INI syntax: %w", err)
	}

	sftpClient, err := s.getSFTPClient()
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	return replaceXochitlConf(sftpClient, "/home/root/.config/remarkable/xochitl.conf", []byte(content))
}

// The live file stays in place until the rename replaces it, because a tablet left without xochitl.conf loses its developer password and Wi-Fi networks.
func replaceXochitlConf(sftpClient *sftp.Client, configPath string, content []byte) error {
	writer, err := openRemoteFileForReplace(sftpClient, configPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	if _, err := writer.Write(content); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to write to temp file: %w", err)
	}
	if err := writer.Commit(int64(len(content))); err != nil {
		return fmt.Errorf("failed to replace config file: %w", err)
	}
	return nil
}

func (s *ConfigService) BackupConfigFile(opID string) {
	s.runOperation(opID, "Backing up the configuration", func(op *Operation) (any, error) {
		backupName, err := s.backupConfigFile()
		if err != nil {
			return nil, err
		}
		return backupName, nil
	})
}

func (s *ConfigService) backupConfigFile() (string, error) {
	sftpClient, err := s.getSFTPClient()
	if err != nil {
		return "", err
	}
	defer sftpClient.Close()

	configPath := "/home/root/.config/remarkable/xochitl.conf"
	file, err := sftpClient.Open(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("failed to read config file: %w", err)
	}

	backupName := fmt.Sprintf("xochitl.conf.backup-%s", time.Now().Format("2006-01-02-150405"))
	backupPath := path.Join("/home/root/.config/remarkable", backupName)

	backupFile, err := sftpClient.Create(backupPath)
	if err != nil {
		return "", fmt.Errorf("failed to create backup file: %w", err)
	}
	defer backupFile.Close()

	_, err = backupFile.Write(content)
	if err != nil {
		return "", fmt.Errorf("failed to write backup file: %w", err)
	}

	return backupName, nil
}

func (s *ConfigService) ListConfigBackups() ([]BackupInfo, error) {
	sftpClient, err := s.getSFTPClient()
	if err != nil {
		return nil, err
	}
	defer sftpClient.Close()

	entries, err := sftpClient.ReadDir("/home/root/.config/remarkable")
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "xochitl.conf.backup-") {
			backups = append(backups, BackupInfo{
				Name:      entry.Name(),
				Timestamp: entry.ModTime().Unix(),
				Size:      entry.Size(),
			})
		}
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Timestamp > backups[j].Timestamp
	})

	return backups, nil
}

func (s *ConfigService) RestoreConfigBackup(opID, backupName string) {
	s.runOperation(opID, "Restoring the configuration", func(op *Operation) (any, error) {
		return nil, s.restoreConfigBackup(backupName)
	})
}

func (s *ConfigService) restoreConfigBackup(backupName string) error {
	debug.Println("RestoreConfigBackup called with backupName:", backupName)

	if !strings.HasPrefix(backupName, "xochitl.conf.backup-") {
		debug.Println("RestoreConfigBackup: invalid backup name")
		return fmt.Errorf("invalid backup name")
	}

	sftpClient, err := s.getSFTPClient()
	if err != nil {
		debug.Println("RestoreConfigBackup: failed to create SFTP client:", err)
		return err
	}
	defer sftpClient.Close()

	backupPath := path.Join("/home/root/.config/remarkable", backupName)
	debug.Println("RestoreConfigBackup: opening backup file:", backupPath)
	file, err := sftpClient.Open(backupPath)
	if err != nil {
		debug.Println("RestoreConfigBackup: failed to open backup file:", err)
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		debug.Println("RestoreConfigBackup: failed to read backup file:", err)
		return fmt.Errorf("failed to read backup file: %w", err)
	}
	debug.Println("RestoreConfigBackup: read", len(content), "bytes from backup")

	_, err = ini.Load(content)
	if err != nil {
		debug.Println("RestoreConfigBackup: backup file has invalid INI syntax:", err)
		return fmt.Errorf("backup file has invalid INI syntax: %w", err)
	}

	debug.Println("RestoreConfigBackup: calling writeConfigFile")
	err = s.writeConfigFile(string(content))
	if err != nil {
		debug.Println("RestoreConfigBackup: writeConfigFile failed:", err)
	} else {
		debug.Println("RestoreConfigBackup: success")
	}
	return err
}

func (s *ConfigService) IsSleepScreenSupported() (bool, error) {
	if s.inDemoMode() {
		return false, nil
	}
	client := s.getClient()

	if client == nil {
		return false, errNotConnected
	}

	output, err := s.runCommand("grep REMARKABLE_RELEASE_VERSION /usr/share/remarkable/update.conf")
	if isTransportError(err) {
		return false, err
	}
	if err != nil {
		output, err = s.runCommand("grep IMG_VERSION /etc/os-release")
		if isTransportError(err) {
			return false, err
		}
		if err != nil {
			return false, nil
		}
	}

	parts := strings.SplitN(output, "=", 2)
	if len(parts) != 2 {
		return false, nil
	}
	version := strings.Trim(strings.TrimSpace(parts[1]), "\"")

	versionParts := strings.Split(version, ".")
	if len(versionParts) < 2 {
		return false, nil
	}

	major, err := strconv.Atoi(versionParts[0])
	if err != nil {
		return false, nil
	}
	minor, err := strconv.Atoi(versionParts[1])
	if err != nil {
		return false, nil
	}

	if major == 3 {
		return (minor >= 2 && minor <= 13) || minor >= 20, nil
	}
	return major > 3, nil
}

func (s *ConfigService) GetSleepScreen() (string, error) {
	sftpClient, err := s.getSFTPClient()
	if err != nil {
		return "", err
	}
	defer sftpClient.Close()

	configPath := "/home/root/.config/remarkable/xochitl.conf"

	file, err := sftpClient.Open(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to open config file: %w", err)
	}
	content, err := io.ReadAll(file)
	file.Close()
	if err != nil {
		return "", fmt.Errorf("failed to read config file: %w", err)
	}

	cfg, err := ini.Load(content)
	if err != nil {
		return "", fmt.Errorf("invalid config file syntax: %w", err)
	}

	section := cfg.Section("General")
	if !section.HasKey("SleepScreenPath") {
		return "", nil
	}
	return section.Key("SleepScreenPath").String(), nil
}

func (s *ConfigService) ResetSleepScreen(opID string) {
	s.runOperation(opID, "Resetting the sleep screen", func(op *Operation) (any, error) {
		return nil, s.resetSleepScreen()
	})
}

func (s *ConfigService) resetSleepScreen() error {
	sftpClient, err := s.getSFTPClient()
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	configPath := "/home/root/.config/remarkable/xochitl.conf"

	file, err := sftpClient.Open(configPath)
	if err != nil {
		return fmt.Errorf("failed to open config file: %w", err)
	}
	content, err := io.ReadAll(file)
	file.Close()
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	cfg, err := ini.Load(content)
	if err != nil {
		return fmt.Errorf("invalid config file syntax: %w", err)
	}

	section := cfg.Section("General")
	section.DeleteKey("SleepScreenPath")

	var buf bytes.Buffer
	_, err = cfg.WriteTo(&buf)
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	return replaceXochitlConf(sftpClient, configPath, buf.Bytes())
}

func (s *ConfigService) SetSleepScreen(opID, imagePath string) {
	s.runOperation(opID, "Setting the sleep screen", func(op *Operation) (any, error) {
		return nil, s.setSleepScreen(imagePath)
	})
}

func (s *ConfigService) setSleepScreen(imagePath string) error {
	sftpClient, err := s.getSFTPClient()
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	configPath := "/home/root/.config/remarkable/xochitl.conf"

	file, err := sftpClient.Open(configPath)
	if err != nil {
		return fmt.Errorf("failed to open config file: %w", err)
	}
	content, err := io.ReadAll(file)
	file.Close()
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	cfg, err := ini.Load(content)
	if err != nil {
		return fmt.Errorf("invalid config file syntax: %w", err)
	}

	section := cfg.Section("General")
	section.Key("SleepScreenPath").SetValue(imagePath)

	var buf bytes.Buffer
	_, err = cfg.WriteTo(&buf)
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	return replaceXochitlConf(sftpClient, configPath, buf.Bytes())
}

func (s *ConfigService) RestartXochitl(opID string) {
	s.runOperation(opID, "Restarting the reMarkable UI", func(op *Operation) (any, error) {
		return nil, s.restartXochitl(op)
	})
}

func (s *ConfigService) restartXochitl(op *Operation) error {
	if err := s.runOperationCommand(op, executor.WithXochitlStartReset(executor.RestartXochitlKeepingMods), false); err != nil {
		return fmt.Errorf("failed to restart xochitl: %w", err)
	}
	return nil
}

const (
	rescanLibraryTimeout   = 30 * time.Second
	rescanLibrarySettle    = 40 * time.Second
	rescanLibraryRetryWait = 2 * time.Second
)

func (s *ConfigService) RescanLibrary(opID string) {
	s.runOperation(opID, "Rescanning the library", func(op *Operation) (any, error) {
		return nil, s.rescanLibrary()
	})
}

func (s *ConfigService) rescanLibrary() error {
	if s.inDemoMode() {
		return nil
	}
	if s.getClient() == nil {
		return errNotConnected
	}
	// librarian answers "library not resolved" then "library not ready" until xochitl loads its library, about 18 s after a start.
	deadline := time.Now().Add(rescanLibrarySettle)
	var out string
	var err error
	for {
		out, err = s.runCommandWithTimeout(`tr '\0' '\n' 2>/dev/null < /proc/$(pidof xochitl)/environ | grep -q 'LD_PRELOAD.*xovi' || { echo 'ERROR: mods are not running'; exit; }; echo '>erescanLibrary:' > /run/xovi-mb; exec cat /run/xovi-mb-out`, rescanLibraryTimeout)
		resp := strings.TrimSpace(out)
		settling := strings.HasPrefix(resp, "ERROR: library not resolved") || strings.HasPrefix(resp, "ERROR: library not ready")
		if err != nil || !settling || time.Now().After(deadline) {
			break
		}
		time.Sleep(rescanLibraryRetryWait)
	}
	resp := strings.TrimSpace(out)
	if err != nil {
		debug.Println("RescanLibrary: command failed:", err, "output:", resp)
		// Not wrapped: the link is fine when the broker goes unanswered, and a timeout in the text would read as a lost connection.
		return fmt.Errorf("the librarian mod did not answer the rescan")
	}

	if strings.HasPrefix(resp, "ERROR:") {
		debug.Println("RescanLibrary: broker returned:", resp)
		return fmt.Errorf("rescan library: %s", resp)
	}

	return nil
}
