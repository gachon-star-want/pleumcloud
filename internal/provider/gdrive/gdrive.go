// Package gdrive implements the Google Drive connector over the Drive API
// v3 REST endpoints. A hand-rolled client (rather than google-api-go-client)
// keeps the single-binary footprint small and gives us direct control of
// resumable uploads and the changes feed.
package gdrive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/pleumcloud/pleumcloud/internal/oauthflow"
	"github.com/pleumcloud/pleumcloud/internal/provider"
	"github.com/pleumcloud/pleumcloud/internal/secret"
)

// Base URLs are vars so tests can point them at a fake server.
var (
	apiBase    = "https://www.googleapis.com/drive/v3"
	uploadBase = "https://www.googleapis.com/upload/drive/v3"
)

const (
	folderMIME   = "application/vnd.google-apps.folder"
	fieldsCommon = "id,name,mimeType,size,modifiedTime,parents,thumbnailLink"
)

func init() {
	provider.RegisterFactory("gdrive", New)
}

type connector struct{ secrets secret.Store }

// New builds the Google Drive connector.
func New(deps provider.Deps) provider.Connector { return &connector{secrets: deps.Secrets} }

func (c *connector) Metadata() provider.Metadata {
	return provider.Metadata{
		ID: "gdrive", Name: "Google Drive", AuthKind: provider.AuthOAuth2,
		Tier: provider.TierNative, FreeTierGB: 15,
		DocsURL: "https://developers.google.com/drive",
	}
}

type tokenBundle struct {
	Token oauth2.Token `json:"token"`
}

// client resolves the account token and returns an auto-refreshing client.
func (c *connector) client(ctx context.Context, acct provider.AccountRef) (*http.Client, error) {
	var tb tokenBundle
	if err := secret.GetJSON(c.secrets, acct.SecretRef, &tb); err != nil {
		return nil, fmt.Errorf("load credentials: %w", err)
	}
	spec := oauthflow.Specs["gdrive"]
	conf := &oauth2.Config{Endpoint: oauth2.Endpoint{AuthURL: spec.AuthURL, TokenURL: spec.TokenURL}}
	return oauthflow.NewClient(ctx, c.secrets, acct.SecretRef, conf, &tb.Token), nil
}

// ---- shared request helpers ----

type apiError struct {
	Status int
	Code   string
	Msg    string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("gdrive: HTTP %d %s: %s", e.Status, e.Code, e.Msg)
}

func checkResp(resp *http.Response, body []byte) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	var e struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Errors  []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &e)
	msg := e.Error.Message
	if len(e.Error.Errors) > 0 && e.Error.Errors[0].Reason != "" {
		msg = e.Error.Errors[0].Reason + ": " + msg
	}
	return &apiError{Status: resp.StatusCode, Msg: msg}
}

// do performs an authenticated JSON request and decodes into out (when non-nil).
func do(ctx context.Context, hc *http.Client, method, url string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json; charset=UTF-8")
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
	if err := checkResp(resp, raw); err != nil {
		return err
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// ---- model mapping ----

type gdFile struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	MIME          string   `json:"mimeType"`
	Size          string   `json:"size"`
	ModifiedTime  string   `json:"modifiedTime"`
	Parents       []string `json:"parents"`
	ThumbnailLink string   `json:"thumbnailLink"`
	Trashed       bool     `json:"trashed"`
	WebViewLink   string   `json:"webViewLink"`
}

func toFile(f gdFile) provider.File {
	size, _ := strconv.ParseInt(f.Size, 10, 64)
	mt, _ := time.Parse(time.RFC3339, f.ModifiedTime)
	parent := ""
	if len(f.Parents) > 0 {
		parent = f.Parents[0]
	}
	return provider.File{
		RemoteID:     f.ID,
		ParentID:     parent,
		Name:         f.Name,
		IsDir:        f.MIME == folderMIME,
		Size:         size,
		MIME:         f.MIME,
		ModTime:      mt,
		ThumbnailURL: f.ThumbnailLink,
	}
}

// ---- Connector implementation ----

func (c *connector) List(ctx context.Context, acct provider.AccountRef, parentRemoteID, pageToken string) ([]provider.File, string, error) {
	if parentRemoteID == "" {
		parentRemoteID = "root"
	}
	hc, err := c.client(ctx, acct)
	if err != nil {
		return nil, "", err
	}
	q := url.Values{
		"q":                         {fmt.Sprintf("'%s' in parents and trashed = false", parentRemoteID)},
		"fields":                    {"files(" + fieldsCommon + "),nextPageToken"},
		"pageSize":                  {"200"},
		"supportsAllDrives":         {"true"},
		"includeItemsFromAllDrives": {"true"},
	}
	if pageToken != "" {
		q.Set("pageToken", pageToken)
	}
	var out struct {
		Files         []gdFile `json:"files"`
		NextPageToken string   `json:"nextPageToken"`
	}
	if err := do(ctx, hc, http.MethodGet, apiBase+"/files?"+q.Encode(), nil, &out); err != nil {
		return nil, "", err
	}
	files := make([]provider.File, 0, len(out.Files))
	for _, f := range out.Files {
		files = append(files, toFile(f))
	}
	return files, out.NextPageToken, nil
}

func (c *connector) Quota(ctx context.Context, acct provider.AccountRef) (provider.Quota, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.Quota{}, err
	}
	var out struct {
		StorageQuota struct {
			Limit string `json:"limit"`
			Usage string `json:"usage"`
		} `json:"storageQuota"`
		User struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"user"`
	}
	if err := do(ctx, hc, http.MethodGet, apiBase+"/about?fields=storageQuota,user(emailAddress)", nil, &out); err != nil {
		return provider.Quota{}, err
	}
	q := provider.Quota{}
	q.TotalBytes, _ = strconv.ParseInt(out.StorageQuota.Limit, 10, 64)
	q.UsedBytes, _ = strconv.ParseInt(out.StorageQuota.Usage, 10, 64)
	return q, nil
}

// AccountLabel fetches the account email for display (used at connect time).
func (c *connector) AccountLabel(ctx context.Context, acct provider.AccountRef) (string, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return "", err
	}
	var out struct {
		User struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"user"`
	}
	if err := do(ctx, hc, http.MethodGet, apiBase+"/about?fields=user(emailAddress)", nil, &out); err != nil {
		return "", err
	}
	return out.User.EmailAddress, nil
}

func (c *connector) Changes(ctx context.Context, acct provider.AccountRef, cursor string) (provider.Changes, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.Changes{}, err
	}
	if cursor == "" {
		var start struct {
			StartPageToken string `json:"startPageToken"`
		}
		if err := do(ctx, hc, http.MethodGet, apiBase+"/changes/startPageToken", nil, &start); err != nil {
			return provider.Changes{}, err
		}
		return provider.Changes{Cursor: start.StartPageToken}, nil
	}
	q := url.Values{
		"pageToken":                 {cursor},
		"includeRemoved":            {"true"},
		"restrictToMyDrive":         {"false"},
		"supportsAllDrives":         {"true"},
		"includeItemsFromAllDrives": {"true"},
		"fields":                    {"changes(fileId,removed,file(" + fieldsCommon + ",trashed)),newStartPageToken,nextPageToken"},
		"pageSize":                  {"500"},
	}
	var out struct {
		Changes []struct {
			FileID  string `json:"fileId"`
			Removed bool   `json:"removed"`
			File    gdFile `json:"file"`
		} `json:"changes"`
		NewStartPageToken string `json:"newStartPageToken"`
		NextPageToken     string `json:"nextPageToken"`
	}
	if err := do(ctx, hc, http.MethodGet, apiBase+"/changes?"+q.Encode(), nil, &out); err != nil {
		return provider.Changes{}, err
	}
	ch := provider.Changes{HasMore: out.NextPageToken != ""}
	if out.NewStartPageToken != "" {
		ch.Cursor = out.NewStartPageToken
	} else {
		ch.Cursor = out.NextPageToken
	}
	for _, cg := range out.Changes {
		if cg.Removed || cg.File.Trashed || cg.File.ID == "" {
			ch.Deleted = append(ch.Deleted, cg.FileID)
			continue
		}
		ch.Upserted = append(ch.Upserted, toFile(cg.File))
	}
	return ch, nil
}

// countingReader reports upload progress.
type countingReader struct {
	r        io.Reader
	n        int64
	progress provider.ProgressFn
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	cr.n += int64(n)
	if cr.progress != nil {
		cr.progress(cr.n, -1)
	}
	return n, err
}

func (c *connector) Upload(ctx context.Context, acct provider.AccountRef, parentRemoteID, name string, r io.Reader, size int64, progress provider.ProgressFn) (provider.File, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.File{}, err
	}
	if parentRemoteID == "" {
		parentRemoteID = "root"
	}
	meta := map[string]any{"name": name}
	if parentRemoteID != "root" {
		meta["parents"] = []string{parentRemoteID}
	}
	metaBuf, _ := json.Marshal(meta)

	// Step 1: open a resumable session.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		uploadBase+"/files?uploadType=resumable&supportsAllDrives=true&fields="+url.QueryEscape(fieldsCommon),
		bytes.NewReader(metaBuf))
	if err != nil {
		return provider.File{}, err
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Length", strconv.FormatInt(size, 10))
	resp, err := hc.Do(req)
	if err != nil {
		return provider.File{}, err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return provider.File{}, fmt.Errorf("gdrive: resumable init HTTP %d", resp.StatusCode)
	}
	session := resp.Header.Get("Location")
	if session == "" {
		return provider.File{}, errors.New("gdrive: no upload session Location")
	}

	// Step 2: PUT the bytes (single shot; resumable chunking lands with the
	// transfer engine in M4).
	body := io.Reader(&countingReader{r: r, progress: progress})
	req2, err := http.NewRequestWithContext(ctx, http.MethodPut, session, body)
	if err != nil {
		return provider.File{}, err
	}
	req2.ContentLength = size
	resp2, err := hc.Do(req2)
	if err != nil {
		return provider.File{}, err
	}
	defer resp2.Body.Close()
	raw, _ := io.ReadAll(resp2.Body)
	if err := checkResp(resp2, raw); err != nil {
		return provider.File{}, err
	}
	var f gdFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return provider.File{}, err
	}
	return toFile(f), nil
}

func (c *connector) Open(ctx context.Context, acct provider.AccountRef, remoteID string, progress provider.ProgressFn) (io.ReadCloser, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/files/"+remoteID+"?alt=media&supportsAllDrives=true", nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, &apiError{Status: resp.StatusCode, Msg: string(body)}
	}
	return resp.Body, nil
}

func (c *connector) Mkdir(ctx context.Context, acct provider.AccountRef, parentRemoteID, name string) (provider.File, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.File{}, err
	}
	if parentRemoteID == "" {
		parentRemoteID = "root"
	}
	meta := map[string]any{"name": name, "mimeType": folderMIME}
	if parentRemoteID != "root" {
		meta["parents"] = []string{parentRemoteID}
	}
	var f gdFile
	if err := do(ctx, hc, http.MethodPost, apiBase+"/files?fields="+url.QueryEscape(fieldsCommon), meta, &f); err != nil {
		return provider.File{}, err
	}
	return toFile(f), nil
}

func (c *connector) Move(ctx context.Context, acct provider.AccountRef, remoteID, newParentRemoteID, newName string) (provider.File, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.File{}, err
	}

	// Drive moves are add/remove parent pairs; we need the current parent.
	var cur gdFile
	if err := do(ctx, hc, http.MethodGet, apiBase+"/files/"+remoteID+"?fields=parents,name", nil, &cur); err != nil {
		return provider.File{}, err
	}
	var removeParent string
	if len(cur.Parents) > 0 && cur.Parents[0] != newParentRemoteID {
		removeParent = cur.Parents[0]
	}

	q := url.Values{"fields": {fieldsCommon}, "supportsAllDrives": {"true"}}
	if newParentRemoteID != "" {
		q.Set("addParents", newParentRemoteID)
	}
	if removeParent != "" {
		q.Set("removeParents", removeParent)
	}
	body := map[string]any{}
	if newName != "" && newName != cur.Name {
		body["name"] = newName
	}
	if len(body) == 0 && newParentRemoteID == "" && removeParent == "" {
		return toFile(cur), nil // no-op
	}
	var f gdFile
	if err := do(ctx, hc, http.MethodPatch, apiBase+"/files/"+remoteID+"?"+q.Encode(), body, &f); err != nil {
		return provider.File{}, err
	}
	return toFile(f), nil
}

func (c *connector) Copy(ctx context.Context, acct provider.AccountRef, remoteID, newParentRemoteID, newName string) (provider.File, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.File{}, err
	}
	body := map[string]any{}
	if newName != "" {
		body["name"] = newName
	}
	if newParentRemoteID != "" && newParentRemoteID != "root" {
		body["parents"] = []string{newParentRemoteID}
	}
	var f gdFile
	if err := do(ctx, hc, http.MethodPost,
		apiBase+"/files/"+remoteID+"/copy?fields="+url.QueryEscape(fieldsCommon)+"&supportsAllDrives=true",
		body, &f); err != nil {
		return provider.File{}, err
	}
	return toFile(f), nil
}

func (c *connector) Delete(ctx context.Context, acct provider.AccountRef, remoteID string) error {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return err
	}
	return do(ctx, hc, http.MethodDelete, apiBase+"/files/"+remoteID+"?supportsAllDrives=true", nil, nil)
}

func (c *connector) ShareLink(ctx context.Context, acct provider.AccountRef, remoteID string, create bool) (string, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return "", err
	}
	if !create {
		err := do(ctx, hc, http.MethodDelete, apiBase+"/files/"+remoteID+"/permissions/anyone?supportsAllDrives=true", nil, nil)
		if err != nil && strings.Contains(err.Error(), "404") {
			return "", nil // no link existed; fine
		}
		return "", err
	}
	if err := do(ctx, hc, http.MethodPost,
		apiBase+"/files/"+remoteID+"/permissions?supportsAllDrives=true",
		map[string]string{"role": "reader", "type": "anyone"}, nil); err != nil {
		return "", err
	}
	var f gdFile
	if err := do(ctx, hc, http.MethodGet, apiBase+"/files/"+remoteID+"?fields=webViewLink", nil, &f); err != nil {
		return "", err
	}
	return f.WebViewLink, nil
}
