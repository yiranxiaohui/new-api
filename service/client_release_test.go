package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testClientReleaseBody = `{
  "tag_name": "v0.2.4",
  "body": "notes",
  "html_url": "https://github.com/yiranxiaohui/Pier/releases/tag/v0.2.4",
  "published_at": "2026-09-28T10:00:00Z",
  "draft": false,
  "prerelease": false,
  "assets": [
    {"name": "latest.json", "size": 1, "browser_download_url": "%[1]s/latest.json"},
    {"name": "pier-mobile-v0.2.4-android.apk", "size": 40, "browser_download_url": "%[1]s/android.apk"},
    {"name": "pier-desktop-v0.2.4-linux-x64.deb", "size": 30, "browser_download_url": "%[1]s/linux.deb"},
    {"name": "pier-desktop-v0.2.4-linux-x64.deb.sig", "size": 1, "browser_download_url": "%[1]s/linux.deb.sig"},
    {"name": "pier-desktop-v0.2.4-darwin-arm64.app.tar.gz", "size": 1, "browser_download_url": "%[1]s/darwin.tar.gz"},
    {"name": "pier-desktop-v0.2.4-darwin-arm64.dmg", "size": 20, "browser_download_url": "%[1]s/darwin.dmg"},
    {"name": "pier-desktop-v0.2.3-windows-x64.setup.exe", "size": 1, "browser_download_url": "%[1]s/old.exe"},
    {"name": "pier-desktop-v0.2.4-windows-x64.setup.exe", "size": 10, "browser_download_url": "%[1]s/windows.exe"},
    {"name": "SHA256SUMS.txt", "size": 1, "browser_download_url": "%[1]s/SHA256SUMS.txt"}
  ]
}`

func newClientReleaseServer(t *testing.T, releaseBody string, releaseStatus *atomic.Int32, releaseHits *atomic.Int32) *httptest.Server {
	t.Helper()
	windowsSum := strings.Repeat("a", 64)
	apkSum := strings.Repeat("B", 64)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release":
			releaseHits.Add(1)
			if status := int(releaseStatus.Load()); status != http.StatusOK {
				w.WriteHeader(status)
				return
			}
			_, _ = fmt.Fprintf(w, releaseBody, server.URL)
		case "/SHA256SUMS.txt":
			_, _ = fmt.Fprintf(w, "%s  pier-desktop-v0.2.4-windows-x64.setup.exe\n%s *pier-mobile-v0.2.4-android.apk\nmalformed line\n", windowsSum, apkSum)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestFetchClientReleaseListsInstallersInDisplayOrder(t *testing.T) {
	var status, hits atomic.Int32
	status.Store(http.StatusOK)
	server := newClientReleaseServer(t, testClientReleaseBody, &status, &hits)

	release, err := fetchClientRelease(context.Background(), server.Client(), server.URL+"/release")
	require.NoError(t, err)

	assert.Equal(t, "v0.2.4", release.Version)
	assert.Equal(t, "notes", release.Notes)
	assert.Equal(t, "2026-09-28T10:00:00Z", release.PublishedAt)
	assert.Equal(t, []ClientDownloadAsset{
		{Name: "pier-desktop-v0.2.4-windows-x64.setup.exe", Platform: "windows", Arch: "x64", Format: "exe", Size: 10, URL: server.URL + "/windows.exe", SHA256: strings.Repeat("a", 64)},
		{Name: "pier-desktop-v0.2.4-darwin-arm64.dmg", Platform: "macos", Arch: "arm64", Format: "dmg", Size: 20, URL: server.URL + "/darwin.dmg"},
		{Name: "pier-desktop-v0.2.4-linux-x64.deb", Platform: "linux", Arch: "x64", Format: "deb", Size: 30, URL: server.URL + "/linux.deb"},
		{Name: "pier-mobile-v0.2.4-android.apk", Platform: "android", Arch: "universal", Format: "apk", Size: 40, URL: server.URL + "/android.apk", SHA256: strings.Repeat("b", 64)},
	}, release.Assets)
}

func TestFetchClientReleaseRejectsUnusableReleases(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "prerelease", body: strings.Replace(testClientReleaseBody, `"prerelease": false`, `"prerelease": true`, 1)},
		{name: "no installers", body: `{"tag_name": "v0.2.4", "assets": [{"name": "latest.json", "browser_download_url": "%[1]s/latest.json"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var status, hits atomic.Int32
			status.Store(http.StatusOK)
			server := newClientReleaseServer(t, tt.body, &status, &hits)

			_, err := fetchClientRelease(context.Background(), server.Client(), server.URL+"/release")
			require.Error(t, err)
		})
	}
}

func TestClientReleaseCacheServesStaleReleaseWhenGitHubFails(t *testing.T) {
	var status, hits atomic.Int32
	status.Store(http.StatusOK)
	server := newClientReleaseServer(t, testClientReleaseBody, &status, &hits)
	releaseURL := server.URL + "/release"
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	var cache clientReleaseCache

	first, err := cache.get(context.Background(), server.Client(), releaseURL, now)
	require.NoError(t, err)
	_, err = cache.get(context.Background(), server.Client(), releaseURL, now.Add(clientReleaseCacheTTL-time.Second))
	require.NoError(t, err)
	assert.EqualValues(t, 1, hits.Load(), "a fresh entry must not refetch")

	status.Store(http.StatusForbidden)
	stale, err := cache.get(context.Background(), server.Client(), releaseURL, now.Add(clientReleaseCacheTTL))
	require.NoError(t, err)
	assert.Same(t, first, stale)
	assert.EqualValues(t, 2, hits.Load())

	_, err = cache.get(context.Background(), server.Client(), releaseURL, now.Add(clientReleaseCacheTTL+clientReleaseRetryDelay-time.Second))
	require.NoError(t, err)
	assert.EqualValues(t, 2, hits.Load(), "a failed lookup waits for the retry delay")
}

func TestClientReleaseCacheRemembersFailureWithoutRelease(t *testing.T) {
	var status, hits atomic.Int32
	status.Store(http.StatusInternalServerError)
	server := newClientReleaseServer(t, testClientReleaseBody, &status, &hits)
	releaseURL := server.URL + "/release"
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	var cache clientReleaseCache

	_, err := cache.get(context.Background(), server.Client(), releaseURL, now)
	require.Error(t, err)
	status.Store(http.StatusOK)
	_, err = cache.get(context.Background(), server.Client(), releaseURL, now.Add(time.Second))
	require.Error(t, err)
	assert.EqualValues(t, 1, hits.Load())

	release, err := cache.get(context.Background(), server.Client(), releaseURL, now.Add(clientReleaseRetryDelay))
	require.NoError(t, err)
	assert.Equal(t, "v0.2.4", release.Version)
}

func TestClientDownloadReleaseWithMirror(t *testing.T) {
	const githubURL = "https://github.com/yiranxiaohui/Pier/releases/download/v0.2.4/pier.dmg"
	tests := []struct {
		name   string
		mirror string
		want   string
	}{
		{name: "prefix with slash", mirror: "https://gh-proxy.com/", want: "https://gh-proxy.com/" + githubURL},
		{name: "prefix without slash", mirror: " https://gh-proxy.com ", want: "https://gh-proxy.com/" + githubURL},
		{name: "empty", mirror: "", want: ""},
		{name: "unsupported scheme", mirror: "ftp://gh-proxy.com/", want: ""},
		{name: "query", mirror: "https://gh-proxy.com/?u=", want: ""},
		{name: "credentials", mirror: "https://user:pass@gh-proxy.com/", want: ""},
		{name: "relative", mirror: "gh-proxy.com", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			release := ClientDownloadRelease{Assets: []ClientDownloadAsset{{URL: githubURL}}}

			mirrored := release.WithMirror(tt.mirror)

			assert.Equal(t, tt.want, mirrored.Assets[0].MirrorURL)
			assert.Equal(t, githubURL, mirrored.Assets[0].URL)
			assert.Empty(t, release.Assets[0].MirrorURL, "the cached release must stay unchanged")
		})
	}
}
