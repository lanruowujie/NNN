package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestCheckAcceptsNewerGitHubRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v1.2.0", "body": "更安全的更新", "draft": false, "prerelease": false,
			"assets": []map[string]any{{"name": "NFCX-1.2.0-linux-amd64.AppImage", "browser_download_url": "https://" + r.Host + "/NFCX.AppImage", "digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "size": 10}},
		})
	}))
	defer server.Close()
	m, err := NewManager(Config{GitHubAPIURL: server.URL, GitHubRepo: "BennyThink/NFCX", CurrentVersion: "1.1.9", Platform: "linux-amd64"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateAvailable || got.LatestVersion != "1.2.0" || got.Notes != "更安全的更新" {
		t.Fatalf("snapshot = %+v", got)
	}
}

func TestCheckRejectsReleaseWithoutDigest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.0","assets":[{"name":"NFCX-1.2.0-linux-amd64.AppImage","browser_download_url":"https://github.com/a","size":1}]}`))
	}))
	defer server.Close()
	m, _ := NewManager(Config{GitHubAPIURL: server.URL, CurrentVersion: "1.1.9", Platform: "linux-amd64"})
	got, err := m.Check(context.Background())
	if err == nil || got.State != StateUnavailable {
		t.Fatalf("snapshot=%+v err=%v", got, err)
	}
}

func TestDownloadVerifiesHashAndPersistsReadyArtifact(t *testing.T) {
	payload := []byte("verified asset")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()
	m, _ := NewManager(Config{StagingDir: t.TempDir(), Platform: "linux-amd64", HTTPClient: server.Client()})
	m.snapshot = Snapshot{State: StateAvailable, CurrentVersion: "1.0.0", LatestVersion: "1.1.0", Asset: Asset{URL: server.URL + "/NFCX.AppImage", SHA256: sha256Hex(payload), Size: int64(len(payload)), Format: "appimage"}}
	got, err := m.Download(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateReady || got.CanApply || filepath.Ext(got.PreparedPath) != ".AppImage" {
		t.Fatalf("snapshot = %+v", got)
	}
}

func TestSemanticVersionOrdering(t *testing.T) {
	stable, _ := parseVersion("1.2.0")
	old, _ := parseVersion("1.1.99")
	pre, _ := parseVersion("1.2.0-rc.1")
	if stable.compare(old) <= 0 || stable.compare(pre) <= 0 {
		t.Fatal("incorrect version ordering")
	}
}

func sha256Hex(value []byte) string { hash := sha256.Sum256(value); return hex.EncodeToString(hash[:]) }
