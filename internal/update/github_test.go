package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckAvailable_findsUpdate(t *testing.T) {
	var wantPackage string
	switch runtime.GOOS {
	case "darwin":
		wantPackage = "svpchain-agent-1.0.2-macos.dmg"
	case "windows":
		wantPackage = "svpchain-agent-1.0.2-windows-amd64.zip"
	default:
		t.Skip("in-app update tests require darwin or windows")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/repos/svpchain/svpchain-agent/releases/latest", r.URL.Path)
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName: "v1.0.2",
			HTMLURL: "https://github.com/svpchain/svpchain-agent/releases/tag/v1.0.2",
			Assets: []releaseAsset{
				{Name: "svpchain-agent-1.0.2-macos.dmg", BrowserDownloadURL: "https://example.com/app.dmg"},
				{Name: "svpchain-agent-1.0.2-windows-amd64.zip", BrowserDownloadURL: "https://example.com/app.zip"},
				{Name: "SHA256SUMS", BrowserDownloadURL: "https://example.com/SHA256SUMS"},
			},
		})
	}))
	t.Cleanup(srv.Close)

	oldURL := latestReleaseURL
	latestReleaseURL = srv.URL + "/repos/svpchain/svpchain-agent/releases/latest"
	t.Cleanup(func() { latestReleaseURL = oldURL })

	info, err := checkAvailable(t.Context(), "1.0.1", "", srv.Client())
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, "1.0.2", info.Latest)
	require.Equal(t, wantPackage, info.PackageName)
}

func TestCheckAvailable_respectsSkipVersion(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("in-app update tests require darwin or windows")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName: "v1.0.2",
			Assets: []releaseAsset{
				{Name: "svpchain-agent-1.0.2-macos.dmg", BrowserDownloadURL: "https://example.com/app.dmg"},
				{Name: "svpchain-agent-1.0.2-windows-amd64.zip", BrowserDownloadURL: "https://example.com/app.zip"},
				{Name: "SHA256SUMS", BrowserDownloadURL: "https://example.com/SHA256SUMS"},
			},
		})
	}))
	t.Cleanup(srv.Close)

	oldURL := latestReleaseURL
	latestReleaseURL = srv.URL + "/repos/svpchain/svpchain-agent/releases/latest"
	t.Cleanup(func() { latestReleaseURL = oldURL })

	info, err := checkAvailable(t.Context(), "1.0.0", "v1.0.2", srv.Client())
	require.NoError(t, err)
	require.Nil(t, info)
}

func TestWindowsZipAsset(t *testing.T) {
	name, url, err := windowsZipAsset(&githubRelease{
		TagName: "v1.0.2",
		Assets: []releaseAsset{
			{Name: "svpchain-agent-1.0.2-windows-amd64.zip", BrowserDownloadURL: "https://example.com/app.zip"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "svpchain-agent-1.0.2-windows-amd64.zip", name)
	require.Equal(t, "https://example.com/app.zip", url)
}
