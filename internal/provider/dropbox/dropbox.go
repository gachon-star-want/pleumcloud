// Package dropbox implements the Dropbox connector over the HTTP API v2.
// RPC calls (metadata) hit api.dropboxapi.com; content calls (bytes) hit
// content.dropboxapi.com with the parameters in a Dropbox-API-Arg header.
package dropbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/gachon-star-want/pleumcloud/internal/oauthflow"
	"github.com/gachon-star-want/pleumcloud/internal/provider"
	"github.com/gachon-star-want/pleumcloud/internal/secret"
)

// Base URLs are vars so tests can point them at a fake server.
var (
	rpcBase     = "https://api.dropboxapi.com"
	contentBase = "https://content.dropboxapi.com"
)

func init() {
	provider.RegisterFactory("dropbox", New)
}

type connector struct{ secrets secret.Store }

// New builds the Dropbox connector.
func New(deps provider.Deps) provider.Connector { return &connector{secrets: deps.Secrets} }

func (c *connector) Metadata() provider.Metadata {
	return provider.Metadata{
		ID: "dropbox", Name: "Dropbox", AuthKind: provider.AuthOAuth2,
		Tier: provider.TierNative, FreeTierGB: 2,
		DocsURL: "https://www.dropbox.com/developers/documentation/http/documentation",
	}
}

type tokenBundle struct {
	Token oauth2.Token `json:"token"`
}

func (c *connector) client(ctx context.Context, acct provider.AccountRef) (*http.Client, error) {
	var tb tokenBundle
	if err := secret.GetJSON(c.secrets, acct.SecretRef, &tb); err != nil {
		return nil, fmt.Errorf("load credentials: %w", err)
	}
	spec := oauthflow.Specs["dropbox"]
	conf := &oauth2.Config{Endpoint: oauth2.Endpoint{AuthURL: spec.AuthURL, TokenURL: spec.TokenURL}}
	return oauthflow.NewClient(ctx, c.secrets, acct.SecretRef, conf, &tb.Token), nil
}

// ---- helpers ----

type dbxError struct {
	Status int
	Msg    string
}

func (e *dbxError) Error() string {
	return fmt.Sprintf("dropbox: HTTP %d: %s", e.Status, e.Msg)
}

// rpc calls a JSON-argument Dropbox endpoint.
func rpc(ctx context.Context, hc *http.Client, method, u string, body any, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return &dbxError{Status: resp.StatusCode, Msg: strings.TrimSpace(string(raw))}
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// idToPath normalizes a remote ID (path without leading slash) for API use.
// The root ID maps to the empty string, which is Dropbox's root convention.
func idToPath(id string) string {
	if id == "" {
		return ""
	}
	return "/" + id
}

// parentOf derives the parent path from a lower-cased path ID.
func parentOf(id string) string {
	i := strings.LastIndex(id, "/")
	if i < 0 {
		return ""
	}
	return id[:i]
}

// ---- model ----

type dbxEntry struct {
	Tag       string `json:".tag"`
	Name      string `json:"name"`
	PathLower string `json:"path_lower"`
	ID        string `json:"id"`
	Size      int64  `json:"size"`
	Modified  string `json:"server_modified"`
}

func toFile(e dbxEntry) provider.File {
	id := strings.TrimPrefix(e.PathLower, "/")
	mt, _ := time.Parse(time.RFC3339, e.Modified)
	return provider.File{
		RemoteID: id,
		ParentID: parentOf(id),
		Name:     e.Name,
		IsDir:    e.Tag == "folder",
		Size:     e.Size,
		ModTime:  mt,
	}
}

// ---- Connector ----

func (c *connector) List(ctx context.Context, acct provider.AccountRef, parentRemoteID, pageToken string) ([]provider.File, string, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return nil, "", err
	}
	var out struct {
		Entries []dbxEntry `json:"entries"`
		Cursor  string     `json:"cursor"`
		HasMore bool       `json:"has_more"`
	}
	if pageToken == "" {
		// Dropbox expects the empty string (not "/") for the account root.
		err = rpc(ctx, hc, http.MethodPost, rpcBase+"/2/files/list_folder",
			map[string]any{"path": idToPath(parentRemoteID), "recursive": false, "include_deleted": false}, &out)
	} else {
		err = rpc(ctx, hc, http.MethodPost, rpcBase+"/2/files/list_folder/continue",
			map[string]any{"cursor": pageToken}, &out)
	}
	if err != nil {
		return nil, "", err
	}
	files := make([]provider.File, 0, len(out.Entries))
	for _, e := range out.Entries {
		files = append(files, toFile(e))
	}
	next := ""
	if out.HasMore {
		next = out.Cursor
	}
	return files, next, nil
}

func (c *connector) Quota(ctx context.Context, acct provider.AccountRef) (provider.Quota, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.Quota{}, err
	}
	var out struct {
		Used       int64 `json:"used"`
		Allocation struct {
			Tag       string `json:".tag"`
			Allocated int64  `json:"allocated"`
		} `json:"allocation"`
	}
	if err := rpc(ctx, hc, http.MethodPost, rpcBase+"/2/users/get_space_usage", map[string]any{}, &out); err != nil {
		return provider.Quota{}, err
	}
	return provider.Quota{TotalBytes: out.Allocation.Allocated, UsedBytes: out.Used}, nil
}

// AccountLabel returns the account email for display.
func (c *connector) AccountLabel(ctx context.Context, acct provider.AccountRef) (string, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return "", err
	}
	var out struct {
		Email string `json:"email"`
	}
	if err := rpc(ctx, hc, http.MethodPost, rpcBase+"/2/users/get_current_account", map[string]any{}, &out); err != nil {
		return "", err
	}
	return out.Email, nil
}

// Changes uses list_folder with recursive=true as a delta feed: the first
// call returns the full tree; later calls return changed/deleted entries.
func (c *connector) Changes(ctx context.Context, acct provider.AccountRef, cursor string) (provider.Changes, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.Changes{}, err
	}
	var out struct {
		Entries []dbxEntry `json:"entries"`
		Cursor  string     `json:"cursor"`
		HasMore bool       `json:"has_more"`
	}
	if cursor == "" {
		err = rpc(ctx, hc, http.MethodPost, rpcBase+"/2/files/list_folder",
			map[string]any{"path": "", "recursive": true, "include_deleted": true}, &out)
	} else {
		err = rpc(ctx, hc, http.MethodPost, rpcBase+"/2/files/list_folder/continue",
			map[string]any{"cursor": cursor}, &out)
	}
	if err != nil {
		return provider.Changes{}, err
	}
	ch := provider.Changes{Cursor: out.Cursor, HasMore: out.HasMore}
	for _, e := range out.Entries {
		if e.Tag == "deleted" {
			ch.Deleted = append(ch.Deleted, strings.TrimPrefix(e.PathLower, "/"))
			continue
		}
		ch.Upserted = append(ch.Upserted, toFile(e))
	}
	return ch, nil
}

func (c *connector) Upload(ctx context.Context, acct provider.AccountRef, parentRemoteID, name string, r io.Reader, size int64, progress provider.ProgressFn) (provider.File, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.File{}, err
	}
	dst := name
	if parentRemoteID != "" {
		dst = parentRemoteID + "/" + strings.ToLower(name)
	}
	arg, _ := json.Marshal(map[string]any{
		"path":       idToPath(dst),
		"mode":       map[string]string{".tag": "overwrite"},
		"autorename": false,
		"mute":       false,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, contentBase+"/2/files/upload", r)
	if err != nil {
		return provider.File{}, err
	}
	req.ContentLength = size
	req.Header.Set("Dropbox-API-Arg", string(arg))
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := hc.Do(req)
	if err != nil {
		return provider.File{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return provider.File{}, &dbxError{Status: resp.StatusCode, Msg: string(raw)}
	}
	if progress != nil {
		progress(size, size)
	}
	var e dbxEntry
	if err := json.Unmarshal(raw, &e); err != nil {
		return provider.File{}, err
	}
	return toFile(e), nil
}

func (c *connector) Open(ctx context.Context, acct provider.AccountRef, remoteID string, progress provider.ProgressFn) (io.ReadCloser, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return nil, err
	}
	arg, _ := json.Marshal(map[string]any{"path": idToPath(remoteID)})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, contentBase+"/2/files/download", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Dropbox-API-Arg", string(arg))
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, &dbxError{Status: resp.StatusCode, Msg: string(body)}
	}
	return resp.Body, nil
}

func (c *connector) Mkdir(ctx context.Context, acct provider.AccountRef, parentRemoteID, name string) (provider.File, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.File{}, err
	}
	path := "/" + name
	if parentRemoteID != "" {
		path = idToPath(parentRemoteID) + "/" + strings.ToLower(name)
	}
	var out struct {
		Metadata dbxEntry `json:"metadata"`
	}
	if err := rpc(ctx, hc, http.MethodPost, rpcBase+"/2/files/create_folder_v2",
		map[string]any{"path": path, "autorename": false}, &out); err != nil {
		return provider.File{}, err
	}
	return toFile(out.Metadata), nil
}

func (c *connector) Move(ctx context.Context, acct provider.AccountRef, remoteID, newParentRemoteID, newName string) (provider.File, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.File{}, err
	}
	to := "/" + strings.ToLower(newName)
	if newParentRemoteID != "" {
		to = idToPath(newParentRemoteID) + "/" + strings.ToLower(newName)
	}
	var out struct {
		Metadata dbxEntry `json:"metadata"`
	}
	if err := rpc(ctx, hc, http.MethodPost, rpcBase+"/2/files/move_v2",
		map[string]any{"from_path": idToPath(remoteID), "to_path": to, "autorename": false, "allow_ownership_transfer": false}, &out); err != nil {
		return provider.File{}, err
	}
	return toFile(out.Metadata), nil
}

func (c *connector) Copy(ctx context.Context, acct provider.AccountRef, remoteID, newParentRemoteID, newName string) (provider.File, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.File{}, err
	}
	to := "/" + strings.ToLower(newName)
	if newParentRemoteID != "" {
		to = idToPath(newParentRemoteID) + "/" + strings.ToLower(newName)
	}
	var out struct {
		Metadata dbxEntry `json:"metadata"`
	}
	if err := rpc(ctx, hc, http.MethodPost, rpcBase+"/2/files/copy_v2",
		map[string]any{"from_path": idToPath(remoteID), "to_path": to, "autorename": false}, &out); err != nil {
		return provider.File{}, err
	}
	return toFile(out.Metadata), nil
}

func (c *connector) Delete(ctx context.Context, acct provider.AccountRef, remoteID string) error {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return err
	}
	return rpc(ctx, hc, http.MethodPost, rpcBase+"/2/files/delete_v2",
		map[string]any{"path": idToPath(remoteID)}, nil)
}

func (c *connector) ShareLink(ctx context.Context, acct provider.AccountRef, remoteID string, create bool) (string, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return "", err
	}
	if !create {
		// revocation needs the link URL, so discover it via list_shared_links.
		var links struct {
			Links []struct {
				URL string `json:"url"`
			} `json:"links"`
		}
		if err := rpc(ctx, hc, http.MethodPost, rpcBase+"/2/sharing/list_shared_links",
			map[string]any{"path": idToPath(remoteID)}, &links); err != nil {
			return "", err
		}
		for _, l := range links.Links {
			if err := rpc(ctx, hc, http.MethodPost, rpcBase+"/2/sharing/revoke_shared_link",
				map[string]any{"url": l.URL}, nil); err != nil {
				return "", err
			}
		}
		return "", nil
	}
	// Reuse an existing link if one is already public.
	var links struct {
		Links []struct {
			URL string `json:"url"`
		} `json:"links"`
	}
	if err := rpc(ctx, hc, http.MethodPost, rpcBase+"/2/sharing/list_shared_links",
		map[string]any{"path": idToPath(remoteID), "direct_only": true}, &links); err == nil && len(links.Links) > 0 {
		return links.Links[0].URL, nil
	}
	var out struct {
		URL string `json:"url"`
	}
	body := map[string]any{
		"path": idToPath(remoteID),
		"settings": map[string]any{
			"requested_visibility": "public",
			"audience":             "public",
			"access":               "viewer",
		},
	}
	if err := rpc(ctx, hc, http.MethodPost, rpcBase+"/2/sharing/create_shared_link_with_settings", body, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

// OpenRange serves a byte range natively via the Range header on
// files/download (the official ranged variant).
func (c *connector) OpenRange(ctx context.Context, acct provider.AccountRef, remoteID string, start, length int64) (io.ReadCloser, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return nil, err
	}
	arg, _ := json.Marshal(map[string]any{"path": idToPath(remoteID)})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, contentBase+"/2/files/download", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Dropbox-API-Arg", string(arg))
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, start+length-1))
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, &dbxError{Status: resp.StatusCode, Msg: string(body)}
	}
	return resp.Body, nil
}
