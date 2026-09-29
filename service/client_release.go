package service

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	pierLatestReleaseURL       = "https://api.github.com/repos/yiranxiaohui/Pier/releases/latest"
	clientReleaseCacheTTL      = 30 * time.Minute
	clientReleaseRetryDelay    = 2 * time.Minute
	clientReleaseFetchTimeout  = 15 * time.Second
	clientReleaseChecksumsName = "SHA256SUMS.txt"
	clientReleaseChecksumsMax  = 64 << 10
)

// clientInstallerTargets lists the installers offered for download, in display
// order. Release assets are named `pier-<desktop|mobile>-<tag>-<suffix>`; update
// signatures, updater archives, and manifests are intentionally absent.
var clientInstallerTargets = []struct {
	suffix   string
	platform string
	arch     string
	format   string
}{
	{"windows-x64.setup.exe", "windows", "x64", "exe"},
	{"windows-arm64.setup.exe", "windows", "arm64", "exe"},
	{"darwin-arm64.dmg", "macos", "arm64", "dmg"},
	{"darwin-x64.dmg", "macos", "x64", "dmg"},
	{"linux-x64.AppImage", "linux", "x64", "appimage"},
	{"linux-x64.deb", "linux", "x64", "deb"},
	{"linux-arm64.AppImage", "linux", "arm64", "appimage"},
	{"linux-arm64.deb", "linux", "arm64", "deb"},
	{"android.apk", "android", "universal", "apk"},
}

// ClientDownloadAsset is one installer of a client release.
type ClientDownloadAsset struct {
	Name      string `json:"name"`
	Platform  string `json:"platform"`
	Arch      string `json:"arch"`
	Format    string `json:"format"`
	Size      int64  `json:"size"`
	URL       string `json:"url"`
	MirrorURL string `json:"mirror_url,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
}

// ClientDownloadRelease is the latest stable client release with its installers.
type ClientDownloadRelease struct {
	Version     string                `json:"version"`
	PublishedAt string                `json:"published_at"`
	Notes       string                `json:"notes"`
	ReleaseURL  string                `json:"release_url"`
	Assets      []ClientDownloadAsset `json:"assets"`
}

// WithMirror returns a copy whose installers also carry a download URL through
// the GitHub acceleration prefix `mirror` (for example `https://gh-proxy.com/`).
// An empty or malformed prefix leaves only the GitHub URLs.
func (release ClientDownloadRelease) WithMirror(mirror string) ClientDownloadRelease {
	release.Assets = append([]ClientDownloadAsset(nil), release.Assets...)
	mirror = strings.TrimSpace(mirror)
	if mirror == "" {
		return release
	}
	parsed, err := url.Parse(mirror)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return release
	}
	if !strings.HasSuffix(mirror, "/") {
		mirror += "/"
	}
	for i := range release.Assets {
		release.Assets[i].MirrorURL = mirror + release.Assets[i].URL
	}
	return release
}

type clientReleaseCache struct {
	sync.Mutex
	release   *ClientDownloadRelease
	err       error
	expiresAt time.Time
}

var pierReleaseCache clientReleaseCache

// GetPierRelease returns the latest stable Pier release from GitHub. Lookups are
// cached; when GitHub is unavailable the last successful result keeps serving.
func GetPierRelease(ctx context.Context) (ClientDownloadRelease, error) {
	release, err := pierReleaseCache.get(ctx, GetHttpClient(), pierLatestReleaseURL, time.Now())
	if err != nil {
		return ClientDownloadRelease{}, err
	}
	return *release, nil
}

func (cache *clientReleaseCache) get(ctx context.Context, client *http.Client, releaseURL string, now time.Time) (*ClientDownloadRelease, error) {
	cache.Lock()
	defer cache.Unlock()

	if now.Before(cache.expiresAt) {
		if cache.release != nil {
			return cache.release, nil
		}
		return nil, cache.err
	}

	// A canceled visitor request must not discard a lookup other visitors wait on.
	fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), clientReleaseFetchTimeout)
	defer cancel()
	release, err := fetchClientRelease(fetchCtx, client, releaseURL)
	if err != nil {
		cache.err = err
		cache.expiresAt = now.Add(clientReleaseRetryDelay)
		if cache.release != nil {
			return cache.release, nil
		}
		return nil, err
	}

	cache.release = release
	cache.err = nil
	cache.expiresAt = now.Add(clientReleaseCacheTTL)
	return release, nil
}

func fetchClientRelease(ctx context.Context, client *http.Client, releaseURL string) (*ClientDownloadRelease, error) {
	if client == nil {
		return nil, fmt.Errorf("nil http client")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "new-api")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("client release lookup failed: status=%d", resp.StatusCode)
	}

	var payload struct {
		TagName     string `json:"tag_name"`
		Body        string `json:"body"`
		HTMLURL     string `json:"html_url"`
		PublishedAt string `json:"published_at"`
		Draft       bool   `json:"draft"`
		Prerelease  bool   `json:"prerelease"`
		Assets      []struct {
			Name               string `json:"name"`
			Size               int64  `json:"size"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := common.DecodeJson(resp.Body, &payload); err != nil {
		return nil, err
	}
	tag := strings.TrimSpace(payload.TagName)
	if payload.Draft || payload.Prerelease || tag == "" {
		return nil, fmt.Errorf("latest client release is not a stable tagged release")
	}

	release := &ClientDownloadRelease{
		Version:     tag,
		PublishedAt: payload.PublishedAt,
		Notes:       payload.Body,
		ReleaseURL:  payload.HTMLURL,
	}
	type releaseAsset struct {
		size int64
		url  string
	}
	assets := make(map[string]releaseAsset, len(payload.Assets))
	for _, asset := range payload.Assets {
		assets[asset.Name] = releaseAsset{size: asset.Size, url: asset.BrowserDownloadURL}
	}
	for _, target := range clientInstallerTargets {
		for _, name := range []string{"pier-desktop-" + tag + "-" + target.suffix, "pier-mobile-" + tag + "-" + target.suffix} {
			asset, ok := assets[name]
			if !ok || asset.url == "" {
				continue
			}
			release.Assets = append(release.Assets, ClientDownloadAsset{
				Name:     name,
				Platform: target.platform,
				Arch:     target.arch,
				Format:   target.format,
				Size:     asset.size,
				URL:      asset.url,
			})
		}
	}
	if len(release.Assets) == 0 {
		return nil, fmt.Errorf("client release %s has no installers", tag)
	}

	// Checksums are informational; a missing or unreadable manifest keeps the release usable.
	checksumsAsset, ok := assets[clientReleaseChecksumsName]
	if !ok || checksumsAsset.url == "" {
		return release, nil
	}
	checksums, err := fetchReleaseChecksums(ctx, client, checksumsAsset.url)
	if err != nil {
		common.SysLog(fmt.Sprintf("client release %s checksums unavailable: %v", tag, err))
		return release, nil
	}
	for i := range release.Assets {
		release.Assets[i].SHA256 = checksums[release.Assets[i].Name]
	}
	return release, nil
}

// fetchReleaseChecksums reads a `sha256sum` manifest into file name → hex digest.
func fetchReleaseChecksums(ctx context.Context, client *http.Client, checksumsURL string) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "new-api")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status=%d", resp.StatusCode)
	}

	checksums := make(map[string]string)
	scanner := bufio.NewScanner(io.LimitReader(resp.Body, clientReleaseChecksumsMax))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || len(fields[0]) != 64 {
			continue
		}
		// sha256sum marks binary mode with a leading `*` on the file name.
		checksums[strings.TrimPrefix(fields[1], "*")] = strings.ToLower(fields[0])
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return checksums, nil
}
