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

	"gopkg.in/ini.v1"

	"reManager/internal/debug"
)

type BackupInfo struct {
	Name      string `json:"name"`
	Timestamp int64  `json:"timestamp"`
	Size      int64  `json:"size"`
}

func (a *App) ReadConfigFile() (string, error) {
	sftpClient, err := a.getSFTPClient()
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

func (a *App) WriteConfigFile(content string) error {
	_, err := ini.Load([]byte(content))
	if err != nil {
		return fmt.Errorf("invalid INI syntax: %w", err)
	}

	sftpClient, err := a.getSFTPClient()
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	tmpPath := "/home/root/.config/remarkable/xochitl.conf.tmp"
	finalPath := "/home/root/.config/remarkable/xochitl.conf"

	tmpFile, err := sftpClient.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	_, err = tmpFile.Write([]byte(content))
	if err != nil {
		tmpFile.Close()
		sftpClient.Remove(tmpPath)
		return fmt.Errorf("failed to write to temp file: %w", err)
	}
	tmpFile.Close()

	_ = sftpClient.Remove(finalPath)

	err = sftpClient.Rename(tmpPath, finalPath)
	if err != nil {
		sftpClient.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

func (a *App) BackupConfigFile() (string, error) {
	sftpClient, err := a.getSFTPClient()
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

func (a *App) ListConfigBackups() ([]BackupInfo, error) {
	sftpClient, err := a.getSFTPClient()
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

func (a *App) RestoreConfigBackup(backupName string) error {
	debug.Println("RestoreConfigBackup called with backupName:", backupName)

	if !strings.HasPrefix(backupName, "xochitl.conf.backup-") {
		debug.Println("RestoreConfigBackup: invalid backup name")
		return fmt.Errorf("invalid backup name")
	}

	sftpClient, err := a.getSFTPClient()
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

	debug.Println("RestoreConfigBackup: calling WriteConfigFile")
	err = a.WriteConfigFile(string(content))
	if err != nil {
		debug.Println("RestoreConfigBackup: WriteConfigFile failed:", err)
	} else {
		debug.Println("RestoreConfigBackup: success")
	}
	return err
}

func (a *App) IsSleepScreenSupported() bool {
	client := a.getClient()

	if client == nil {
		return false
	}

	output, err := a.runCommand("grep REMARKABLE_RELEASE_VERSION /usr/share/remarkable/update.conf")
	if err != nil {
		output, err = a.runCommand("grep IMG_VERSION /etc/os-release")
		if err != nil {
			return false
		}
	}

	parts := strings.SplitN(output, "=", 2)
	if len(parts) != 2 {
		return false
	}
	version := strings.Trim(strings.TrimSpace(parts[1]), "\"")

	versionParts := strings.Split(version, ".")
	if len(versionParts) < 2 {
		return false
	}

	major, err := strconv.Atoi(versionParts[0])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(versionParts[1])
	if err != nil {
		return false
	}

	if major == 3 {
		return (minor >= 2 && minor <= 13) || minor >= 20
	}
	return major > 3
}

func (a *App) GetSleepScreen() (string, error) {
	sftpClient, err := a.getSFTPClient()
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

func (a *App) ResetSleepScreen() error {
	sftpClient, err := a.getSFTPClient()
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

	tmpPath := configPath + ".tmp"
	tmpFile, err := sftpClient.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	_, err = tmpFile.Write(buf.Bytes())
	if err != nil {
		tmpFile.Close()
		sftpClient.Remove(tmpPath)
		return fmt.Errorf("failed to write to temp file: %w", err)
	}
	tmpFile.Close()

	_ = sftpClient.Remove(configPath)
	err = sftpClient.Rename(tmpPath, configPath)
	if err != nil {
		sftpClient.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

func (a *App) SetSleepScreen(imagePath string) error {
	sftpClient, err := a.getSFTPClient()
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

	tmpPath := configPath + ".tmp"
	tmpFile, err := sftpClient.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	_, err = tmpFile.Write(buf.Bytes())
	if err != nil {
		tmpFile.Close()
		sftpClient.Remove(tmpPath)
		return fmt.Errorf("failed to write to temp file: %w", err)
	}
	tmpFile.Close()

	_ = sftpClient.Remove(configPath)
	err = sftpClient.Rename(tmpPath, configPath)
	if err != nil {
		sftpClient.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

func (a *App) RestartXochitl() error {
	client := a.getClient()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	_, err = session.CombinedOutput("systemctl restart xochitl")
	if err != nil {
		return fmt.Errorf("failed to restart xochitl: %w", err)
	}

	return nil
}

func (a *App) RescanLibrary() error {
	client := a.getClient()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	out, err := session.CombinedOutput("echo '>erescanLibrary:' > /run/xovi-mb; cat /run/xovi-mb-out")
	if err != nil {
		return fmt.Errorf("failed to rescan library: %w", err)
	}

	resp := strings.TrimSpace(string(out))
	if strings.HasPrefix(resp, "ERROR:") {
		return fmt.Errorf("rescan library: %s", resp)
	}

	return nil
}

