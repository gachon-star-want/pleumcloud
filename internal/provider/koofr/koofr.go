// Package koofr implements the Koofr connector. Endpoints are pinned from
// the official go-koofrclient source: mount-scoped paths under /api/v2,
// content under /content/api/v2, HTTP Basic auth (email + API token from
// Settings → API Tokens).
package koofr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/gachon-star-want/pleumcloud/internal/provider"
	"github.com/gachon-star-want/pleumcloud/internal/secret"
)

// apiBase is a var so tests can point it at a fake server.
var apiBase = "https://app.koofr.net"

func init() {
	provider.RegisterFactory("koofr", New)
}

type connector struct{ secrets secret.Store }

// New builds the Koofr connector.
func New(deps provider.Deps) provider.Connector { return &connector{secrets: deps.Secrets} }

func (c *connector) Metadata() provider.Metadata {
	return provider.Metadata{
		ID: "koofr", Name: "Koofr", AuthKind: provider.AuthPAT,
		Tier: provider.TierNative, FreeTierGB: 10,
		DocsURL: "https://koofr.net/help/koofr-api-documentation/",
	}
}

type credBundle struct {
	Email string `json:"email"`
	PAT   string `json:"pat"`
}

func (c *connector) creds(acct provider.AccountRef) (credBundle, error) {
	var cb credBundle
	if err := secret.GetJSON(c.secrets, acct.SecretRef, &cb); err != nil {
		return cb, fmt.Errorf("load credentials: %w", err)
	}
	if cb.Email == "" || cb.PAT == "" {
		return cb, fmt.Errorf("koofr: email and API token required")
	}
	return cb, nil
}

// ---- helpers ----

type kfError struct {
	Status int
	Msg    string
}

func (e *kfError) Error() string { return fmt.Sprintf("koofr: HTTP %d: %s", e.Status, e.Msg) }

// do performs a Basic-auth request. body marshals to JSON when non-nil;
// raw, when non-nil, is sent as-is with the given content type.
func (c *connector) do(ctx context.Context, acct provider.AccountRef, method, u string, body any, raw []byte, contentType string, out any) (*http.Response, error) {
	cb, err := c.creds(acct)
	if err != nil {
		return nil, err
	}
	var rd io.Reader
	if raw != nil {
		rd = bytes.NewReader(raw)
	} else if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(cb.Email, cb.PAT)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	} else if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		resp.Body.Close()
		return nil, &kfError{Status: resp.StatusCode, Msg: strings.TrimSpace(string(raw))}
	}
	if out != nil {
		defer resp.Body.Close()
		buf, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return nil, json.Unmarshal(buf, out)
	}
	return resp, nil
}

// mountID resolves the primary (koofr-origin) mount.
func (c *connector) mountID(ctx context.Context, acct provider.AccountRef) (string, error) {
	var out struct {
		Mounts []struct {
			ID     string `json:"id"`
			Origin string `json:"origin"`
		} `json:"mounts"`
	}
	if _, err := c.do(ctx, acct, http.MethodGet, apiBase+"/api/v2/mounts", nil, nil, "", &out); err != nil {
		return "", err
	}
	for _, m := range out.Mounts {
		if m.Origin == "koofr" {
			return m.ID, nil
		}
	}
	if len(out.Mounts) > 0 {
		return out.Mounts[0].ID, nil
	}
	return "", fmt.Errorf("koofr: no mounts on account")
}

// mountDetails returns the primary mount record (quota lives there).
func (c *connector) mountDetails(ctx context.Context, acct provider.AccountRef) (struct {
	ID         string `json:"id"`
	SpaceTotal int64  `json:"spaceTotal"`
	SpaceUsed  int64  `json:"spaceUsed"`
	Owner      struct {
		Email string `json:"email"`
	} `json:"owner"`
}, error) {
	var m struct {
		ID         string `json:"id"`
		SpaceTotal int64  `json:"spaceTotal"`
		SpaceUsed  int64  `json:"spaceUsed"`
		Owner      struct {
			Email string `json:"email"`
		} `json:"owner"`
	}
	id, err := c.mountID(ctx, acct)
	if err != nil {
		return m, err
	}
	_, err = c.do(ctx, acct, http.MethodGet, apiBase+"/api/v2/mounts/"+url.PathEscape(id), nil, nil, "", &m)
	return m, err
}

// ---- model ----

type kfFile struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // "file" | "dir"
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	Modified    int64  `json:"modified"` // unix seconds
	ContentType string `json:"contentType"`
}

func toFile(f kfFile) provider.File {
	id := strings.TrimPrefix(f.Path, "/")
	parent := ""
	if i := strings.LastIndex(id, "/"); i >= 0 {
		parent = id[:i]
	}
	return provider.File{
		RemoteID: id,
		ParentID: parent,
		Name:     f.Name,
		IsDir:    f.Type == "dir",
		Size:     f.Size,
		MIME:     f.ContentType,
		ModTime:  time.Unix(f.Modified, 0).UTC(),
	}
}

func idToPath(id string) string { return "/" + id }

// ---- Connector ----

func (c *connector) List(ctx context.Context, acct provider.AccountRef, parentRemoteID, pageToken string) ([]provider.File, string, error) {
	mid, err := c.mountID(ctx, acct)
	if err != nil {
		return nil, "", err
	}
	q := url.Values{"path": {idToPath(parentRemoteID)}}
	var out struct {
		Files []kfFile `json:"files"`
	}
	if _, err := c.do(ctx, acct, http.MethodGet,
		apiBase+"/api/v2/mounts/"+url.PathEscape(mid)+"/files/list?"+q.Encode(),
		nil, nil, "", &out); err != nil {
		return nil, "", err
	}
	files := make([]provider.File, 0, len(out.Files))
	for _, f := range out.Files {
		files = append(files, toFile(f))
	}
	return files, "", nil // no pagination
}

func (c *connector) Quota(ctx context.Context, acct provider.AccountRef) (provider.Quota, error) {
	m, err := c.mountDetails(ctx, acct)
	if err != nil {
		return provider.Quota{}, err
	}
	return provider.Quota{TotalBytes: m.SpaceTotal, UsedBytes: m.SpaceUsed}, nil
}

// AccountLabel prefers the mount owner email, falling back to the
// credential email (identical in practice).
func (c *connector) AccountLabel(ctx context.Context, acct provider.AccountRef) (string, error) {
	cb, err := c.creds(acct)
	if err != nil {
		return "", err
	}
	return cb.Email, nil
}

// Changes: no delta feed — BFS walk stands in.
func (c *connector) Changes(ctx context.Context, acct provider.AccountRef, cursor string) (provider.Changes, error) {
	var all []provider.File
	queue := []string{""}
	for len(queue) > 0 && len(all) < 50000 {
		cur := queue[0]
		queue = queue[1:]
		files, _, err := c.List(ctx, acct, cur, "")
		if err != nil {
			return provider.Changes{}, err
		}
		for _, f := range files {
			all = append(all, f)
			if f.IsDir {
				queue = append(queue, f.RemoteID)
			}
		}
	}
	return provider.Changes{Cursor: "walk", Upserted: all}, nil
}

func (c *connector) Upload(ctx context.Context, acct provider.AccountRef, parentRemoteID, name string, r io.Reader, size int64, progress provider.ProgressFn) (provider.File, error) {
	mid, err := c.mountID(ctx, acct)
	if err != nil {
		return provider.File{}, err
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		return provider.File{}, err
	}
	if _, err := io.Copy(fw, r); err != nil {
		return provider.File{}, err
	}
	if err := mw.Close(); err != nil {
		return provider.File{}, err
	}
	if progress != nil {
		progress(size, size)
	}
	q := url.Values{"path": {idToPath(parentRemoteID)}, "filename": {name}, "info": {"true"}}
	var f kfFile
	if _, err := c.do(ctx, acct, http.MethodPost,
		apiBase+"/content/api/v2/mounts/"+url.PathEscape(mid)+"/files/put?"+q.Encode(),
		nil, body.Bytes(), mw.FormDataContentType(), &f); err != nil {
		return provider.File{}, err
	}
	return toFile(f), nil
}

func (c *connector) Open(ctx context.Context, acct provider.AccountRef, remoteID string, progress provider.ProgressFn) (io.ReadCloser, error) {
	mid, err := c.mountID(ctx, acct)
	if err != nil {
		return nil, err
	}
	q := url.Values{"path": {idToPath(remoteID)}}
	resp, err := c.do(ctx, acct, http.MethodGet,
		apiBase+"/content/api/v2/mounts/"+url.PathEscape(mid)+"/files/get?"+q.Encode(),
		nil, nil, "", nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (c *connector) Mkdir(ctx context.Context, acct provider.AccountRef, parentRemoteID, name string) (provider.File, error) {
	mid, err := c.mountID(ctx, acct)
	if err != nil {
		return provider.File{}, err
	}
	q := url.Values{"path": {idToPath(parentRemoteID)}, "name": {name}}
	if _, err := c.do(ctx, acct, http.MethodPost,
		apiBase+"/api/v2/mounts/"+url.PathEscape(mid)+"/files/folder?"+q.Encode(),
		nil, nil, "", nil); err != nil {
		return provider.File{}, err
	}
	return provider.File{RemoteID: path.Join(parentRemoteID, name), ParentID: parentRemoteID, Name: name, IsDir: true}, nil
}

func (c *connector) Move(ctx context.Context, acct provider.AccountRef, remoteID, newParentRemoteID, newName string) (provider.File, error) {
	mid, err := c.mountID(ctx, acct)
	if err != nil {
		return provider.File{}, err
	}
	if newName == "" {
		newName = path.Base(idToPath(remoteID))
	}
	to := idToPath(newParentRemoteID) + "/" + newName
	q := url.Values{"path": {idToPath(remoteID)}}
	body := map[string]string{"toMountId": mid, "toPath": to}
	if _, err := c.do(ctx, acct, http.MethodPut,
		apiBase+"/api/v2/mounts/"+url.PathEscape(mid)+"/files/move?"+q.Encode(),
		body, nil, "", nil); err != nil {
		return provider.File{}, err
	}
	// Refresh info for accurate metadata.
	var f kfFile
	q2 := url.Values{"path": {to}}
	if _, err := c.do(ctx, acct, http.MethodGet,
		apiBase+"/api/v2/mounts/"+url.PathEscape(mid)+"/files/info?"+q2.Encode(),
		nil, nil, "", &f); err == nil {
		return toFile(f), nil
	}
	return provider.File{RemoteID: strings.TrimPrefix(to, "/"), ParentID: newParentRemoteID, Name: newName}, nil
}

func (c *connector) Copy(ctx context.Context, acct provider.AccountRef, remoteID, newParentRemoteID, newName string) (provider.File, error) {
	mid, err := c.mountID(ctx, acct)
	if err != nil {
		return provider.File{}, err
	}
	if newName == "" {
		newName = path.Base(idToPath(remoteID))
	}
	to := idToPath(newParentRemoteID) + "/" + newName
	q := url.Values{"path": {idToPath(remoteID)}}
	body := map[string]any{"toMountId": mid, "toPath": to, "setModified": true}
	if _, err := c.do(ctx, acct, http.MethodPut,
		apiBase+"/api/v2/mounts/"+url.PathEscape(mid)+"/files/copy?"+q.Encode(),
		body, nil, "", nil); err != nil {
		return provider.File{}, err
	}
	return provider.File{RemoteID: strings.TrimPrefix(to, "/"), ParentID: newParentRemoteID, Name: newName}, nil
}

func (c *connector) Delete(ctx context.Context, acct provider.AccountRef, remoteID string) error {
	mid, err := c.mountID(ctx, acct)
	if err != nil {
		return err
	}
	q := url.Values{"path": {idToPath(remoteID)}}
	_, err = c.do(ctx, acct, http.MethodDelete,
		apiBase+"/api/v2/mounts/"+url.PathEscape(mid)+"/files/remove?"+q.Encode(),
		nil, nil, "", nil)
	return err
}

// ShareLink: links exist (POST /links) but the public share-page URL
// format is not pinned from a primary source; revisit with the link
// response documented.
func (c *connector) ShareLink(ctx context.Context, acct provider.AccountRef, remoteID string, create bool) (string, error) {
	return "", provider.ErrUnsupported
}
