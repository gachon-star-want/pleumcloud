package app_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pleumcloud/pleumcloud/internal/app"
	"github.com/pleumcloud/pleumcloud/internal/config"
)

// localCfg builds a fresh loopback-only config over a temp data dir.
// Port 0 is intentional: Start must report the actually-bound port.
func localCfg(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		DataDir: t.TempDir(),
		Port:    0,
		Bind:    "127.0.0.1",
	}
}

func startLocal(t *testing.T, cfg *config.Config) *app.App {
	t.Helper()
	a, err := app.Start(app.Options{Config: cfg})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return a
}

func get(t *testing.T, rawURL string) (int, string) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(rawURL)
	if err != nil {
		t.Fatalf("GET %s: %v", rawURL, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, string(b)
}

func TestStartServesAPIAndUI(t *testing.T) {
	a := startLocal(t, localCfg(t))
	defer a.Close()

	if a.URL == "" {
		t.Fatal("App.URL is empty")
	}
	u, err := url.Parse(a.URL)
	if err != nil || u.Hostname() != "127.0.0.1" {
		t.Fatalf("local mode must serve on loopback, got URL %q (err %v)", a.URL, err)
	}
	if u.Port() == "0" || u.Port() == "" {
		t.Fatalf("App.URL must carry the actually-bound port, got %q", a.URL)
	}

	if code, _ := get(t, a.URL+"/api/health"); code != http.StatusOK {
		t.Fatalf("/api/health = %d, want 200", code)
	}
	code, body := get(t, a.URL+"/api/auth/mode")
	if code != http.StatusOK {
		t.Fatalf("/api/auth/mode = %d, want 200", code)
	}
	var mode struct {
		MultiUser bool `json:"multiUser"`
	}
	if err := json.Unmarshal([]byte(body), &mode); err != nil {
		t.Fatalf("auth/mode body: %v", err)
	}
	if mode.MultiUser {
		t.Fatalf("local mode reported multiUser=true: %s", body)
	}

	// Local mode has exactly one implicit user, so the auth wrapper must
	// pass through and the account list comes back as an empty array.
	code, body = get(t, a.URL+"/api/accounts")
	if code != http.StatusOK {
		t.Fatalf("/api/accounts = %d, want 200 (local user auto-auth)", code)
	}
	var accounts struct {
		Accounts []any `json:"accounts"`
	}
	if err := json.Unmarshal([]byte(body), &accounts); err != nil {
		t.Fatalf("/api/accounts body: %v (%s)", err, body)
	}
	if len(accounts.Accounts) != 0 {
		t.Fatalf("fresh install has accounts: %s", body)
	}

	code, body = get(t, a.URL+"/")
	if code != http.StatusOK || len(body) == 0 {
		t.Fatalf("SPA shell = %d/%d bytes, want 200 and non-empty", code, len(body))
	}
}

func TestCloseStopsHTTPServer(t *testing.T) {
	a := startLocal(t, localCfg(t))
	servedURL := a.URL
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	if _, err := client.Get(servedURL + "/api/health"); err == nil {
		t.Fatal("server still answering after Close")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	a := startLocal(t, localCfg(t))
	if err := a.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestSecretStorePerMode(t *testing.T) {
	local := startLocal(t, localCfg(t))
	if err := local.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(local.Cfg.DataDir, "secret.key")); !os.IsNotExist(err) {
		t.Fatalf("local mode must not create secret.key, stat err=%v", err)
	}

	mu := localCfg(t)
	mu.MultiUser = true
	a := startLocal(t, mu)
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(a.Cfg.DataDir, "secret.key")); err != nil {
		t.Fatalf("multiuser mode must create secret.key (encrypted store): %v", err)
	}
}
