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

	"reManager/internal/debug"
	"reManager/internal/httputil"
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

type proxyPackage struct {
	name, version, url string
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
	debug.Printf("[DEBUG] ProxyDownload called with packages: %v\n", packages)
	onProgress(ProxyProgress{Phase: "index", Message: "Downloading package index..."})

	apkindexURL := fmt.Sprintf("%s/%s/APKINDEX.tar.gz", VellumRepoBaseURL, p.arch)
	debug.Printf("[DEBUG] ProxyDownload downloading APKINDEX from: %s\n", apkindexURL)
	apkindexData, err := downloadFile(apkindexURL)
	if err != nil {
		debug.Printf("[DEBUG] ProxyDownload APKINDEX download failed: %v\n", err)
		return nil, fmt.Errorf("failed to download APKINDEX: %w", err)
	}
	debug.Printf("[DEBUG] ProxyDownload APKINDEX downloaded, size: %d bytes\n", len(apkindexData))

	// Parse APKINDEX to get C: fields
	checksums, err := parseAPKINDEX(apkindexData)
	if err != nil {
		debug.Printf("[DEBUG] ProxyDownload APKINDEX parse failed: %v\n", err)
		return nil, fmt.Errorf("failed to parse APKINDEX: %w", err)
	}
	debug.Printf("[DEBUG] ProxyDownload parsed %d checksums from APKINDEX\n", len(checksums))

	apkindexCacheName := computeAPKINDEXCacheName(apkindexURL)
	remotePath := fmt.Sprintf("%s/%s", VellumCacheDir, apkindexCacheName)
	debug.Printf("[DEBUG] ProxyDownload uploading APKINDEX to: %s\n", remotePath)
	onProgress(ProxyProgress{Phase: "index", Message: fmt.Sprintf("Transferring %s...", apkindexCacheName)})

	if err := p.uploadToDevice(apkindexData, remotePath); err != nil {
		debug.Printf("[DEBUG] ProxyDownload APKINDEX upload failed: %v\n", err)
		return nil, fmt.Errorf("failed to upload APKINDEX: %w", err)
	}
	debug.Printf("[DEBUG] ProxyDownload APKINDEX uploaded successfully\n")

	onProgress(ProxyProgress{Phase: "resolving", Message: "Resolving dependencies..."})
	debug.Printf("[DEBUG] ProxyDownload calling SimulateAddWithVersions with: %v\n", packages)
	toInstall, err := p.client.SimulateAddWithVersions(packages...)
	if err != nil {
		debug.Printf("[DEBUG] ProxyDownload SimulateAddWithVersions failed: %v\n", err)
		return nil, fmt.Errorf("failed to simulate install: %w", err)
	}
	debug.Printf("[DEBUG] ProxyDownload SimulateAddWithVersions returned %d packages: %v\n", len(toInstall), toInstall)

	if len(toInstall) == 0 {
		debug.Printf("[DEBUG] ProxyDownload: nothing to install, returning early\n")
		onProgress(ProxyProgress{Phase: "complete", Message: "All packages already installed"})
		return nil, nil
	}

	var pkgs []proxyPackage
	for _, pkg := range toInstall {
		pkgs = append(pkgs, proxyPackage{
			name:    pkg.Name,
			version: pkg.Version,
			url:     fmt.Sprintf("%s/%s/%s-%s.apk", VellumRepoBaseURL, p.arch, pkg.Name, pkg.Version),
		})
	}

	debug.Printf("[DEBUG] ProxyDownload starting package download loop for %d packages\n", len(pkgs))
	for i, pkg := range pkgs {
		debug.Printf("[DEBUG] ProxyDownload processing package %d/%d: %s-%s\n", i+1, len(pkgs), pkg.name, pkg.version)

		onProgress(ProxyProgress{
			Phase:   "downloading",
			Current: i + 1,
			Total:   len(pkgs),
			Package: pkg.name,
			Message: fmt.Sprintf("Downloading %s (%d/%d)...", pkg.name, i+1, len(pkgs)),
		})

		pkgData, err := downloadFile(pkg.url)
		if err != nil {
			debug.Printf("[DEBUG] ProxyDownload download failed for %s: %v\n", pkg.name, err)
			return nil, fmt.Errorf("failed to download %s: %w", pkg.name, err)
		}
		debug.Printf("[DEBUG] ProxyDownload downloaded %s, size: %d bytes\n", pkg.name, len(pkgData))

		cField, ok := checksums[pkg.name+"-"+pkg.version]
		if !ok {
			cField, ok = checksums[pkg.name]
		}
		if !ok {
			debug.Printf("[DEBUG] ProxyDownload checksum not found for: %s (available: %d checksums)\n", pkg.name, len(checksums))
			return nil, fmt.Errorf("checksum not found for package: %s", pkg.name)
		}
		hash8 := computePackageHash(cField)
		cacheFilename := fmt.Sprintf("%s-%s.%s.apk", pkg.name, pkg.version, hash8)
		debug.Printf("[DEBUG] ProxyDownload computed cache filename: %s (cField=%s, hash8=%s)\n", cacheFilename, cField, hash8)

		remotePath := fmt.Sprintf("%s/%s", VellumCacheDir, cacheFilename)
		onProgress(ProxyProgress{
			Phase:   "transferring",
			Current: i + 1,
			Total:   len(pkgs),
			Package: pkg.name,
			Message: fmt.Sprintf("Transferring %s (%d/%d)...", pkg.name, i+1, len(pkgs)),
		})

		if err := p.uploadToDevice(pkgData, remotePath); err != nil {
			debug.Printf("[DEBUG] ProxyDownload upload failed for %s: %v\n", pkg.name, err)
			return nil, fmt.Errorf("failed to upload %s: %w", pkg.name, err)
		}
		debug.Printf("[DEBUG] ProxyDownload uploaded %s to %s\n", pkg.name, remotePath)
	}

	debug.Printf("[DEBUG] ProxyDownload completed successfully\n")
	onProgress(ProxyProgress{Phase: "complete", Total: len(pkgs), Current: len(pkgs), Message: "All packages downloaded and cached"})

	var names []string
	for _, pkg := range toInstall {
		names = append(names, pkg.Name)
	}
	return names, nil
}

func (p *Proxy) uploadToDevice(data []byte, remotePath string) error {
	sftpClient, err := sftp.NewClient(p.sshClient)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	dir := path.Dir(remotePath)
	if err := sftpClient.MkdirAll(dir); err != nil {
		return fmt.Errorf("failed to create remote directory %s: %w", dir, err)
	}

	f, err := sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create remote file %s: %w", remotePath, err)
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

func downloadFile(url string) ([]byte, error) {
	client := httputil.NewClient(ProxyTimeout)
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
		var currentC, currentP, currentV string
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "C:") {
				currentC = strings.TrimPrefix(line, "C:")
			} else if strings.HasPrefix(line, "P:") {
				currentP = strings.TrimPrefix(line, "P:")
			} else if strings.HasPrefix(line, "V:") {
				currentV = strings.TrimPrefix(line, "V:")
			} else if line == "" {
				if currentC != "" && currentP != "" {
					checksums[currentP] = currentC
					if currentV != "" {
						checksums[currentP+"-"+currentV] = currentC
					}
				}
				currentC = ""
				currentP = ""
				currentV = ""
			}
		}

		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}

	return checksums, nil
}

// ProxyUpgradeDownload downloads APKINDEX, resolves upgradable packages, and uploads them to device cache.
func (p *Proxy) ProxyUpgradeDownload(onProgress func(ProxyProgress)) error {
	debug.Printf("[DEBUG] ProxyUpgradeDownload called\n")
	onProgress(ProxyProgress{Phase: "index", Message: "Downloading package index..."})

	apkindexURL := fmt.Sprintf("%s/%s/APKINDEX.tar.gz", VellumRepoBaseURL, p.arch)
	apkindexData, err := downloadFile(apkindexURL)
	if err != nil {
		return fmt.Errorf("failed to download APKINDEX: %w", err)
	}

	checksums, err := parseAPKINDEX(apkindexData)
	if err != nil {
		return fmt.Errorf("failed to parse APKINDEX: %w", err)
	}

	apkindexCacheName := computeAPKINDEXCacheName(apkindexURL)
	remotePath := fmt.Sprintf("%s/%s", VellumCacheDir, apkindexCacheName)
	onProgress(ProxyProgress{Phase: "index", Message: fmt.Sprintf("Transferring %s...", apkindexCacheName)})

	if err := p.uploadToDevice(apkindexData, remotePath); err != nil {
		return fmt.Errorf("failed to upload APKINDEX: %w", err)
	}

	onProgress(ProxyProgress{Phase: "resolving", Message: "Checking for upgradable packages..."})
	toUpgrade, err := p.client.SimulateUpgradeWithVersions()
	if err != nil {
		return fmt.Errorf("failed to simulate upgrade: %w", err)
	}

	if len(toUpgrade) == 0 {
		debug.Printf("[DEBUG] ProxyUpgradeDownload: no upgrades available\n")
		onProgress(ProxyProgress{Phase: "complete", Message: "No upgrades available"})
		return nil
	}

	debug.Printf("[DEBUG] ProxyUpgradeDownload: %d upgradable packages: %v\n", len(toUpgrade), toUpgrade)

	localOnlyPackages := map[string]bool{
		"remarkable-os": true,
		"rm1":           true,
		"rm2":           true,
		"rmpp":          true,
		"rmppmove":      true,
		"rmppure":       true,
	}

	var pinnedPkgs []proxyPackage
	for _, pkg := range toUpgrade {
		if localOnlyPackages[pkg.Name] {
			continue
		}
		pinnedPkgs = append(pinnedPkgs, proxyPackage{
			name:    pkg.Name,
			version: pkg.Version,
			url:     fmt.Sprintf("%s/%s/%s-%s.apk", VellumRepoBaseURL, p.arch, pkg.Name, pkg.Version),
		})
	}

	for i, pkg := range pinnedPkgs {
		onProgress(ProxyProgress{
			Phase:   "downloading",
			Current: i + 1,
			Total:   len(pinnedPkgs),
			Package: pkg.name,
			Message: fmt.Sprintf("Downloading %s (%d/%d)...", pkg.name, i+1, len(pinnedPkgs)),
		})

		pkgData, err := downloadFile(pkg.url)
		if err != nil {
			return fmt.Errorf("failed to download %s: %w", pkg.name, err)
		}

		cField, ok := checksums[pkg.name+"-"+pkg.version]
		if !ok {
			cField, ok = checksums[pkg.name]
		}
		if !ok {
			return fmt.Errorf("checksum not found for package: %s", pkg.name)
		}
		hash8 := computePackageHash(cField)
		cacheFilename := fmt.Sprintf("%s-%s.%s.apk", pkg.name, pkg.version, hash8)

		pkgRemotePath := fmt.Sprintf("%s/%s", VellumCacheDir, cacheFilename)
		onProgress(ProxyProgress{
			Phase:   "transferring",
			Current: i + 1,
			Total:   len(pinnedPkgs),
			Package: pkg.name,
			Message: fmt.Sprintf("Transferring %s (%d/%d)...", pkg.name, i+1, len(pinnedPkgs)),
		})

		if err := p.uploadToDevice(pkgData, pkgRemotePath); err != nil {
			return fmt.Errorf("failed to upload %s: %w", pkg.name, err)
		}
	}

	onProgress(ProxyProgress{Phase: "complete", Total: len(pinnedPkgs), Current: len(pinnedPkgs), Message: "All upgrade packages downloaded and cached"})
	return nil
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

