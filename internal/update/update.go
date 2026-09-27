// Package update implements the trust and download half of NFCX in-app updates.
// It deliberately has no dependency on Wails or NFC hardware.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/BennyThink/NFCX/internal/buildinfo"
)

const maxFeedSize = 128 << 10

var (
	ErrNotConfigured = errors.New("updates are not configured for this build")
	ErrUntrustedFeed = errors.New("update feed is not trusted")
	ErrNoAsset       = errors.New("no update asset matches this platform")
)

type State string

const (
	StateIdle        State = "idle"
	StateChecking    State = "checking"
	StateAvailable   State = "available"
	StateDownloading State = "downloading"
	StateReady       State = "ready"
	StateCurrent     State = "current"
	StateUnavailable State = "unavailable"
	StateFailed      State = "failed"
)

type Asset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Format string `json:"format,omitempty"`
}

type Feed struct {
	Schema      int               `json:"schema"`
	Channel     string            `json:"channel"`
	Version     string            `json:"version"`
	PublishedAt string            `json:"published_at"`
	Notes       map[string]string `json:"notes"`
	Assets      map[string]Asset  `json:"assets"`
}

type Snapshot struct {
	State          State  `json:"state"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion,omitempty"`
	Notes          string `json:"notes,omitempty"`
	Downloaded     int64  `json:"downloaded,omitempty"`
	Total          int64  `json:"total,omitempty"`
	AutoDownload   bool   `json:"autoDownload"`
	CanApply       bool   `json:"canApply"`
	Detail         string `json:"detail,omitempty"`
	PreparedPath   string `json:"-"`
	Asset          Asset  `json:"-"`
	CheckedAt      string `json:"checkedAt,omitempty"`
}

type Config struct {
	GitHubRepo     string // owner/repository; the normal NFCX update source
	GitHubAPIURL   string // test seam; production uses api.github.com
	ConfigPath     string
	StagingDir     string
	AllowedHosts   []string
	HTTPClient     *http.Client
	Platform       string
	CurrentVersion string
}

type Manager struct {
	mu       sync.Mutex
	config   Config
	snapshot Snapshot
}

func NewManager(config Config) (*Manager, error) {
	if config.Platform == "" {
		config.Platform = platformKey()
	}
	if config.GitHubRepo == "" {
		config.GitHubRepo = "BennyThink/NFCX"
	}
	if config.GitHubAPIURL == "" {
		config.GitHubAPIURL = "https://api.github.com"
	}
	if config.CurrentVersion == "" {
		config.CurrentVersion = buildinfo.Current().Version
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 20 * time.Second}
	}
	initial := Snapshot{State: StateIdle, CurrentVersion: cleanVersion(config.CurrentVersion)}
	m := &Manager{config: config, snapshot: initial}
	_ = m.load()
	return m, nil
}

func (m *Manager) Snapshot() Snapshot { m.mu.Lock(); defer m.mu.Unlock(); return m.snapshot }

func (m *Manager) SetAutoDownload(enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.snapshot.AutoDownload = enabled
	return m.saveLocked()
}

// CheckIfDue preserves the quiet-startup policy. A manual check should call
// Check directly; automatic checks happen no more than once per day.
func (m *Manager) CheckIfDue(ctx context.Context) (Snapshot, bool, error) {
	m.mu.Lock()
	checkedAt := m.snapshot.CheckedAt
	configured := m.config.GitHubRepo != ""
	m.mu.Unlock()
	if !configured {
		return m.Snapshot(), false, ErrNotConfigured
	}
	if parsed, err := time.Parse(time.RFC3339, checkedAt); err == nil && time.Since(parsed) < 24*time.Hour {
		return m.Snapshot(), false, nil
	}
	snapshot, err := m.Check(ctx)
	if err == nil && snapshot.AutoDownload && snapshot.State == StateAvailable {
		snapshot, err = m.Download(ctx)
	}
	return snapshot, true, err
}

// Check obtains and verifies the stable feed. It never downloads an asset.
func (m *Manager) Check(ctx context.Context) (Snapshot, error) {
	m.mu.Lock()
	m.snapshot.State, m.snapshot.Detail = StateChecking, ""
	m.snapshot.CheckedAt = time.Now().UTC().Format(time.RFC3339)
	m.mu.Unlock()

	feed, err := m.fetch(ctx)
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.snapshot.State, m.snapshot.Detail = StateUnavailable, safeDetail(err)
		_ = m.saveLocked()
		return m.snapshot, err
	}
	current, err := parseVersion(m.snapshot.CurrentVersion)
	if err != nil {
		m.snapshot.State, m.snapshot.Detail = StateUnavailable, "此构建版本不能安全比较更新"
		_ = m.saveLocked()
		return m.snapshot, err
	}
	latest, err := parseVersion(feed.Version)
	if err != nil || latest.pre != "" {
		m.snapshot.State, m.snapshot.Detail = StateUnavailable, "更新 feed 包含无效的稳定版本"
		_ = m.saveLocked()
		return m.snapshot, ErrUntrustedFeed
	}
	if latest.compare(current) <= 0 {
		m.snapshot.State, m.snapshot.LatestVersion, m.snapshot.Notes, m.snapshot.Detail = StateCurrent, feed.Version, "", ""
		_ = m.saveLocked()
		return m.snapshot, nil
	}
	asset, ok := feed.Assets[m.config.Platform]
	if !ok || !validAsset(asset, m.config.AllowedHosts) {
		m.snapshot.State, m.snapshot.LatestVersion, m.snapshot.Notes, m.snapshot.Detail = StateUnavailable, feed.Version, localNote(feed.Notes), "此平台没有受支持的更新包"
		_ = m.saveLocked()
		return m.snapshot, ErrNoAsset
	}
	m.snapshot.State, m.snapshot.LatestVersion, m.snapshot.Notes, m.snapshot.Asset, m.snapshot.Total, m.snapshot.Detail = StateAvailable, feed.Version, localNote(feed.Notes), asset, asset.Size, ""
	_ = m.saveLocked()
	return m.snapshot, nil
}

// Download streams the currently verified asset to private staging, checking
// its declared byte count and SHA-256 before it becomes prepared.
func (m *Manager) Download(ctx context.Context) (Snapshot, error) {
	m.mu.Lock()
	if m.snapshot.State != StateAvailable || m.snapshot.Asset.URL == "" {
		s := m.snapshot
		m.mu.Unlock()
		return s, errors.New("no verified update is available")
	}
	asset := m.snapshot.Asset
	m.snapshot.State, m.snapshot.Downloaded, m.snapshot.Detail = StateDownloading, 0, ""
	m.mu.Unlock()

	path, count, err := m.download(ctx, asset)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.snapshot.Downloaded = count
	if err != nil {
		m.snapshot.State, m.snapshot.Detail = StateAvailable, safeDetail(err)
		_ = m.saveLocked()
		return m.snapshot, err
	}
	m.snapshot.State, m.snapshot.PreparedPath, m.snapshot.CanApply, m.snapshot.Detail = StateReady, path, m.canApply(asset), ""
	_ = m.saveLocked()
	return m.snapshot, nil
}

func (m *Manager) fetch(ctx context.Context) (Feed, error) {
	return m.fetchGitHubRelease(ctx)
}

// fetchGitHubRelease is the normal NFCX source. GitHub returns a SHA-256
// digest for each release asset; we require it and verify the download again.
func (m *Manager) fetchGitHubRelease(ctx context.Context) (Feed, error) {
	if m.config.GitHubRepo == "" || strings.Count(m.config.GitHubRepo, "/") != 1 {
		return Feed{}, ErrNotConfigured
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(m.config.GitHubAPIURL, "/")+"/repos/"+m.config.GitHubRepo+"/releases/latest", nil)
	if err != nil {
		return Feed{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	response, err := m.config.HTTPClient.Do(req)
	if err != nil {
		return Feed{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Feed{}, fmt.Errorf("GitHub releases status %d", response.StatusCode)
	}
	var release struct {
		TagName    string `json:"tag_name"`
		Body       string `json:"body"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name   string `json:"name"`
			URL    string `json:"browser_download_url"`
			Digest string `json:"digest"`
			Size   int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxFeedSize)).Decode(&release); err != nil {
		return Feed{}, err
	}
	version := cleanVersion(release.TagName)
	if release.Draft || release.Prerelease {
		return Feed{}, ErrUntrustedFeed
	}
	assets := make(map[string]Asset)
	expected := releaseAssetName(version, m.config.Platform)
	for _, item := range release.Assets {
		if item.Name != expected || !strings.HasPrefix(item.Digest, "sha256:") {
			continue
		}
		assets[m.config.Platform] = Asset{URL: item.URL, SHA256: strings.TrimPrefix(item.Digest, "sha256:"), Size: item.Size, Format: assetFormat(item.Name)}
	}
	return Feed{Schema: 1, Channel: "stable", Version: version, Notes: map[string]string{"en": release.Body, "zh-CN": release.Body}, Assets: assets}, nil
}

func (m *Manager) download(ctx context.Context, asset Asset) (string, int64, error) {
	if !validAsset(asset, m.config.AllowedHosts) {
		return "", 0, ErrUntrustedFeed
	}
	if m.config.StagingDir == "" {
		return "", 0, ErrNotConfigured
	}
	if err := os.MkdirAll(m.config.StagingDir, 0700); err != nil {
		return "", 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return "", 0, err
	}
	response, err := m.config.HTTPClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("update download status %d", response.StatusCode)
	}
	temporary, err := os.CreateTemp(m.config.StagingDir, ".download-*")
	if err != nil {
		return "", 0, err
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()
	hash := sha256.New()
	count, copyErr := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(response.Body, asset.Size+1))
	closeErr := temporary.Close()
	if copyErr != nil {
		return "", count, copyErr
	}
	if closeErr != nil {
		return "", count, closeErr
	}
	if count != asset.Size || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), asset.SHA256) {
		return "", count, errors.New("download verification failed")
	}
	final := filepath.Join(m.config.StagingDir, "NFCX-"+cleanVersion(m.snapshot.LatestVersion)+"-"+m.config.Platform+filepath.Ext(urlPath(asset.URL)))
	if err := os.Rename(temporaryName, final); err != nil {
		return "", count, err
	}
	return final, count, nil
}

func (m *Manager) load() error {
	if m.config.ConfigPath == "" {
		return nil
	}
	data, err := os.ReadFile(m.config.ConfigPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var saved Snapshot
	if err := json.Unmarshal(data, &saved); err != nil {
		return err
	}
	if saved.CurrentVersion == m.snapshot.CurrentVersion {
		m.snapshot = saved
	}
	return nil
}
func (m *Manager) saveLocked() error {
	if m.config.ConfigPath == "" {
		return nil
	}
	data, err := json.MarshalIndent(m.snapshot, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.config.ConfigPath), 0700); err != nil {
		return err
	}
	tmp := m.config.ConfigPath + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, m.config.ConfigPath)
}

func platformKey() string {
	if runtime.GOOS == "darwin" {
		return "darwin-" + runtime.GOARCH
	}
	return runtime.GOOS + "-" + runtime.GOARCH
}
func urlPath(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Path
}
func localNote(notes map[string]string) string {
	if notes == nil {
		return ""
	}
	if value := notes["zh-CN"]; value != "" {
		return value
	}
	return notes["en"]
}
func validAsset(asset Asset, hosts []string) bool {
	parsed, err := url.Parse(asset.URL)
	if err != nil || parsed.Scheme != "https" || asset.Size < 1 || asset.Size > 2<<30 || len(asset.SHA256) != 64 {
		return false
	}
	if len(hosts) == 0 {
		return true
	}
	for _, host := range hosts {
		if strings.EqualFold(parsed.Hostname(), host) {
			return true
		}
	}
	return false
}
func (m *Manager) canApply(asset Asset) bool {
	if asset.Format != "appimage" && asset.Format != "zip" {
		return false
	}
	executable, err := os.Executable()
	if err != nil {
		return false
	}
	helper := filepath.Join(filepath.Dir(executable), "NFCX Updater")
	if runtime.GOOS == "windows" {
		helper += ".exe"
	}
	_, err = os.Stat(helper)
	return err == nil
}
func releaseAssetName(version, platform string) string {
	switch platform {
	case "darwin-arm64":
		return "NFCX-" + version + "-darwin-arm64.dmg"
	case "windows-amd64":
		return "NFCX-" + version + "-windows-amd64.zip"
	case "linux-amd64":
		return "NFCX-" + version + "-linux-amd64.AppImage"
	default:
		return ""
	}
}
func assetFormat(name string) string {
	switch {
	case strings.HasSuffix(name, ".AppImage"):
		return "appimage"
	case strings.HasSuffix(name, ".zip"):
		return "zip"
	case strings.HasSuffix(name, ".app.zip"):
		return "app"
	default:
		return "manual"
	}
}
func safeDetail(err error) string {
	if errors.Is(err, ErrNotConfigured) {
		return "更新源配置无效"
	}
	if errors.Is(err, ErrUntrustedFeed) {
		return "更新信息未通过安全验证"
	}
	if errors.Is(err, ErrNoAsset) {
		return "此平台没有可用更新包"
	}
	return "暂时无法检查或下载更新"
}

type version struct {
	major, minor, patch int
	pre                 string
}

func parseVersion(value string) (version, error) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	value = strings.SplitN(value, "+", 2)[0]
	var v version
	var base string
	base, v.pre, _ = strings.Cut(value, "-")
	if _, err := fmt.Sscanf(base, "%d.%d.%d", &v.major, &v.minor, &v.patch); err != nil || strings.Count(base, ".") != 2 {
		return version{}, errors.New("invalid semantic version")
	}
	return v, nil
}
func (v version) compare(other version) int {
	if v.major != other.major {
		if v.major < other.major {
			return -1
		}
		return 1
	}
	if v.minor != other.minor {
		if v.minor < other.minor {
			return -1
		}
		return 1
	}
	if v.patch != other.patch {
		if v.patch < other.patch {
			return -1
		}
		return 1
	}
	if v.pre == other.pre {
		return 0
	}
	if v.pre == "" {
		return 1
	}
	if other.pre == "" {
		return -1
	}
	if v.pre < other.pre {
		return -1
	}
	return 1
}
func cleanVersion(value string) string { return strings.TrimPrefix(strings.TrimSpace(value), "v") }
