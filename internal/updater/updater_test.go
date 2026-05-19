package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsNewer(t *testing.T) {
	tests := []struct {
		name    string
		latest  string
		current string
		want    bool
		wantErr bool
	}{
		{name: "newer patch", latest: "v1.0.1", current: "v1.0.0", want: true},
		{name: "newer minor", latest: "v1.1.0", current: "v1.0.5", want: true},
		{name: "newer major", latest: "v2.0.0", current: "v1.9.9", want: true},
		{name: "same version", latest: "v1.2.3", current: "v1.2.3", want: false},
		{name: "older version", latest: "v1.0.0", current: "v1.2.3", want: false},
		{name: "dev build current", latest: "v1.2.3", current: "dev", want: false},
		{name: "invalid semver latest", latest: "not-a-version", current: "v1.0.0", wantErr: true},
		{name: "invalid semver current", latest: "v1.2.3", current: "custom-build", want: false},
		{name: "no v prefix", latest: "1.2.3", current: "1.0.0", want: true},
		{name: "pre-release latest", latest: "v1.2.3-rc.1", current: "v1.2.2", want: true},
		{name: "pre-release current", latest: "v1.2.3", current: "v1.2.3-rc.1", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsNewer(tt.latest, tt.current)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCheckLatestRelease_Success(t *testing.T) {
	want := ReleaseInfo{TagName: "v1.2.3", HTMLURL: "https://github.com/m00nk0d3/nexus/releases/tag/v1.2.3"}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	}))
	defer ts.Close()

	old := githubAPIBase
	githubAPIBase = ts.URL
	defer func() { githubAPIBase = old }()

	got, err := CheckLatestRelease(t.Context())
	require.NoError(t, err)
	assert.Equal(t, want.TagName, got.TagName)
	assert.Equal(t, want.HTMLURL, got.HTMLURL)
}

func TestCheckLatestRelease_NonOK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	old := githubAPIBase
	githubAPIBase = ts.URL
	defer func() { githubAPIBase = old }()

	_, err := CheckLatestRelease(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestCheckLatestRelease_InvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{invalid}`))
	}))
	defer ts.Close()

	old := githubAPIBase
	githubAPIBase = ts.URL
	defer func() { githubAPIBase = old }()

	_, err := CheckLatestRelease(t.Context())
	require.Error(t, err)
}

func TestDownloadURL(t *testing.T) {
	tag := "v1.2.3"

	old := githubReleaseBase
	githubReleaseBase = "https://example.com/releases"
	defer func() { githubReleaseBase = old }()

	url := DownloadURL(tag)

	assert.True(t, strings.HasPrefix(url, "https://example.com/releases"), "should use githubReleaseBase: %s", url)
	assert.Contains(t, url, tag)
	assert.Contains(t, url, runtime.GOOS)
	assert.Contains(t, url, runtime.GOARCH)

	if runtime.GOOS == "windows" {
		assert.True(t, strings.HasSuffix(url, ".zip"), "windows should use .zip: %s", url)
	} else {
		assert.True(t, strings.HasSuffix(url, ".tar.gz"), "non-windows should use .tar.gz: %s", url)
	}
}

func TestFetchChecksum_Success(t *testing.T) {
	const filename = "nexus_v1.2.3_linux_amd64.tar.gz"
	const wantHash = "abc123def456"
	body := wantHash + "  " + filename + "\ndeadbeef  other_file.tar.gz\n"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "checksums.txt")
		fmt.Fprint(w, body)
	}))
	defer ts.Close()

	old := githubReleaseBase
	githubReleaseBase = ts.URL
	defer func() { githubReleaseBase = old }()

	got, err := fetchChecksum(t.Context(), "v1.2.3", filename)
	require.NoError(t, err)
	assert.Equal(t, wantHash, got)
}

func TestFetchChecksum_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	old := githubReleaseBase
	githubReleaseBase = ts.URL
	defer func() { githubReleaseBase = old }()

	_, err := fetchChecksum(t.Context(), "v1.2.3", "nexus_v1.2.3_linux_amd64.tar.gz")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestFetchChecksum_MissingEntry(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "abc123  some_other_file.tar.gz\n")
	}))
	defer ts.Close()

	old := githubReleaseBase
	githubReleaseBase = ts.URL
	defer func() { githubReleaseBase = old }()

	_, err := fetchChecksum(t.Context(), "v1.2.3", "nexus_v1.2.3_linux_amd64.tar.gz")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no entry for")
}

func TestVerifyChecksum_Valid(t *testing.T) {
	content := []byte("hello nexus")
	sum := sha256.Sum256(content)
	expectedHex := hex.EncodeToString(sum[:])

	f, err := os.CreateTemp(t.TempDir(), "nexus-test-*")
	require.NoError(t, err)
	_, err = f.Write(content)
	require.NoError(t, err)
	f.Close()

	require.NoError(t, verifyChecksum(f.Name(), expectedHex))
}

func TestVerifyChecksum_Mismatch(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "nexus-test-*")
	require.NoError(t, err)
	_, err = f.Write([]byte("some content"))
	require.NoError(t, err)
	f.Close()

	err = verifyChecksum(f.Name(), "0000000000000000000000000000000000000000000000000000000000000000")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")
}
