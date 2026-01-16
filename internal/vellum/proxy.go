package vellum

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

const (
	VellumRepoBaseURL = "https://packages.vellum.delivery"
	VellumCacheDir    = "/home/root/.vellum/etc/apk/cache"
	ProxyTimeout      = 60 * time.Second
)

type Proxy struct {
	client    *Client
	sshClient *ssh.Client
	arch      string
}

func NewProxy(client *Client, sshClient *ssh.Client, arch string) *Proxy {
	// Map architecture names to vellum repo format
	repoArch := arch
	if arch == "arm32" {
		repoArch = "armv7"
	}

	return &Proxy{
		client:    client,
		sshClient: sshClient,
		arch:      repoArch,
	}
}

type ProxyProgress struct {
	Phase   string // "index", "resolving", "downloading", "transferring", "complete"
	Current int
	Total   int
	Package string
	Message string
}

// ProxyDownloadWithProgress downloads packages with structured progress reporting.
func (p *Proxy) ProxyDownloadWithProgress(packages []string, onProgress func(ProxyProgress)) ([]string, error) {
	return p.proxyDownloadInternal(packages, onProgress)
}

// ProxyDownload downloads APKINDEX and packages, uploads them to device cache.
// Returns the list of all package names that will be installed (including dependencies).
func (p *Proxy) ProxyDownload(packages []string, onProgress func(string)) ([]string, error) {
	return p.proxyDownloadInternal(packages, func(progress ProxyProgress) {
		onProgress(progress.Message)
	})
}

func (p *Proxy) proxyDownloadInternal(packages []string, onProgress func(ProxyProgress)) ([]string, error) {
	fmt.Printf("[DEBUG] ProxyDownload called with packages: %v\n", packages)
	onProgress(ProxyProgress{Phase: "index", Message: "Downloading package index..."})

	apkindexURL := fmt.Sprintf("%s/%s/APKINDEX.tar.gz", VellumRepoBaseURL, p.arch)
	fmt.Printf("[DEBUG] ProxyDownload downloading APKINDEX from: %s\n", apkindexURL)
	apkindexData, err := downloadFile(apkindexURL)
	if err != nil {
		fmt.Printf("[DEBUG] ProxyDownload APKINDEX download failed: %v\n", err)
		return nil, fmt.Errorf("failed to download APKINDEX: %w", err)
	}
	fmt.Printf("[DEBUG] ProxyDownload APKINDEX downloaded, size: %d bytes\n", len(apkindexData))

	// Parse APKINDEX to get C: fields
	checksums, err := parseAPKINDEX(apkindexData)
	if err != nil {
		fmt.Printf("[DEBUG] ProxyDownload APKINDEX parse failed: %v\n", err)
		return nil, fmt.Errorf("failed to parse APKINDEX: %w", err)
	}
	fmt.Printf("[DEBUG] ProxyDownload parsed %d checksums from APKINDEX\n", len(checksums))

	apkindexCacheName := computeAPKINDEXCacheName(apkindexURL)
	remotePath := fmt.Sprintf("%s/%s", VellumCacheDir, apkindexCacheName)
	fmt.Printf("[DEBUG] ProxyDownload uploading APKINDEX to: %s\n", remotePath)
	onProgress(ProxyProgress{Phase: "index", Message: fmt.Sprintf("Transferring %s...", apkindexCacheName)})

	if err := p.uploadToDevice(apkindexData, remotePath); err != nil {
		fmt.Printf("[DEBUG] ProxyDownload APKINDEX upload failed: %v\n", err)
		return nil, fmt.Errorf("failed to upload APKINDEX: %w", err)
	}
	fmt.Printf("[DEBUG] ProxyDownload APKINDEX uploaded successfully\n")

	onProgress(ProxyProgress{Phase: "resolving", Message: "Resolving dependencies..."})
	fmt.Printf("[DEBUG] ProxyDownload calling SimulateAdd with: %v\n", packages)
	toInstall, err := p.client.SimulateAdd(packages...)
	if err != nil {
		fmt.Printf("[DEBUG] ProxyDownload SimulateAdd failed: %v\n", err)
		return nil, fmt.Errorf("failed to simulate install: %w", err)
	}
	fmt.Printf("[DEBUG] ProxyDownload SimulateAdd returned %d packages: %v\n", len(toInstall), toInstall)

	if len(toInstall) == 0 {
		fmt.Printf("[DEBUG] ProxyDownload: nothing to install, returning early\n")
		onProgress(ProxyProgress{Phase: "complete", Message: "All packages already installed"})
		return nil, nil
	}

	fmt.Printf("[DEBUG] ProxyDownload calling FetchURLs with: %v\n", toInstall)
	urls, err := p.client.FetchURLs(toInstall...)
	if err != nil {
		fmt.Printf("[DEBUG] ProxyDownload FetchURLs failed: %v\n", err)
		return nil, fmt.Errorf("failed to get package URLs: %w", err)
	}
	fmt.Printf("[DEBUG] ProxyDownload FetchURLs returned %d URLs: %v\n", len(urls), urls)

	fmt.Printf("[DEBUG] ProxyDownload starting package download loop for %d URLs\n", len(urls))
	for i, url := range urls {
		fmt.Printf("[DEBUG] ProxyDownload processing URL %d/%d: %s\n", i+1, len(urls), url)
		// Skip local paths (not https://)
		if !strings.HasPrefix(url, "https://") {
			fmt.Printf("[DEBUG] ProxyDownload skipping non-https URL: %s\n", url)
			continue
		}

		pkgName, pkgVersion := parsePackageURL(url)
		if pkgName == "" {
			fmt.Printf("[DEBUG] ProxyDownload failed to parse URL: %s\n", url)
			continue
		}
		fmt.Printf("[DEBUG] ProxyDownload parsed package: name=%s, version=%s\n", pkgName, pkgVersion)

		onProgress(ProxyProgress{
			Phase:   "downloading",
			Current: i + 1,
			Total:   len(urls),
			Package: pkgName,
			Message: fmt.Sprintf("Downloading %s (%d/%d)...", pkgName, i+1, len(urls)),
		})

		pkgData, err := downloadFile(url)
		if err != nil {
			fmt.Printf("[DEBUG] ProxyDownload download failed for %s: %v\n", pkgName, err)
			return nil, fmt.Errorf("failed to download %s: %w", pkgName, err)
		}
		fmt.Printf("[DEBUG] ProxyDownload downloaded %s, size: %d bytes\n", pkgName, len(pkgData))

		cField, ok := checksums[pkgName]
		if !ok {
			fmt.Printf("[DEBUG] ProxyDownload checksum not found for: %s (available: %d checksums)\n", pkgName, len(checksums))
			return nil, fmt.Errorf("checksum not found for package: %s", pkgName)
		}
		hash8 := computePackageHash(cField)
		cacheFilename := fmt.Sprintf("%s-%s.%s.apk", pkgName, pkgVersion, hash8)
		fmt.Printf("[DEBUG] ProxyDownload computed cache filename: %s (cField=%s, hash8=%s)\n", cacheFilename, cField, hash8)

		remotePath := fmt.Sprintf("%s/%s", VellumCacheDir, cacheFilename)
		onProgress(ProxyProgress{
			Phase:   "transferring",
			Current: i + 1,
			Total:   len(urls),
			Package: pkgName,
			Message: fmt.Sprintf("Transferring %s (%d/%d)...", pkgName, i+1, len(urls)),
		})

		if err := p.uploadToDevice(pkgData, remotePath); err != nil {
			fmt.Printf("[DEBUG] ProxyDownload upload failed for %s: %v\n", pkgName, err)
			return nil, fmt.Errorf("failed to upload %s: %w", pkgName, err)
		}
		fmt.Printf("[DEBUG] ProxyDownload uploaded %s to %s\n", pkgName, remotePath)
	}

	fmt.Printf("[DEBUG] ProxyDownload completed successfully\n")
	onProgress(ProxyProgress{Phase: "complete", Total: len(urls), Current: len(urls), Message: "All packages downloaded and cached"})
	return toInstall, nil
}

func (p *Proxy) uploadToDevice(data []byte, remotePath string) error {
	sftpClient, err := sftp.NewClient(p.sshClient)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	f, err := sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create remote file %s: %w", remotePath, err)
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

func downloadFile(url string) ([]byte, error) {
	client := &http.Client{Timeout: ProxyTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// parseAPKINDEX extracts package name → C: field mapping from APKINDEX.tar.gz
func parseAPKINDEX(data []byte) (map[string]string, error) {
	checksums := make(map[string]string)

	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if hdr.Name != "APKINDEX" {
			continue
		}

		scanner := bufio.NewScanner(tr)
		var currentC string
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "C:") {
				currentC = strings.TrimPrefix(line, "C:")
			} else if strings.HasPrefix(line, "P:") {
				pkgName := strings.TrimPrefix(line, "P:")
				if currentC != "" {
					checksums[pkgName] = currentC
				}
			} else if line == "" {
				currentC = ""
			}
		}

		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}

	return checksums, nil
}

// UploadAPKINDEX downloads the APKINDEX and uploads it to device cache.
// This should be called before vellum update to ensure the index is available.
func (p *Proxy) UploadAPKINDEX(onProgress func(string)) error {
	onProgress("Downloading package index...")

	apkindexURL := fmt.Sprintf("%s/%s/APKINDEX.tar.gz", VellumRepoBaseURL, p.arch)
	apkindexData, err := downloadFile(apkindexURL)
	if err != nil {
		return fmt.Errorf("failed to download APKINDEX: %w", err)
	}

	apkindexCacheName := computeAPKINDEXCacheName(apkindexURL)
	remotePath := fmt.Sprintf("%s/%s", VellumCacheDir, apkindexCacheName)
	onProgress(fmt.Sprintf("Transferring %s...", apkindexCacheName))

	if err := p.uploadToDevice(apkindexData, remotePath); err != nil {
		return fmt.Errorf("failed to upload APKINDEX: %w", err)
	}

	return nil
}

// computeAPKINDEXCacheName returns the cache filename for APKINDEX
// Format: APKINDEX.{sha256(url)[:8]}.tar.gz
func computeAPKINDEXCacheName(url string) string {
	hash := sha256.Sum256([]byte(url))
	hash8 := hex.EncodeToString(hash[:])[:8]
	return fmt.Sprintf("APKINDEX.%s.tar.gz", hash8)
}

// computePackageHash computes the 8-char hash from C: field
// C: field format: Q1{base64} where base64 is SHA1 of package content
func computePackageHash(cField string) string {
	if strings.HasPrefix(cField, "Q1") {
		cField = cField[2:]
	}

	decoded, err := base64.StdEncoding.DecodeString(cField)
	if err != nil {
		return ""
	}

	return hex.EncodeToString(decoded)[:8]
}

// parsePackageURL extracts package name and version from URL
// URL format: https://packages.vellum.delivery/aarch64/pkg-name-1.0.0-r0.apk
func parsePackageURL(url string) (name, version string) {
	filename := path.Base(url)

	if !strings.HasSuffix(filename, ".apk") {
		return "", ""
	}
	filename = strings.TrimSuffix(filename, ".apk")

	// Parse name-version format
	// Version typically ends with -rN (e.g., 1.0.0-r0)
	// Find the last occurrence of version pattern
	parts := strings.Split(filename, "-")
	if len(parts) < 3 {
		return "", ""
	}

	// Find where version starts (usually contains digits and dots)
	versionStart := -1
	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]
		// Check if this looks like a revision (r0, r1, etc.)
		if strings.HasPrefix(part, "r") && len(part) >= 2 {
			continue
		}
		// Check if this looks like a version number
		if len(part) > 0 && (part[0] >= '0' && part[0] <= '9') {
			versionStart = i
			break
		}
	}

	if versionStart < 1 {
		return "", ""
	}

	name = strings.Join(parts[:versionStart], "-")
	version = strings.Join(parts[versionStart:], "-")

	return name, version
}
