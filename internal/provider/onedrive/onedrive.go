// Package onedrive implements the Microsoft OneDrive connector over the
// Graph API (personal Microsoft accounts).
package onedrive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/pleumcloud/pleumcloud/internal/oauthflow"
	"github.com/pleumcloud/pleumcloud/internal/provider"
	"github.com/pleumcloud/pleumcloud/internal/secret"
)

// graphBase is a var so tests can point it at a fake server.
var graphBase = "https://graph.microsoft.com/v1.0"

func init() {
	provider.RegisterFactory("onedrive", New)
}

type connector struct{ secrets secret.Store }

// New builds the OneDrive connector.
func New(deps provider.Deps) provider.Connector { return &connector{secrets: deps.Secrets} }

func (c *connector) Metadata() provider.Metadata {
	return provider.Metadata{
		ID: "onedrive", Name: "Microsoft OneDrive", AuthKind: provider.AuthOAuth2,
		Tier: provider.TierNative, FreeTierGB: 5,
		DocsURL: "https://learn.microsoft.com/onedrive/developer/rest-api/",
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
	spec := oauthflow.Specs["onedrive"]
	conf := &oauth2.Config{Endpoint: oauth2.Endpoint{AuthURL: spec.AuthURL, TokenURL: spec.TokenURL}}
	return oauthflow.NewClient(ctx, c.secrets, acct.SecretRef, conf, &tb.Token), nil
}

type graphError struct {
	Status int
	Msg    string
}

func (e *graphError) Error() string {
	return fmt.Sprintf("onedrive: HTTP %d: %s", e.Status, e.Msg)
}

// do performs an authenticated request. body marshals to JSON when non-nil
// (except for raw uploads); out decodes the JSON response when non-nil.
func do(ctx context.Context, hc *http.Client, method, u string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(raw))
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Error.Message != "" {
			msg = e.Error.Message
		}
		return &graphError{Status: resp.StatusCode, Msg: msg}
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil && string(raw) != "" {
			return fmt.Errorf("decode %s %s: %w", method, u, err)
		}
	}
	return nil
}

// ---- model ----

type gdItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Modified string `json:"lastModifiedDateTime"`
	Parent   struct {
		ID string `json:"id"`
	} `json:"parentReference"`
	Folder *struct{} `json:"folder"`
	File   *struct {
		MIMEType string `json:"mimeType"`
	} `json:"file"`
	Deleted *struct{} `json:"deleted"`
}

func toFile(it gdItem) provider.File {
	mt, _ := time.Parse(time.RFC3339, it.Modified)
	mime := ""
	if it.File != nil {
		mime = it.File.MIMEType
	}
	return provider.File{
		RemoteID: it.ID,
		ParentID: it.Parent.ID,
		Name:     it.Name,
		IsDir:    it.Folder != nil,
		Size:     it.Size,
		MIME:     mime,
		ModTime:  mt,
	}
}

// ---- Connector ----

func (c *connector) List(ctx context.Context, acct provider.AccountRef, parentRemoteID, pageToken string) ([]provider.File, string, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return nil, "", err
	}
	path := "/me/drive/root/children"
	if parentRemoteID != "" {
		path = "/me/drive/items/" + url.PathEscape(parentRemoteID) + "/children"
	}
	q := url.Values{"$top": {"200"}}
	if pageToken != "" {
		q.Set("$skiptoken", pageToken)
	}
	var out struct {
		Value []gdItem `json:"value"`
		Next  string   `json:"@odata.nextLink"`
	}
	if err := do(ctx, hc, http.MethodGet, graphBase+path+"?"+q.Encode(), nil, &out); err != nil {
		return nil, "", err
	}
	files := make([]provider.File, 0, len(out.Value))
	for _, it := range out.Value {
		files = append(files, toFile(it))
	}
	next := ""
	if out.Next != "" {
		if u, err := url.Parse(out.Next); err == nil {
			next = u.Query().Get("$skiptoken")
		}
	}
	return files, next, nil
}

func (c *connector) Quota(ctx context.Context, acct provider.AccountRef) (provider.Quota, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.Quota{}, err
	}
	var out struct {
		Quota struct {
			Total int64 `json:"total"`
			Used  int64 `json:"used"`
		} `json:"quota"`
	}
	if err := do(ctx, hc, http.MethodGet, graphBase+"/me/drive", nil, &out); err != nil {
		return provider.Quota{}, err
	}
	return provider.Quota{TotalBytes: out.Quota.Total, UsedBytes: out.Quota.Used}, nil
}

// AccountLabel returns the Microsoft account address for display.
func (c *connector) AccountLabel(ctx context.Context, acct provider.AccountRef) (string, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return "", err
	}
	var out struct {
		UPN string `json:"userPrincipalName"`
	}
	if err := do(ctx, hc, http.MethodGet, graphBase+"/me", nil, &out); err != nil {
		return "", err
	}
	return out.UPN, nil
}

func (c *connector) Changes(ctx context.Context, acct provider.AccountRef, cursor string) (provider.Changes, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.Changes{}, err
	}
	q := url.Values{}
	if cursor != "" {
		q.Set("token", cursor)
	}
	var out struct {
		Value     []gdItem `json:"value"`
		DeltaLink string   `json:"@odata.deltaLink"`
		NextLink  string   `json:"@odata.nextLink"`
	}
	if err := do(ctx, hc, http.MethodGet, graphBase+"/me/drive/root/delta?"+q.Encode(), nil, &out); err != nil {
		return provider.Changes{}, err
	}
	ch := provider.Changes{HasMore: out.NextLink != ""}
	switch {
	case out.DeltaLink != "":
		if u, err := url.Parse(out.DeltaLink); err == nil {
			ch.Cursor = u.Query().Get("$deltatoken")
		}
	case out.NextLink != "":
		if u, err := url.Parse(out.NextLink); err == nil {
			ch.Cursor = u.Query().Get("$skiptoken")
		}
	}
	for _, it := range out.Value {
		if it.Deleted != nil {
			ch.Deleted = append(ch.Deleted, it.ID)
			continue
		}
		ch.Upserted = append(ch.Upserted, toFile(it))
	}
	return ch, nil
}

func (c *connector) Upload(ctx context.Context, acct provider.AccountRef, parentRemoteID, name string, r io.Reader, size int64, progress provider.ProgressFn) (provider.File, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.File{}, err
	}
	// Simple content upload (fine up to 250 MB; resumable sessions land
	// with the transfer engine in M4).
	path := "/me/drive/root:/" + escapePath(name) + ":/content"
	if parentRemoteID != "" {
		path = "/me/drive/items/" + url.PathEscape(parentRemoteID) + ":/" + escapePath(name) + ":/content"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, graphBase+path, r)
	if err != nil {
		return provider.File{}, err
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := hc.Do(req)
	if err != nil {
		return provider.File{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return provider.File{}, &graphError{Status: resp.StatusCode, Msg: string(raw)}
	}
	var it gdItem
	if err := json.Unmarshal(raw, &it); err != nil {
		return provider.File{}, err
	}
	if progress != nil {
		progress(size, size)
	}
	return toFile(it), nil
}

// escapePath encodes a filename for use inside a :/path:/ URL segment.
func escapePath(name string) string {
	return url.PathEscape(strings.ReplaceAll(name, "/", "%2F"))
}

func (c *connector) Open(ctx context.Context, acct provider.AccountRef, remoteID string, progress provider.ProgressFn) (io.ReadCloser, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Get(graphBase + "/me/drive/items/" + url.PathEscape(remoteID) + "/content")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, &graphError{Status: resp.StatusCode, Msg: string(body)}
	}
	return resp.Body, nil
}

func (c *connector) Mkdir(ctx context.Context, acct provider.AccountRef, parentRemoteID, name string) (provider.File, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.File{}, err
	}
	path := "/me/drive/root/children"
	if parentRemoteID != "" {
		path = "/me/drive/items/" + url.PathEscape(parentRemoteID) + "/children"
	}
	body := map[string]any{
		"name":                              name,
		"folder":                            map[string]any{},
		"@microsoft.graph.conflictBehavior": "rename",
	}
	var it gdItem
	if err := do(ctx, hc, http.MethodPost, graphBase+path, body, &it); err != nil {
		return provider.File{}, err
	}
	return toFile(it), nil
}

func (c *connector) Move(ctx context.Context, acct provider.AccountRef, remoteID, newParentRemoteID, newName string) (provider.File, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.File{}, err
	}
	body := map[string]any{}
	if newName != "" {
		body["name"] = newName
	}
	if newParentRemoteID != "" {
		body["parentReference"] = map[string]any{"id": newParentRemoteID}
	}
	var it gdItem
	if err := do(ctx, hc, http.MethodPatch,
		graphBase+"/me/drive/items/"+url.PathEscape(remoteID), body, &it); err != nil {
		return provider.File{}, err
	}
	return toFile(it), nil
}

// Copy is asynchronous in Graph (202 + monitor URL); M2 accepts the job
// without waiting — the indexer's next delta picks up the result.
func (c *connector) Copy(ctx context.Context, acct provider.AccountRef, remoteID, newParentRemoteID, newName string) (provider.File, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.File{}, err
	}
	body := map[string]any{}
	if newName != "" {
		body["name"] = newName
	}
	if newParentRemoteID != "" {
		body["parentReference"] = map[string]any{"id": newParentRemoteID}
	}
	if err := do(ctx, hc, http.MethodPost,
		graphBase+"/me/drive/items/"+url.PathEscape(remoteID)+"/copy", body, nil); err != nil {
		return provider.File{}, err
	}
	return provider.File{Name: newName, ParentID: newParentRemoteID}, nil
}

func (c *connector) Delete(ctx context.Context, acct provider.AccountRef, remoteID string) error {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return err
	}
	return do(ctx, hc, http.MethodDelete, graphBase+"/me/drive/items/"+url.PathEscape(remoteID), nil, nil)
}

func (c *connector) ShareLink(ctx context.Context, acct provider.AccountRef, remoteID string, create bool) (string, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return "", err
	}
	base := graphBase + "/me/drive/items/" + url.PathEscape(remoteID)
	if !create {
		// Anonymous links are revoked by deleting their permission entry.
		var perms struct {
			Value []struct {
				ID   string `json:"id"`
				Link *struct {
					Scope string `json:"scope"`
				} `json:"link"`
				GrantedToIdentities []any `json:"grantedToIdentities"`
			} `json:"value"`
		}
		if err := do(ctx, hc, http.MethodGet, base+"/permissions", nil, &perms); err != nil {
			return "", err
		}
		for _, p := range perms.Value {
			if p.Link != nil && p.Link.Scope == "anonymous" {
				if err := do(ctx, hc, http.MethodDelete, base+"/permissions/"+url.PathEscape(p.ID), nil, nil); err != nil {
					return "", err
				}
			}
		}
		return "", nil
	}
	var out struct {
		Link struct {
			WebURL string `json:"webUrl"`
		} `json:"link"`
	}
	body := map[string]any{"type": "view", "scope": "anonymous"}
	if err := do(ctx, hc, http.MethodPost, base+"/createLink", body, &out); err != nil {
		return "", err
	}
	return out.Link.WebURL, nil
}
