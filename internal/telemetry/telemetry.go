// Package telemetry provides the deliberately small, allowlisted NFCX usage telemetry client.
package telemetry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/BennyThink/NFCX/internal/buildinfo"
)

const Endpoint = "https://telemetry.nfcx.tools/api/telemetry"

var allowedEvents = map[string]struct{}{
	"app_started": {}, "reader_scan_clicked": {}, "card_scan_clicked": {}, "read_clicked": {},
	"write_clicked": {}, "dump_clicked": {}, "key_recovery_clicked": {}, "settings_opened": {},
	"about_opened": {}, "update_check_clicked": {},
}

// Settings is all persistent state used by telemetry. InstallationID is a random identifier,
// never derived from a machine, user, reader, or card.
type Settings struct {
	Configured     bool   `json:"telemetryConfigured"`
	Enabled        bool   `json:"telemetryEnabled"`
	InstallationID string `json:"installationID,omitempty"`
}

type Store struct {
	mu       sync.Mutex
	path     string
	settings Settings
}

func NewStore(path string) *Store { return &Store{path: path} }

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.settings)
}

func (s *Store) Settings() Settings { s.mu.Lock(); defer s.mu.Unlock(); return s.settings }

func (s *Store) SetEnabled(enabled bool) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.settings
	next.Configured, next.Enabled = true, enabled
	if enabled && next.InstallationID == "" {
		id, err := randomUUIDv4()
		if err != nil {
			return Settings{}, err
		}
		next.InstallationID = id
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return Settings{}, err
	}
	if s.path == "" {
		s.settings = next
		return next, nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return Settings{}, err
	}
	temporary := s.path + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0600); err != nil {
		return Settings{}, err
	}
	if err := os.Rename(temporary, s.path); err != nil {
		return Settings{}, err
	}
	s.settings = next
	return next, nil
}

func randomUUIDv4() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	encoded := hex.EncodeToString(b)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}

type Client struct {
	store      *Store
	endpoint   string
	httpClient *http.Client
}

func NewClient(store *Store) *Client {
	return &Client{store: store, endpoint: Endpoint, httpClient: &http.Client{Timeout: 3 * time.Second}}
}

// Track ignores unsupported events and sends accepted events in a detached, best-effort goroutine.
func (c *Client) Track(event string) {
	if _, ok := allowedEvents[event]; !ok {
		return
	}
	settings := c.store.Settings()
	if !settings.Enabled || settings.InstallationID == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = c.send(ctx, settings, event)
	}()
}

func (c *Client) send(ctx context.Context, settings Settings, event string) error {
	payload, err := json.Marshal(struct {
		InstallationID string `json:"installation_id"`
		Timestamp      string `json:"timestamp"`
		Version        string `json:"version"`
		OS             string `json:"os"`
		Event          string `json:"event"`
	}{settings.InstallationID, time.Now().UTC().Format(time.RFC3339), buildinfo.Current().Version, coarseOS(), event})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("telemetry status %d", response.StatusCode)
	}
	return nil
}

func coarseOS() string {
	switch runtime.GOOS {
	case "windows", "darwin", "linux":
		if runtime.GOOS == "darwin" {
			return "macos"
		}
		return runtime.GOOS
	default:
		return "other"
	}
}
