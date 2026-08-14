// Package provider defines the connector abstraction every cloud backend
// implements, plus the registry of supported providers.
//
// Design notes (settled in planning, 2026-08):
//   - Three auth families: OAuth2 (browser loopback redirect), personal
//     access tokens, and WebDAV credentials.
//   - Native connectors get one-click auth, incremental indexing and quota
//     support. Long-tail providers ride on an rclone sidecar bridge behind
//     the same interface and are promoted to native over time.
package provider

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/pleumcloud/pleumcloud/internal/secret"
)

// AuthKind identifies how a provider authenticates.
type AuthKind string

const (
	AuthOAuth2 AuthKind = "oauth2"
	AuthPAT    AuthKind = "pat"
	AuthWebDAV AuthKind = "webdav"
	AuthBridge AuthKind = "bridge" // experimental rclone bridge
)

// Tier describes implementation status, surfaced in the UI as a badge.
type Tier string

const (
	TierNative       Tier = "native"       // full one-click experience
	TierExperimental Tier = "experimental" // rclone bridge, best-effort
)

// Metadata is static provider information safe to expose to the UI.
type Metadata struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	AuthKind   AuthKind `json:"authKind"`
	Tier       Tier     `json:"tier"`
	FreeTierGB int      `json:"freeTierGB"`
	DocsURL    string   `json:"docsUrl,omitempty"`
	// MaxUploadBytes is the largest single file the free tier accepts
	// (0 = unknown or unlimited).
	MaxUploadBytes int64 `json:"maxUploadBytes,omitempty"`
}

// ErrUnsupported marks optional capabilities a provider lacks (e.g. share
// links on MyBox, quota on plain WebDAV).
var ErrUnsupported = errors.New("capability not supported by this provider")

// File is a provider-agnostic file or folder entry.
type File struct {
	RemoteID string    `json:"remoteId"`
	ParentID string    `json:"parentId"`
	Name     string    `json:"name"`
	IsDir    bool      `json:"isDir"`
	Size     int64     `json:"size"`
	MIME     string    `json:"mime,omitempty"`
	ModTime  time.Time `json:"modTime,omitempty"`
	// ThumbnailURL, when non-empty, is a provider-native image link valid
	// for a short window (Google/OneDrive generate these server-side).
	ThumbnailURL string `json:"thumbnailUrl,omitempty"`
}

// Quota reports storage usage for an account.
type Quota struct {
	TotalBytes int64 `json:"totalBytes"`
	UsedBytes  int64 `json:"usedBytes"`
}

// Changes is a page of incremental updates from a provider delta feed.
type Changes struct {
	Cursor   string   `json:"cursor"`
	Upserted []File   `json:"upserted"`
	Deleted  []string `json:"deleted"` // remote IDs
	HasMore  bool     `json:"hasMore"`
}

// ProgressFn receives cumulative byte counts during long transfers.
type ProgressFn func(done, total int64)

// Connector is the complete backend contract. Native providers implement
// everything; the rclone bridge implements it generically.
type Connector interface {
	Metadata() Metadata

	// List returns children of a folder ('' = account root). When pageToken
	// is empty the listing starts from the beginning; a non-empty returned
	// token means more pages remain.
	List(ctx context.Context, acct AccountRef, parentRemoteID, pageToken string) (files []File, nextToken string, err error)

	Quota(ctx context.Context, acct AccountRef) (Quota, error)

	// Changes returns incremental modifications since the given cursor
	// (empty cursor = initial full listing).
	Changes(ctx context.Context, acct AccountRef, cursor string) (Changes, error)

	Upload(ctx context.Context, acct AccountRef, parentRemoteID, name string, r io.Reader, size int64, progress ProgressFn) (File, error)

	// Open returns a reader for the file's content.
	Open(ctx context.Context, acct AccountRef, remoteID string, progress ProgressFn) (io.ReadCloser, error)

	Mkdir(ctx context.Context, acct AccountRef, parentRemoteID, name string) (File, error)
	Move(ctx context.Context, acct AccountRef, remoteID, newParentRemoteID, newName string) (File, error)
	Copy(ctx context.Context, acct AccountRef, remoteID, newParentRemoteID, newName string) (File, error)
	Delete(ctx context.Context, acct AccountRef, remoteID string) error

	// ShareLink creates (or with create=false, revokes) a public link.
	ShareLink(ctx context.Context, acct AccountRef, remoteID string, create bool) (string, error)
}

// AccountRef identifies a connected account; credentials live in the secret
// store and are looked up by SecretRef, never passed around in the clear.
type AccountRef struct {
	ID         string
	ProviderID string
	SecretRef  string
}

// Deps are the shared dependencies connectors are built with.
type Deps struct {
	Secrets secret.Store
}

// Factory builds a connector wired with deps.
type Factory func(Deps) Connector

// factories holds connector builders keyed by provider ID; connector
// packages register via RegisterFactory from init().
var factories = map[string]Factory{}

// RegisterFactory registers a connector builder for a provider ID.
func RegisterFactory(id string, f Factory) { factories[id] = f }

// Build constructs the connector for a provider ID.
func Build(id string, deps Deps) (Connector, bool) {
	f, ok := factories[id]
	if !ok {
		return nil, false
	}
	return f(deps), true
}

// Supported reports whether a connector implementation is registered
// (vs. catalog-only providers awaiting their bridge).
func Supported(id string) bool { _, ok := factories[id]; return ok }
