package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckAvailable_findsUpdate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/repos/svpchain/svpchain-agent/releases/latest", r.URL.Path)
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName: "v1.0.2",
			HTMLURL: "https://github.com/svpchain/svpchain-agent/releases/tag/v1.0.2",
			Assets: []releaseAsset{
				{Name: "svpchain-agent-1.0.2-macos.dmg", BrowserDownloadURL: "https://example.com/app.dmg"},
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
	require.Equal(t, "svpchain-agent-1.0.2-macos.dmg", info.DmgName)
}

func TestCheckAvailable_respectsSkipVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName: "v1.0.2",
			Assets: []releaseAsset{
				{Name: "svpchain-agent-1.0.2-macos.dmg", BrowserDownloadURL: "https://example.com/app.dmg"},
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
