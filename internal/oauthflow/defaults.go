package oauthflow

import (
	"os"
	"strings"
)

// builtinApps holds the project's official OAuth app credentials. The keys
// stay empty until each provider's app is registered and approved — fill
// them in as registrations land (docs/oauth-setup.md walks every console).
//
// Publishing client IDs/secrets in an open-source repo is the rclone model:
// a local-first binary cannot keep a secret confidential anyway, and the
// registered redirect URIs are what actually bound abuse. Env overrides let
// server-mode deployments swap in their own apps without a rebuild.
var builtinApps = map[string]ClientID{
	"gdrive":   {},
	"onedrive": {},
	"dropbox":  {},
	"pcloud":   {},
}

// BuiltinApps returns the official apps with environment overrides applied:
// PLEUMCLOUD_OAUTH_<PROVIDER>_CLIENT_ID and _CLIENT_SECRET replace the
// compiled values per provider.
func BuiltinApps() map[string]ClientID {
	out := make(map[string]ClientID, len(builtinApps))
	for id, c := range builtinApps {
		if v := os.Getenv(envName(id, "CLIENT_ID")); v != "" {
			c.ID = v
		}
		if v := os.Getenv(envName(id, "CLIENT_SECRET")); v != "" {
			c.Secret = v
		}
		out[id] = c
	}
	return out
}

func envName(provider, suffix string) string {
	return "PLEUMCLOUD_OAUTH_" + strings.ToUpper(provider) + "_" + suffix
}
