// Package webdav implements the generic WebDAV connector. One connector
// covers Nextcloud, ownCloud, InfiniCLOUD, MagentaCloud, Koofr mounts,
// self-hosted servers and every other DAV-speaking service.
package webdav

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/studio-b12/gowebdav"

	"github.com/pleumcloud/pleumcloud/internal/provider"
	"github.com/pleumcloud/pleumcloud/internal/secret"
)

func init() {
	provider.RegisterFactory("webdav", New)
}

type connector struct{ secrets secret.Store }

// New builds the generic WebDAV connector.
func New(deps provider.Deps) provider.Connector { return &connector{secrets: deps.Secrets} }

func (c *connector) Metadata() provider.Metadata {
	return provider.Metadata{
		ID: "webdav", Name: "WebDAV", AuthKind: provider.AuthWebDAV,
		Tier: provider.TierNative, FreeTierGB: 0,
		DocsURL: "https://en.wikipedia.org/wiki/WebDAV",
	}
}

type credBundle struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (c *connector) client(acct provider.AccountRef) (*gowebdav.Client, error) {
	cb, err := c.creds(acct)
	if err != nil {
		return nil, err
	}
	cl := gowebdav.NewClient(strings.TrimRight(cb.URL, "/"), cb.Username, cb.Password)
	cl.SetTimeout(60 * time.Second)
	return cl, nil
}

// creds loads the URL/username/password bundle for an account.
func (c *connector) creds(acct provider.AccountRef) (credBundle, error) {
	var cb credBundle
	if err := secret.GetJSON(c.secrets, acct.SecretRef, &cb); err != nil {
		return cb, fmt.Errorf("load credentials: %w", err)
	}
	if cb.URL == "" {
		return cb, errors.New("empty WebDAV URL")
	}
	return cb, nil
}

// join builds a DAV path. Remote IDs are paths relative to the account
// root; "" is the root itself.
func join(parent, name string) string {
	if parent == "" {
		return "/" + name
	}
	return path.Join("/", parent, name)
}

func toFile(parent string, info fsInfo) provider.File {
	p := join(parent, info.name)
	return provider.File{
		RemoteID: strings.TrimPrefix(p, "/"),
		ParentID: strings.TrimPrefix(parent, "/"),
		Name:     info.name,
		IsDir:    info.isDir,
		Size:     info.size,
		ModTime:  info.modTime,
	}
}

// fsInfo adapts os.FileInfo to what we need without leaking it in signatures.
type fsInfo struct {
	name    string
	size    int64
	isDir   bool
	modTime time.Time
}

func adapt(parent string, fi interface {
	Name() string
	Size() int64
	IsDir() bool
	ModTime() time.Time
}) fsInfo {
	return fsInfo{name: fi.Name(), size: fi.Size(), isDir: fi.IsDir(), modTime: fi.ModTime()}
}

func (c *connector) List(ctx context.Context, acct provider.AccountRef, parentRemoteID, pageToken string) ([]provider.File, string, error) {
	cl, err := c.client(acct)
	if err != nil {
		return nil, "", err
	}
	dir := "/"
	if parentRemoteID != "" {
		dir = "/" + parentRemoteID
	}
	infos, err := cl.ReadDir(dir)
	if err != nil {
		return nil, "", fmt.Errorf("webdav PROPFIND %s: %w", dir, err)
	}
	files := make([]provider.File, 0, len(infos))
	for _, fi := range infos {
		files = append(files, toFile(parentRemoteID, adapt(parentRemoteID, fi)))
	}
	return files, "", nil // DAV has no cursor pagination
}

func (c *connector) Quota(ctx context.Context, acct provider.AccountRef) (provider.Quota, error) {
	return provider.Quota{}, provider.ErrUnsupported
}

// Changes: DAV has no delta feed — a BFS walk (bounded) stands in.
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
	cl, err := c.client(acct)
	if err != nil {
		return provider.File{}, err
	}
	dst := join(parentRemoteID, name)
	if err := cl.WriteStreamWithLength(dst, r, size, 0o644); err != nil {
		return provider.File{}, fmt.Errorf("webdav PUT %s: %w", dst, err)
	}
	if progress != nil {
		progress(size, size)
	}
	return provider.File{
		RemoteID: strings.TrimPrefix(dst, "/"),
		ParentID: parentRemoteID,
		Name:     name,
		Size:     size,
	}, nil
}

func (c *connector) Open(ctx context.Context, acct provider.AccountRef, remoteID string, progress provider.ProgressFn) (io.ReadCloser, error) {
	cl, err := c.client(acct)
	if err != nil {
		return nil, err
	}
	rc, err := cl.ReadStream("/" + remoteID)
	if err != nil {
		return nil, fmt.Errorf("webdav GET %s: %w", remoteID, err)
	}
	return rc, nil
}

func (c *connector) Mkdir(ctx context.Context, acct provider.AccountRef, parentRemoteID, name string) (provider.File, error) {
	cl, err := c.client(acct)
	if err != nil {
		return provider.File{}, err
	}
	p := join(parentRemoteID, name)
	if err := cl.Mkdir(p, 0o755); err != nil {
		return provider.File{}, fmt.Errorf("webdav MKCOL %s: %w", p, err)
	}
	return provider.File{RemoteID: strings.TrimPrefix(p, "/"), ParentID: parentRemoteID, Name: name, IsDir: true}, nil
}

func (c *connector) Move(ctx context.Context, acct provider.AccountRef, remoteID, newParentRemoteID, newName string) (provider.File, error) {
	cb, err := c.creds(acct)
	if err != nil {
		return provider.File{}, err
	}
	old := "/" + remoteID
	if newName == "" {
		newName = path.Base(old)
	}
	dst := join(newParentRemoteID, newName)
	if old == dst {
		return provider.File{RemoteID: remoteID, Name: newName}, nil
	}
	if err := c.moveOrCopy(ctx, cb, "MOVE", old, dst); err != nil {
		return provider.File{}, err
	}
	return provider.File{RemoteID: strings.TrimPrefix(dst, "/"), ParentID: newParentRemoteID, Name: newName}, nil
}

func (c *connector) Copy(ctx context.Context, acct provider.AccountRef, remoteID, newParentRemoteID, newName string) (provider.File, error) {
	cb, err := c.creds(acct)
	if err != nil {
		return provider.File{}, err
	}
	src := "/" + remoteID
	if newName == "" {
		newName = path.Base(src)
	}
	dst := join(newParentRemoteID, newName)
	if err := c.moveOrCopy(ctx, cb, "COPY", src, dst); err != nil {
		return provider.File{}, err
	}
	return provider.File{RemoteID: strings.TrimPrefix(dst, "/"), ParentID: newParentRemoteID, Name: newName}, nil
}

// moveOrCopy issues a single DAV MOVE/COPY with basic auth on a connection
// that is never reused. This bypasses gowebdav deliberately: Go's http
// stack can silently re-send bodiless requests on reused connections, and
// a retried MOVE with Overwrite:T destroys the already-moved destination.
func (c *connector) moveOrCopy(ctx context.Context, cb credBundle, method, src, dst string) error {
	base := strings.TrimRight(cb.URL, "/")
	req, err := http.NewRequestWithContext(ctx, method, base+src, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(cb.Username, cb.Password)
	req.Header.Set("Destination", base+dst)
	req.Header.Set("Overwrite", "T")

	client := &http.Client{
		Timeout:   120 * time.Second,
		Transport: &http.Transport{DisableKeepAlives: true},
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webdav %s %s→%s: %w", method, src, dst, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	switch resp.StatusCode {
	case http.StatusCreated, http.StatusNoContent, http.StatusOK:
		return nil
	default:
		return fmt.Errorf("webdav %s %s→%s: HTTP %d", method, src, dst, resp.StatusCode)
	}
}

func (c *connector) Delete(ctx context.Context, acct provider.AccountRef, remoteID string) error {
	cl, err := c.client(acct)
	if err != nil {
		return err
	}
	if err := cl.Remove("/" + remoteID); err != nil {
		return fmt.Errorf("webdav DELETE %s: %w", remoteID, err)
	}
	return nil
}

// ShareLink: plain WebDAV has no share-link concept (Nextcloud/OCS adds one;
// that lands with a Nextcloud-specific connector later).
func (c *connector) ShareLink(ctx context.Context, acct provider.AccountRef, remoteID string, create bool) (string, error) {
	return "", provider.ErrUnsupported
}

// Validate checks credentials at connect time (used by the API layer).
func (c *connector) Validate(cb credBundle) error {
	cl := gowebdav.NewClient(strings.TrimRight(cb.URL, "/"), cb.Username, cb.Password)
	cl.SetTimeout(15 * time.Second)
	if err := cl.Connect(); err != nil {
		return fmt.Errorf("connect %s: %w", cb.URL, err)
	}
	return nil
}
