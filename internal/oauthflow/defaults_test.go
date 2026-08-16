package oauthflow

import (
	"net/url"
	"testing"
)

// unsetBuiltinEnv neutralizes ambient env overrides so tests are hermetic
// on machines that export PLEUMCLOUD_OAUTH_* (dev shells, CI secrets).
func unsetBuiltinEnv(t *testing.T) {
	t.Helper()
	for id := range Specs {
		t.Setenv(envName(id, "CLIENT_ID"), "")
		t.Setenv(envName(id, "CLIENT_SECRET"), "")
	}
}

// TestBuiltinAppsStartEmptyAndTakeEnv verifies the compiled official apps
// ship empty (until registered) and that env vars override both fields.
func TestBuiltinAppsStartEmptyAndTakeEnv(t *testing.T) {
	unsetBuiltinEnv(t)
	for id, c := range BuiltinApps() {
		if c.ID != "" || c.Secret != "" {
			t.Fatalf("builtin %s should be empty before registration, got %+v", id, c)
		}
	}
	t.Setenv(envName("gdrive", "CLIENT_ID"), "env-id")
	t.Setenv(envName("gdrive", "CLIENT_SECRET"), "env-secret")
	apps := BuiltinApps()
	if apps["gdrive"].ID != "env-id" || apps["gdrive"].Secret != "env-secret" {
		t.Fatalf("env override ignored: %+v", apps["gdrive"])
	}
}

// TestBuiltinPrecedence pins the credential order: BYO > built-in > error.
func TestBuiltinPrecedence(t *testing.T) {
	unsetBuiltinEnv(t)
	m := NewManager(memSecret{})
	if _, err := m.Start("gdrive", "http://127.0.0.1:7777"); err != ErrNoClientID {
		t.Fatalf("err = %v, want ErrNoClientID", err)
	}

	m.SetBuiltin(map[string]ClientID{"gdrive": {ID: "official-id", Secret: "official-secret"}})
	if !m.HasBuiltin("gdrive") {
		t.Fatal("HasBuiltin(gdrive) = false after SetBuiltin")
	}
	authURL, err := m.Start("gdrive", "http://127.0.0.1:7777")
	if err != nil {
		t.Fatal(err)
	}
	q, _ := url.Parse(authURL)
	if q.Query().Get("client_id") != "official-id" {
		t.Errorf("builtin client_id = %q", q.Query().Get("client_id"))
	}

	m.SetBYO("gdrive", ClientID{ID: "byo-id"})
	authURL, err = m.Start("gdrive", "http://127.0.0.1:7777")
	if err != nil {
		t.Fatal(err)
	}
	q, _ = url.Parse(authURL)
	if q.Query().Get("client_id") != "byo-id" {
		t.Errorf("BYO should win over builtin, got client_id = %q", q.Query().Get("client_id"))
	}
}

// TestSetBuiltinIgnoresEmpty ensures a half-configured map (empty IDs)
// can't wipe a good built-in app.
func TestSetBuiltinIgnoresEmpty(t *testing.T) {
	unsetBuiltinEnv(t)
	m := NewManager(memSecret{})
	m.SetBuiltin(map[string]ClientID{"gdrive": {ID: "official-id"}})
	m.SetBuiltin(map[string]ClientID{"gdrive": {}})
	if !m.HasBuiltin("gdrive") {
		t.Fatal("empty SetBuiltin entry wiped the official app")
	}
}

// TestSpecsUsePKCE keeps the PKCE flags honest for the providers whose
// authorization servers support S256.
func TestSpecsUsePKCE(t *testing.T) {
	for _, id := range []string{"gdrive", "onedrive", "dropbox"} {
		if !Specs[id].UsePKCE {
			t.Errorf("%s should require PKCE", id)
		}
	}
}
