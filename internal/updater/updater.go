package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// githubAPIBase is the base URL for GitHub API calls. It can be overridden in tests.
var githubAPIBase = "https://api.github.com"

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

// parseSemver strips an optional "v" prefix and returns the [major, minor, patch]
// integer components. Returns an error if the string is not a valid semver.
func parseSemver(v string) ([3]int, error) {
	v = strings.TrimPrefix(v, "v")
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
	filename := fmt.Sprintf("nexus_%s_%s_%s.%s", tagName, runtime.GOOS, runtime.GOARCH, ext)
	return fmt.Sprintf("https://github.com/m00nk0d3/nexus/releases/download/%s/%s", tagName, filename)
}

// SelfUpdate downloads the release archive for the current platform, extracts the
// nexus binary from it, and replaces the running executable. onProgress is called
// with human-readable status strings. Returns an error if any step fails.
func SelfUpdate(ctx context.Context, tagName string, onProgress func(string)) error {
	currentExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("self-update: get executable path: %w", err)
	}

	url := DownloadURL(tagName)
	onProgress("Downloading " + url + "...")

	archiveFile, err := os.CreateTemp(os.TempDir(), "nexus-update-archive-*")
	if err != nil {
		return fmt.Errorf("self-update: create temp archive: %w", err)
	}
	archivePath := archiveFile.Name()
	defer os.Remove(archivePath)

	if err := downloadToFile(ctx, url, archiveFile); err != nil {
		archiveFile.Close()
		return fmt.Errorf("self-update: download: %w", err)
	}
	archiveFile.Close()

	onProgress("Extracting...")

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

	onProgress("Replacing binary...")

	if err := os.Chmod(binaryPath, 0755); err != nil && runtime.GOOS != "windows" {
		return fmt.Errorf("self-update: chmod: %w", err)
	}

	if err := os.Rename(binaryPath, currentExe); err != nil {
		if runtime.GOOS == "windows" {
			return fmt.Errorf("self-update: cannot replace running binary on Windows; please download manually from %s", url)
		}
		return fmt.Errorf("self-update: replace binary: %w", err)
	}

	onProgress("Done! Please restart nexus.")
	return nil
}

// downloadToFile performs an HTTP GET with the given context and writes the
// response body into f. Returns an error on non-200 status or I/O failure.
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
	_, err = io.Copy(f, resp.Body)
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
		_, copyErr := io.Copy(out, tr)
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
		_, copyErr := io.Copy(out, rc)
		out.Close()
		rc.Close()
		return copyErr
	}
	return fmt.Errorf("binary %q not found in archive", binaryName)
}
