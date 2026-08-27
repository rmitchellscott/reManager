package vellum

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	rmdevice "github.com/rmitchellscott/remarkable-go/device"
	"golang.org/x/crypto/ssh"

	"reManager/internal/httputil"
)

const (
	VellumCLIReleasesAPI = "https://api.github.com/repos/vellum-dev/vellum-cli/releases"
	VellumKeysURL        = "https://raw.githubusercontent.com/vellum-dev/vellum/main/keys/packages.rsa.pub"
	BootstrapDir         = "/tmp/vellum-bootstrap"
	BootstrapTimeout     = 120 * time.Second
)

type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

type BootstrapFiles struct {
	BootstrapScript []byte
	APKBinary       []byte
	VellumBinary    []byte
	RSAKey          []byte
	APKFilename     string
	VellumFilename  string
}

func getLatestVellumRelease() (*GitHubRelease, error) {
	client := httputil.NewClient(30 * time.Second)
	resp, err := client.Get(VellumCLIReleasesAPI)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var releases []GitHubRelease
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, fmt.Errorf("failed to parse releases: %w", err)
	}

	versionRegex := regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)
	var latestRelease *GitHubRelease
	var latestVersion [3]int

	for i := range releases {
		release := &releases[i]
		matches := versionRegex.FindStringSubmatch(release.TagName)
		if matches == nil {
			continue
		}

		var version [3]int
		for i, match := range matches[1:] {
			version[i], _ = strconv.Atoi(match)
		}

		if latestRelease == nil || isNewerVersion(version, latestVersion) {
			latestRelease = release
			latestVersion = version
		}
	}

	if latestRelease == nil {
		return nil, fmt.Errorf("no versioned release found")
	}

	return latestRelease, nil
}

func isNewerVersion(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

func parseAPKToolsVersion(bootstrapScript []byte) (string, error) {
	re := regexp.MustCompile(`APK_TOOLS_VERSION="([^"]+)"`)
	matches := re.FindSubmatch(bootstrapScript)
	if matches == nil {
		return "", fmt.Errorf("APK_TOOLS_VERSION not found in bootstrap.sh")
	}
	return string(matches[1]), nil
}

func getArchFiles(arch rmdevice.Architecture) (apkFile, vellumFile string) {
	switch arch {
	case rmdevice.Aarch64:
		return "apk-aarch64", "vellum-linux-arm64"
	default:
		return "apk-armv7", "vellum-linux-armv7"
	}
}

func loadLocalBootstrapFiles(dir string, arch rmdevice.Architecture, onProgress func(string)) (*BootstrapFiles, error) {
	files := &BootstrapFiles{}
	apkFile, vellumFile := getArchFiles(arch)
	files.APKFilename = apkFile
	files.VellumFilename = vellumFile

	onProgress(fmt.Sprintf("Loading bootstrap files from %s", dir))

	var err error
	files.BootstrapScript, err = os.ReadFile(filepath.Join(dir, "bootstrap.sh"))
	if err != nil {
		return nil, fmt.Errorf("failed to read bootstrap.sh: %w", err)
	}

	files.APKBinary, err = os.ReadFile(filepath.Join(dir, apkFile))
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", apkFile, err)
	}

	files.VellumBinary, err = os.ReadFile(filepath.Join(dir, vellumFile))
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", vellumFile, err)
	}

	files.RSAKey, err = os.ReadFile(filepath.Join(dir, "packages.rsa.pub"))
	if err != nil {
		return nil, fmt.Errorf("failed to read packages.rsa.pub: %w", err)
	}

	onProgress("Loaded all bootstrap files")
	return files, nil
}

func downloadBootstrapFiles(arch rmdevice.Architecture, onProgress func(string)) (*BootstrapFiles, error) {
	if localDir := os.Getenv("VELLUM_BOOTSTRAP_DIR"); localDir != "" {
		return loadLocalBootstrapFiles(localDir, arch, onProgress)
	}

	files := &BootstrapFiles{}
	apkFile, vellumFile := getArchFiles(arch)
	files.APKFilename = apkFile
	files.VellumFilename = vellumFile

	onProgress("Finding latest vellum release...")
	release, err := getLatestVellumRelease()
	if err != nil {
		return nil, fmt.Errorf("failed to get vellum release: %w", err)
	}
	onProgress(fmt.Sprintf("Using vellum %s", release.TagName))

	var bootstrapURL, vellumURL string
	for _, asset := range release.Assets {
		switch asset.Name {
		case "bootstrap.sh":
			bootstrapURL = asset.BrowserDownloadURL
		case vellumFile:
			vellumURL = asset.BrowserDownloadURL
		}
	}

	if bootstrapURL == "" {
		return nil, fmt.Errorf("bootstrap.sh not found in release assets")
	}
	if vellumURL == "" {
		return nil, fmt.Errorf("%s not found in release assets", vellumFile)
	}

	onProgress("Downloading bootstrap.sh...")
	files.BootstrapScript, err = downloadFile(context.Background(), bootstrapURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download bootstrap.sh: %w", err)
	}

	apkVersion, err := parseAPKToolsVersion(files.BootstrapScript)
	if err != nil {
		return nil, err
	}
	onProgress(fmt.Sprintf("Using apk-tools %s", apkVersion))

	apkURL := fmt.Sprintf("https://github.com/vellum-dev/apk-tools/releases/download/%s/%s", apkVersion, apkFile)
	onProgress(fmt.Sprintf("Downloading %s...", apkFile))
	files.APKBinary, err = downloadFile(context.Background(), apkURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download %s: %w", apkFile, err)
	}

	onProgress(fmt.Sprintf("Downloading %s...", vellumFile))
	files.VellumBinary, err = downloadFile(context.Background(), vellumURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download %s: %w", vellumFile, err)
	}

	onProgress("Downloading packages.rsa.pub...")
	files.RSAKey, err = downloadFile(context.Background(), VellumKeysURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download RSA key: %w", err)
	}

	return files, nil
}

func (c *Client) BootstrapOffline(sshClient *ssh.Client, arch rmdevice.Architecture, onOutput func(line string)) error {
	onOutput("Downloading bootstrap files...\n")
	files, err := downloadBootstrapFiles(arch, func(msg string) {
		onOutput(msg + "\n")
	})
	if err != nil {
		return err
	}

	onOutput("Connecting to device...\n")
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	onOutput("Creating bootstrap directory...\n")
	if err := sftpClient.MkdirAll(BootstrapDir); err != nil {
		return fmt.Errorf("failed to create bootstrap dir: %w", err)
	}

	uploadFile := func(name string, data []byte, executable bool) error {
		onOutput(fmt.Sprintf("Transferring %s...\n", name))
		remotePath := fmt.Sprintf("%s/%s", BootstrapDir, name)
		f, err := sftpClient.Create(remotePath)
		if err != nil {
			return fmt.Errorf("failed to create %s: %w", name, err)
		}
		if _, err := f.Write(data); err != nil {
			f.Close()
			return fmt.Errorf("failed to write %s: %w", name, err)
		}
		f.Close()
		if executable {
			if err := sftpClient.Chmod(remotePath, 0755); err != nil {
				return fmt.Errorf("failed to chmod %s: %w", name, err)
			}
		}
		return nil
	}

	if err := uploadFile("bootstrap.sh", files.BootstrapScript, true); err != nil {
		return err
	}
	if err := uploadFile(files.APKFilename, files.APKBinary, true); err != nil {
		return err
	}
	if err := uploadFile(files.VellumFilename, files.VellumBinary, true); err != nil {
		return err
	}
	if err := uploadFile("packages.rsa.pub", files.RSAKey, false); err != nil {
		return err
	}

	onOutput("Running bootstrap...\n")
	noVerify := ""
	if os.Getenv("VELLUM_BOOTSTRAP_DIR") != "" {
		noVerify = "--no-verify "
	}
	cmd := fmt.Sprintf("cd %s && sh bootstrap.sh %s--offline %s", BootstrapDir, noVerify, BootstrapDir)
	if err := c.executor.ExecuteStreaming(cmd, onOutput); err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}

	onOutput("Cleaning up...\n")
	cleanupCmd := fmt.Sprintf("rm -rf %s", BootstrapDir)
	c.executor.ExecuteWithOutput(cleanupCmd)

	return nil
}

func (c *Client) BootstrapOfflineWithPackages(sshClient *ssh.Client, arch rmdevice.Architecture, onOutput func(line string)) error {
	if err := c.BootstrapOffline(sshClient, arch, onOutput); err != nil {
		return err
	}

	repoArch := string(arch)
	if arch == rmdevice.Arm32 {
		repoArch = "armv7"
	}

	proxy := NewProxy(c, sshClient, repoArch)

	if err := proxy.UploadAPKINDEX(context.Background(), func(msg string) {
		onOutput(msg + "\n")
	}); err != nil {
		return fmt.Errorf("failed to upload APKINDEX: %w", err)
	}
	onOutput("Installing required packages...\n")
	packages := []string{"vellum", "mount-utils", "vellum-bash-completion"}

	_, err := proxy.ProxyDownload(context.Background(), packages, func(msg string) {
		onOutput(msg + "\n")
	})
	if err != nil {
		onOutput(fmt.Sprintf("Warning: failed to proxy packages: %v\n", err))
		onOutput("Attempting direct install...\n")
	}

	cmd := fmt.Sprintf("%s add %s", VellumBin, strings.Join(packages, " "))
	if err := c.executor.ExecuteStreaming(cmd, onOutput); err != nil {
		return fmt.Errorf("failed to install packages: %w", err)
	}

	return nil
}
