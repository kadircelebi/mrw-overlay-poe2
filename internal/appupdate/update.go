package appupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultAPIURL    = "https://api.github.com/repos/kadircelebi/poe2-filtre/releases/latest"
	releaseAssetName = "poe2filtre-windows-amd64.exe"
	maxMetadata      = 1 << 20
	maxAssetSize     = 200 << 20
	checkInterval    = 24 * time.Hour
)

// State is the application-update status exposed to the panel.
type State struct {
	Status         string `json:"status"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion,omitempty"`
	ReleaseURL     string `json:"releaseUrl,omitempty"`
	Progress       int    `json:"progress,omitempty"`
	Error          string `json:"error,omitempty"`
	CanInstall     bool   `json:"canInstall"`
	CheckedAtMs    int64  `json:"checkedAtMs,omitempty"`
}

type githubAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type githubRelease struct {
	TagName    string        `json:"tag_name"`
	HTMLURL    string        `json:"html_url"`
	Draft      bool          `json:"draft"`
	Prerelease bool          `json:"prerelease"`
	Assets     []githubAsset `json:"assets"`
}

type cacheFile struct {
	CheckedAt       time.Time     `json:"checked_at"`
	Release         githubRelease `json:"release"`
	NotifiedVersion string        `json:"notified_version,omitempty"`
}

// Options configures a Manager. APIURL, Client and ExecutablePath are mainly
// injectable so the network and file handling can be tested without GitHub or
// the running executable.
type Options struct {
	CurrentVersion string
	DataDir        string
	APIURL         string
	Client         *http.Client
	ExecutablePath string
	Disabled       bool
	OnChange       func(State)
}

// Manager checks GitHub Releases, downloads the matching Windows executable
// and hands it to the staged updater process.
type Manager struct {
	mu         sync.Mutex
	current    string
	dataDir    string
	apiURL     string
	client     *http.Client
	executable string
	disabled   bool
	onChange   func(State)
	state      State
	fallback   State
	release    githubRelease
	asset      githubAsset
	cache      cacheFile
}

func New(opts Options) *Manager {
	apiURL := opts.APIURL
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Minute}
	}
	exe := opts.ExecutablePath
	if exe == "" {
		exe, _ = os.Executable()
	}
	m := &Manager{
		current:    normaliseVersion(opts.CurrentVersion),
		dataDir:    opts.DataDir,
		apiURL:     apiURL,
		client:     client,
		executable: exe,
		disabled:   opts.Disabled,
		onChange:   opts.OnChange,
	}
	m.state = State{Status: "idle", CurrentVersion: m.current}
	if opts.Disabled {
		m.state.Status = "disabled"
		return m
	}
	m.loadCache()
	return m
}

func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// CheckIfDue performs the automatic daily check. A cached result remains
// visible between launches, so skipping the HTTP request does not hide a known
// update from the panel.
func (m *Manager) CheckIfDue(ctx context.Context) (State, error) {
	m.mu.Lock()
	due := time.Since(m.cache.CheckedAt) >= checkInterval
	m.mu.Unlock()
	if !due {
		return m.State(), nil
	}
	return m.Check(ctx)
}

// Check fetches the latest stable GitHub release immediately.
func (m *Manager) Check(ctx context.Context) (State, error) {
	m.mu.Lock()
	if m.disabled {
		st := m.state
		m.mu.Unlock()
		return st, errors.New("application updates are disabled in test mode")
	}
	if m.state.Status == "checking" || m.state.Status == "downloading" || m.state.Status == "installing" {
		st := m.state
		m.mu.Unlock()
		return st, errors.New("an application update operation is already running")
	}
	m.fallback = State{}
	if m.state.Status == "available" || m.state.Status == "ready" {
		m.fallback = m.state
	}
	m.state.Status, m.state.Error, m.state.Progress = "checking", "", 0
	checking := m.state
	m.mu.Unlock()
	m.emit(checking)

	checkCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, m.apiURL, nil)
	if err == nil {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		req.Header.Set("User-Agent", "MrW-POE2-Filter/"+m.current)
	}
	if err != nil {
		return m.fail(err)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return m.fail(fmt.Errorf("GitHub release check failed: %w", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return m.fail(fmt.Errorf("GitHub release check returned HTTP %d", resp.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadata+1))
	if err != nil {
		return m.fail(fmt.Errorf("could not read GitHub release metadata: %w", err))
	}
	if len(data) > maxMetadata {
		return m.fail(errors.New("GitHub release metadata is unexpectedly large"))
	}
	var release githubRelease
	if err := json.Unmarshal(data, &release); err != nil {
		return m.fail(fmt.Errorf("could not decode GitHub release metadata: %w", err))
	}
	if release.Draft || release.Prerelease {
		return m.fail(errors.New("GitHub returned a non-stable release"))
	}
	latest := normaliseVersion(release.TagName)
	if _, ok := parseVersion(latest); !ok {
		return m.fail(fmt.Errorf("invalid release version %q", release.TagName))
	}

	asset := findReleaseAsset(release, latest)
	now := time.Now()
	m.mu.Lock()
	m.release, m.asset = release, asset
	m.cache.CheckedAt, m.cache.Release = now, release
	m.state = State{
		Status:         "up_to_date",
		CurrentVersion: m.current,
		LatestVersion:  latest,
		ReleaseURL:     release.HTMLURL,
		CheckedAtMs:    now.UnixMilli(),
	}
	if compareVersions(latest, m.current) > 0 {
		if asset.Name == "" {
			m.mu.Unlock()
			return m.fail(fmt.Errorf("release asset %q was not found", releaseAssetName))
		}
		if _, err := assetSHA256(asset); err != nil {
			m.mu.Unlock()
			return m.fail(err)
		}
		m.state.Status = "available"
		m.state.CanInstall = canReplace(m.executable)
		if path := m.downloadPathLocked(); validDownloaded(path, asset) {
			m.state.Status = "ready"
		}
	}
	st := m.state
	cache := m.cache
	m.fallback = State{}
	m.mu.Unlock()
	m.saveCache(cache)
	m.emit(st)
	return st, nil
}

// Download downloads the release asset to the app-data staging directory and
// verifies both its advertised size and GitHub's sha256 digest before exposing
// it as ready to install.
func (m *Manager) Download(ctx context.Context) (State, error) {
	m.mu.Lock()
	if m.state.Status != "available" && m.state.Status != "ready" {
		st := m.state
		m.mu.Unlock()
		return st, errors.New("no application update is available")
	}
	asset := m.asset
	finalPath := m.downloadPathLocked()
	m.fallback = m.state
	m.state.Status, m.state.Error, m.state.Progress = "downloading", "", 0
	st := m.state
	m.mu.Unlock()
	m.emit(st)

	if validDownloaded(finalPath, asset) {
		return m.ready()
	}
	if asset.Size <= 0 || asset.Size > maxAssetSize {
		return m.fail(fmt.Errorf("invalid release asset size: %d", asset.Size))
	}
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return m.fail(fmt.Errorf("could not create update folder: %w", err))
	}
	partPath := finalPath + ".part"
	_ = os.Remove(partPath)
	out, err := os.OpenFile(partPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return m.fail(fmt.Errorf("could not create update file: %w", err))
	}
	keep := false
	defer func() {
		_ = out.Close()
		if !keep {
			_ = os.Remove(partPath)
		}
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err == nil {
		req.Header.Set("Accept", "application/octet-stream")
		req.Header.Set("User-Agent", "MrW-POE2-Filter/"+m.current)
	}
	if err != nil {
		return m.fail(err)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return m.fail(fmt.Errorf("application update download failed: %w", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return m.fail(fmt.Errorf("application update download returned HTTP %d", resp.StatusCode))
	}

	hash := sha256.New()
	progress := &progressWriter{manager: m, total: asset.Size}
	written, err := io.Copy(io.MultiWriter(out, hash, progress), io.LimitReader(resp.Body, maxAssetSize+1))
	if err != nil {
		return m.fail(fmt.Errorf("could not save application update: %w", err))
	}
	if written != asset.Size {
		return m.fail(fmt.Errorf("application update size mismatch: got %d, want %d", written, asset.Size))
	}
	expected, err := assetSHA256(asset)
	if err != nil {
		return m.fail(err)
	}
	if actual := hex.EncodeToString(hash.Sum(nil)); actual != expected {
		return m.fail(errors.New("application update SHA-256 verification failed"))
	}
	if err := out.Sync(); err != nil {
		return m.fail(fmt.Errorf("could not flush application update: %w", err))
	}
	if err := out.Close(); err != nil {
		return m.fail(fmt.Errorf("could not close application update: %w", err))
	}
	_ = os.Remove(finalPath)
	if err := os.Rename(partPath, finalPath); err != nil {
		return m.fail(fmt.Errorf("could not finalise application update: %w", err))
	}
	keep = true
	return m.ready()
}

// LaunchInstaller starts the verified new executable in updater mode. The
// caller should quit the current application immediately after this returns.
func (m *Manager) LaunchInstaller() error {
	m.mu.Lock()
	if m.state.Status != "ready" {
		m.mu.Unlock()
		return errors.New("the application update is not ready")
	}
	staged, target, asset := m.downloadPathLocked(), m.executable, m.asset
	if !m.state.CanInstall {
		m.mu.Unlock()
		return errors.New("the application folder is not writable")
	}
	m.mu.Unlock()
	if !validDownloaded(staged, asset) {
		return errors.New("the verified application update is missing or changed")
	}
	cmd := exec.Command(staged, "-apply-update", target, "-wait-pid", strconv.Itoa(os.Getpid()), "-data", m.dataDir)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start the application updater: %w", err)
	}
	m.setStatus("installing", "", 100)
	return nil
}

// TakeNotification returns a version only once, even across launches. It is
// used to avoid repeating the same desktop notification every day.
func (m *Manager) TakeNotification() string {
	m.mu.Lock()
	if m.state.Status != "available" && m.state.Status != "ready" || m.state.LatestVersion == m.cache.NotifiedVersion {
		m.mu.Unlock()
		return ""
	}
	m.cache.NotifiedVersion = m.state.LatestVersion
	version := m.state.LatestVersion
	cache := m.cache
	m.mu.Unlock()
	m.saveCache(cache)
	return version
}

func (m *Manager) ready() (State, error) {
	m.mu.Lock()
	m.state.Status, m.state.Error, m.state.Progress = "ready", "", 100
	m.fallback = State{}
	st := m.state
	m.mu.Unlock()
	m.emit(st)
	return st, nil
}

func (m *Manager) fail(err error) (State, error) {
	m.mu.Lock()
	if m.fallback.Status == "available" || m.fallback.Status == "ready" {
		m.state = m.fallback
		m.state.Error = err.Error()
	} else {
		m.state.Status, m.state.Error, m.state.Progress = "error", err.Error(), 0
	}
	m.fallback = State{}
	st := m.state
	m.mu.Unlock()
	m.emit(st)
	return st, err
}

func (m *Manager) setStatus(status, message string, progress int) {
	m.mu.Lock()
	m.state.Status, m.state.Error, m.state.Progress = status, message, progress
	st := m.state
	m.mu.Unlock()
	m.emit(st)
}

func (m *Manager) emit(st State) {
	if m.onChange != nil {
		m.onChange(st)
	}
}

func (m *Manager) downloadPathLocked() string {
	return filepath.Join(m.dataDir, "updates", "v"+m.state.LatestVersion, m.asset.Name)
}

func (m *Manager) cachePath() string { return filepath.Join(m.dataDir, "update-state.json") }

func (m *Manager) loadCache() {
	data, err := os.ReadFile(m.cachePath())
	if err != nil || json.Unmarshal(data, &m.cache) != nil || m.cache.Release.TagName == "" {
		return
	}
	m.release = m.cache.Release
	latest := normaliseVersion(m.release.TagName)
	m.asset = findReleaseAsset(m.release, latest)
	m.state.LatestVersion = latest
	m.state.ReleaseURL = m.release.HTMLURL
	m.state.CheckedAtMs = m.cache.CheckedAt.UnixMilli()
	m.state.Status = "up_to_date"
	if compareVersions(latest, m.current) > 0 && m.asset.Name != "" {
		m.state.Status = "available"
		m.state.CanInstall = canReplace(m.executable)
		if validDownloaded(m.downloadPathLocked(), m.asset) {
			m.state.Status = "ready"
		}
	}
}

// findReleaseAsset prefers the stable filename used by current releases. The
// versioned form keeps cached metadata and transition releases compatible.
func findReleaseAsset(release githubRelease, version string) githubAsset {
	names := []string{releaseAssetName, "poe2filtre-v" + version + "-windows-amd64.exe"}
	for _, name := range names {
		for _, asset := range release.Assets {
			if asset.Name == name {
				return asset
			}
		}
	}
	return githubAsset{}
}

func (m *Manager) saveCache(cache cacheFile) {
	if err := os.MkdirAll(m.dataDir, 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return
	}
	tmp := m.cachePath() + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Remove(m.cachePath())
		_ = os.Rename(tmp, m.cachePath())
	}
}

type progressWriter struct {
	manager *Manager
	total   int64
	written int64
	last    int
}

func (p *progressWriter) Write(data []byte) (int, error) {
	p.written += int64(len(data))
	progress := int(p.written * 100 / p.total)
	if progress > 100 {
		progress = 100
	}
	if progress >= p.last+2 || progress == 100 {
		p.last = progress
		p.manager.setStatus("downloading", "", progress)
	}
	return len(data), nil
}

func assetSHA256(asset githubAsset) (string, error) {
	digest := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(asset.Digest)), "sha256:")
	if len(digest) != sha256.Size*2 {
		return "", errors.New("release asset has no valid SHA-256 digest")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return "", errors.New("release asset has no valid SHA-256 digest")
	}
	return digest, nil
}

func validDownloaded(path string, asset githubAsset) bool {
	if path == "" || asset.Name == "" || asset.Size <= 0 {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() != asset.Size {
		return false
	}
	expected, err := assetSHA256(asset)
	if err != nil {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return false
	}
	return hex.EncodeToString(hash.Sum(nil)) == expected
}

func canReplace(executable string) bool {
	if runtime.GOOS != "windows" || executable == "" {
		return false
	}
	dir := filepath.Dir(executable)
	f, err := os.CreateTemp(dir, ".poe2-update-write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

type semanticVersion [3]int

func normaliseVersion(version string) string {
	return strings.TrimPrefix(strings.TrimSpace(version), "v")
}

func parseVersion(version string) (semanticVersion, bool) {
	var out semanticVersion
	parts := strings.Split(normaliseVersion(version), ".")
	if len(parts) != len(out) {
		return out, false
	}
	for i, part := range parts {
		if part == "" || strings.ContainsAny(part, "+-") {
			return out, false
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func compareVersions(a, b string) int {
	av, aok := parseVersion(a)
	bv, bok := parseVersion(b)
	if !aok || !bok {
		return 0
	}
	for i := range av {
		if av[i] < bv[i] {
			return -1
		}
		if av[i] > bv[i] {
			return 1
		}
	}
	return 0
}
