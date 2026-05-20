package updater

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// githubAPIBase is the base URL for GitHub API calls. It can be overridden in tests.
var githubAPIBase = "https://api.github.com"

// githubReleaseBase is the base URL for release asset downloads. It can be overridden in tests.
var githubReleaseBase = "https://github.com/m00nk0d3/nexus/releases/download"

// maxDownloadBytes caps the size of any single HTTP download (archive or checksums file)
// to prevent runaway disk writes from unexpectedly large or malformed responses.
const maxDownloadBytes = 200 * 1024 * 1024 // 200 MB

// ReleaseInfo holds the relevant fields returned by the GitHub releases/latest API.
type ReleaseInfo struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"` // Release notes / changelog markdown
}

// CheckLatestRelease queries the GitHub releases API and returns the latest release info.
// Returns an error if the request fails, times out, or returns a non-200 status.
func CheckLatestRelease(ctx context.Context) (ReleaseInfo, error) {
	url := githubAPIBase + "/repos/m00nk0d3/nexus/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ReleaseInfo{}, fmt.Errorf("update check: build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "nexus-updater")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ReleaseInfo{}, fmt.Errorf("update check: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ReleaseInfo{}, fmt.Errorf("update check: unexpected status %d", resp.StatusCode)
	}

	var info ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return ReleaseInfo{}, fmt.Errorf("update check: decode response: %w", err)
	}
	return info, nil
}

// IsNewer reports whether latest is a higher semantic version than current.
// Both versions may optionally start with "v". Returns false (not an error) for
// "dev" or other non-semver current versions so that dev builds don't pop the modal.
func IsNewer(latest, current string) (bool, error) {
	if current == "dev" {
		return false, nil
	}

	latestParts, err := parseSemver(latest)
	if err != nil {
		return false, fmt.Errorf("IsNewer: parse latest %q: %w", latest, err)
	}

	currentParts, err := parseSemver(current)
	if err != nil {
		// Non-semver current version (e.g. dirty build) — treat as non-fatal.
		return false, nil
	}

	for i := range latestParts {
		if latestParts[i] > currentParts[i] {
			return true, nil
		}
		if latestParts[i] < currentParts[i] {
			return false, nil
		}
	}
	return false, nil
}

// parseSemver strips an optional "v" prefix and pre-release suffix
// (e.g. "v1.2.3-rc.1" → "1.2.3") and returns the [major, minor, patch]
// integer components. Returns an error if the string is not a valid semver.
func parseSemver(v string) ([3]int, error) {
	v = strings.TrimPrefix(v, "v")
	// Drop pre-release / build-metadata suffixes so "v1.2.3-rc.1" parses cleanly.
	if idx := strings.IndexByte(v, '-'); idx != -1 {
		v = v[:idx]
	}
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return [3]int{}, fmt.Errorf("not semver: %q", v)
	}
	var nums [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}, fmt.Errorf("not semver component %q: %w", p, err)
		}
		nums[i] = n
	}
	return nums, nil
}

// DownloadURL builds the GitHub release download URL for the running OS/arch.
// tagName must be the raw tag_name field from the API (e.g. "v1.2.3").
func DownloadURL(tagName string) string {
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	// GoReleaser's name_template uses {{ .Version }} which strips the "v" prefix,
	// so the archive filename is e.g. "nexus_1.2.3_linux_amd64.tar.gz" while the
	// release directory (tag) path still uses the full "v1.2.3".
	version := strings.TrimPrefix(tagName, "v")
	filename := fmt.Sprintf("nexus_%s_%s_%s.%s", version, runtime.GOOS, runtime.GOARCH, ext)
	return fmt.Sprintf("%s/%s/%s", githubReleaseBase, tagName, filename)
}

// SelfUpdate downloads the release archive for the current platform, extracts the
// nexus binary from it, and replaces the running executable. Progress is logged
// at debug level. Returns an error if any step fails.
func SelfUpdate(ctx context.Context, tagName string) error {
	currentExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("self-update: get executable path: %w", err)
	}

	url := DownloadURL(tagName)
	slog.Debug("self-update", "status", "Downloading "+url+"...")

	archiveFile, err := os.CreateTemp(os.TempDir(), "nexus-update-archive-*")
	if err != nil {
		return fmt.Errorf("self-update: create temp archive: %w", err)
	}
	archivePath := archiveFile.Name()
	defer os.Remove(archivePath)

	if err := downloadToFile(ctx, url, archiveFile); err != nil {
		archiveFile.Close()
		return fmt.Errorf("self-update: download %s: %w", url, err)
	}
	archiveFile.Close()

	slog.Debug("self-update", "status", "Verifying checksum...")

	archiveFilename := filepath.Base(url)
	expectedHash, err := fetchChecksum(ctx, tagName, archiveFilename)
	if err != nil {
		return fmt.Errorf("self-update: %w", err)
	}
	if err := verifyChecksum(archivePath, expectedHash); err != nil {
		return fmt.Errorf("self-update: %w", err)
	}

	slog.Debug("self-update", "status", "Extracting...")

	binaryFile, err := os.CreateTemp(os.TempDir(), "nexus-update-binary-*")
	if err != nil {
		return fmt.Errorf("self-update: create temp binary: %w", err)
	}
	binaryPath := binaryFile.Name()
	binaryFile.Close()
	defer os.Remove(binaryPath)

	binaryName := "nexus"
	if runtime.GOOS == "windows" {
		binaryName = "nexus.exe"
	}

	if runtime.GOOS == "windows" {
		if err := extractFromZip(archivePath, binaryName, binaryPath); err != nil {
			return fmt.Errorf("self-update: extract: %w", err)
		}
	} else {
		if err := extractFromTarGz(archivePath, binaryName, binaryPath); err != nil {
			return fmt.Errorf("self-update: extract: %w", err)
		}
	}

	slog.Debug("self-update", "status", "Replacing binary...")

	if err := os.Chmod(binaryPath, 0755); err != nil && runtime.GOOS != "windows" {
		return fmt.Errorf("self-update: chmod: %w", err)
	}

	if err := replaceBinary(binaryPath, currentExe); err != nil {
		return fmt.Errorf("self-update: %w", err)
	}

	slog.Debug("self-update", "status", "Done! Please restart nexus.")
	return nil
}

// downloadToFile performs an HTTP GET with the given context and writes the
// response body into f, capped at maxDownloadBytes. Returns an error on
// non-200 status or I/O failure.
func downloadToFile(ctx context.Context, url string, f *os.File) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	_, err = io.Copy(f, io.LimitReader(resp.Body, maxDownloadBytes))
	return err
}

// extractFromTarGz opens a .tar.gz archive at archivePath, finds the entry whose
// base name matches binaryName, and writes it to destPath.
func extractFromTarGz(archivePath, binaryName, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}
		if filepath.Base(hdr.Name) != binaryName {
			continue
		}
		out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
		if err != nil {
			return fmt.Errorf("open dest: %w", err)
		}
		_, copyErr := io.Copy(out, io.LimitReader(tr, maxDownloadBytes))
		out.Close()
		return copyErr
	}
	return fmt.Errorf("binary %q not found in archive", binaryName)
}

// extractFromZip opens a .zip archive at archivePath, finds the entry whose
// base name matches binaryName, and writes it to destPath.
func extractFromZip(archivePath, binaryName, destPath string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, f := range zr.File {
		if filepath.Base(f.Name) != binaryName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open zip entry: %w", err)
		}
		out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
		if err != nil {
			rc.Close()
			return fmt.Errorf("open dest: %w", err)
		}
		_, copyErr := io.Copy(out, io.LimitReader(rc, maxDownloadBytes))
		out.Close()
		rc.Close()
		return copyErr
	}
	return fmt.Errorf("binary %q not found in archive", binaryName)
}

// fetchChecksum downloads the GoReleaser-generated checksums.txt for the given
// release tag and returns the expected SHA-256 hex digest for archiveFilename.
func fetchChecksum(ctx context.Context, tagName, archiveFilename string) (string, error) {
	url := githubReleaseBase + "/" + tagName + "/checksums.txt"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("fetch checksum: build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch checksum: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch checksum: unexpected status %d", resp.StatusCode)
	}

	// GoReleaser format: "<sha256hex>  <filename>\n"
	scanner := bufio.NewScanner(io.LimitReader(resp.Body, 64*1024))
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "  ", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[1]) == archiveFilename {
			return strings.TrimSpace(parts[0]), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("fetch checksum: read body: %w", err)
	}
	return "", fmt.Errorf("fetch checksum: no entry for %q in checksums.txt", archiveFilename)
}

// verifyChecksum computes the SHA-256 digest of the file at path and compares
// it against expectedHex. Returns an error if they do not match.
func verifyChecksum(path, expectedHex string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash: %w", err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != expectedHex {
		return fmt.Errorf("checksum mismatch: got %s, want %s", got, expectedHex)
	}
	return nil
}

// moveFile moves src to dst, falling back to a copy+delete when os.Rename
// fails (e.g. across drive letters on Windows).
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Fallback: copy bytes then remove the source.
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer srcFile.Close()

	info, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("stat src: %w", err)
	}

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return fmt.Errorf("create dst: %w", err)
	}
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		dstFile.Close()
		os.Remove(dst)
		return fmt.Errorf("copy: %w", err)
	}
	if err := dstFile.Close(); err != nil {
		os.Remove(dst)
		return fmt.Errorf("close dst: %w", err)
	}
	_ = os.Remove(src)
	return nil
}

// replaceBinary replaces currentExe with the file at newPath.
//
// The strategy used on all platforms is:
//  1. Rename currentExe → currentExe+".old"  (the kernel holds the open inode
//     by reference, so the running process is unaffected by the rename)
//  2. Move   newPath    → currentExe          (target slot is now free)
//
// This avoids ETXTBSY on Linux/macOS (writing to a running executable is
// forbidden) and the equivalent restriction on Windows. Step 2 uses moveFile
// which falls back to copy+delete when newPath and currentExe are on different
// filesystems (e.g. /tmp vs /usr/local/bin, or C: vs D: on Windows).
//
// The ".old" file is cleaned up on the next startup by CleanupOldBinary.
func replaceBinary(newPath, currentExe string) error {
	oldExe := currentExe + ".old"
	// Remove any leftover .old from a previous update so the rename below
	// doesn't fail if the file already exists.
	_ = os.Remove(oldExe)

	if err := os.Rename(currentExe, oldExe); err != nil {
		return fmt.Errorf("rename current binary: %w", err)
	}
	if err := moveFile(newPath, currentExe); err != nil {
		// Best-effort rollback: put the original back.
		_ = os.Rename(oldExe, currentExe)
		return fmt.Errorf("replace binary: %w", err)
	}
	// oldExe will be cleaned up on the next startup by CleanupOldBinary.
	return nil
}

// CleanupOldBinary removes the "<executable>.old" file left behind by a
// self-update. It silently ignores any errors (the file may not exist or may
// still be locked by the OS).
func CleanupOldBinary() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	_ = os.Remove(exe + ".old")
}
