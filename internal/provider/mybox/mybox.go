// Package mybox implements the Naver MyBox connector over the MyBox Open
// API (opened 2026-08-11). Auth is a personal access token; uploads and
// downloads go through short-lived presigned URLs issued by the API.
package mybox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gachon-star-want/pleumcloud/internal/provider"
	"github.com/gachon-star-want/pleumcloud/internal/secret"
)

// apiBase is a var so tests can point it at a fake server.
var apiBase = "https://open-api.mybox.naver.com"

// The MyBox API rate-limits (PLAT-429) without documented thresholds, so
// the initial walk paces itself and backs off hard on 429 — live testing
// showed the window can outlast short retries. Vars for tests.
var (
	mbWalkPace    = 150 * time.Millisecond // pause between listing calls
	mb429Backoff  = 1 * time.Second        // first backoff, doubling per retry
	mb429MaxTries = 7
)

func init() {
	provider.RegisterFactory("mybox", New)
}

type connector struct{ secrets secret.Store }

// New builds the Naver MyBox connector.
func New(deps provider.Deps) provider.Connector { return &connector{secrets: deps.Secrets} }

func (c *connector) Metadata() provider.Metadata {
	return provider.Metadata{
		ID: "mybox", Name: "Naver MyBox", AuthKind: provider.AuthPAT,
		Tier: provider.TierNative, FreeTierGB: 30,
		DocsURL: "https://developers.mybox.naver.com",
	}
}

type patBundle struct {
	PAT string `json:"pat"`
}

func (c *connector) httpClient(ctx context.Context, acct provider.AccountRef) (*http.Client, map[string]string, error) {
	var pb patBundle
	if err := secret.GetJSON(c.secrets, acct.SecretRef, &pb); err != nil {
		return nil, nil, fmt.Errorf("load credentials: %w", err)
	}
	if pb.PAT == "" {
		return nil, nil, errors.New("empty MyBox token")
	}
	return &http.Client{Timeout: 0}, map[string]string{"Authorization": "Bearer " + pb.PAT}, nil
}

type mbError struct {
	Status int
	Code   string
	Msg    string
}

func (e *mbError) Error() string {
	return fmt.Sprintf("mybox: HTTP %d %s: %s", e.Status, e.Code, e.Msg)
}

func checkResp(resp *http.Response, body []byte) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &e)
	if e.Code == "" {
		e.Code = "PLAT-" + strconv.Itoa(resp.StatusCode)
	}
	return &mbError{Status: resp.StatusCode, Code: e.Code, Msg: e.Message}
}

// do performs an authenticated JSON request. rawBody, when non-nil, is sent
// as-is; out decodes the JSON response when non-nil.
func (c *connector) do(ctx context.Context, acct provider.AccountRef, method, u string, body any, out any) ([]byte, error) {
	hc, headers, err := c.httpClient(ctx, acct)
	if err != nil {
		return nil, err
	}
	var rd io.Reader
	if body != nil {
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
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := checkResp(resp, raw); err != nil {
		return nil, err
	}
	if out != nil && len(raw) > 0 {
		return raw, json.Unmarshal(raw, out)
	}
	return raw, nil
}

// ---- model mapping ----

type mbResource struct {
	ResourceID string `json:"resourceId"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Size       int64  `json:"size"`
	ParentID   string `json:"parentId"`
	Category   string `json:"category"`
	ModifiedAt string `json:"modifiedAt"`
	CreatedAt  string `json:"createdAt"`
}

func toFile(r mbResource) provider.File {
	mt, _ := time.Parse(time.RFC3339, r.ModifiedAt)
	parent := r.ParentID
	if strings.EqualFold(parent, "root") {
		parent = "" // unified root is keyed on ""
	}
	return provider.File{
		RemoteID: r.ResourceID,
		ParentID: parent,
		Name:     r.Name,
		IsDir:    strings.Contains(strings.ToLower(r.Type), "folder"),
		Size:     r.Size,
		MIME:     mimeFor(r),
		ModTime:  mt,
	}
}

// pinnedMIME keeps the HEIC family identical on every OS: system mime
// tables disagree (macOS resolves .heic to image/heic, some Linux tables
// to image/heif, most none at all) and gallery filtering plus preview
// transcoding key off this value.
var pinnedMIME = map[string]string{
	".heic": "image/heic",
	".heif": "image/heic",
	".hif":  "image/heic",
}

// mimeFor derives a MIME type: a pinned table for the HEIC family first,
// then the file extension, then the API's category as a coarse fallback
// (HEIC files are why the fallback exists — few systems resolve .heic
// from their mime tables).
func mimeFor(r mbResource) string {
	if strings.Contains(strings.ToLower(r.Type), "folder") {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(r.Name))
	if m, ok := pinnedMIME[ext]; ok {
		return m
	}
	if m := mime.TypeByExtension(ext); m != "" {
		return m
	}
	switch strings.ToLower(r.Category) {
	case "image", "video", "audio":
		return strings.ToLower(r.Category) + "/" + strings.TrimPrefix(ext, ".")
	}
	return ""
}

// ---- Connector implementation ----

func (c *connector) listURL(parentRemoteID string) string {
	if parentRemoteID == "" {
		return apiBase + "/v1/drive/resources"
	}
	return apiBase + "/v1/drive/folders/" + url.PathEscape(parentRemoteID) + "/resources"
}

func (c *connector) List(ctx context.Context, acct provider.AccountRef, parentRemoteID, pageToken string) ([]provider.File, string, error) {
	// Docs cap `count` at 1000 (default 100); max pages mean the fewest
	// requests against the undocumented rate limit.
	q := url.Values{"sort": {"name,asc"}, "count": {"1000"}}
	if pageToken != "" {
		q.Set("cursor", pageToken)
	}
	var out struct {
		Resources        []mbResource `json:"resources"`
		ResponseMetaData struct {
			NextCursor string `json:"nextCursor"`
		} `json:"responseMetaData"`
	}
	if _, err := c.do(ctx, acct, http.MethodGet, c.listURL(parentRemoteID)+"?"+q.Encode(), nil, &out); err != nil {
		return nil, "", err
	}
	files := make([]provider.File, 0, len(out.Resources))
	for _, r := range out.Resources {
		f := toFile(r)
		if parentRemoteID == "" {
			// Root listings report the account's opaque root-folder id as
			// parentId (base64 blob, not a fixed sentinel); the unified
			// root is keyed on "".
			f.ParentID = ""
		}
		files = append(files, f)
	}
	return files, out.ResponseMetaData.NextCursor, nil
}

func (c *connector) Quota(ctx context.Context, acct provider.AccountRef) (provider.Quota, error) {
	var out struct {
		QuotaBytes int64 `json:"quotaBytes"`
		UsedBytes  int64 `json:"usedBytes"`
	}
	if _, err := c.do(ctx, acct, http.MethodGet, apiBase+"/v1/drive/storage", nil, &out); err != nil {
		return provider.Quota{}, err
	}
	return provider.Quota{TotalBytes: out.QuotaBytes, UsedBytes: out.UsedBytes}, nil
}

// Changes: MyBox has no delta feed — a bounded BFS walk stands in for the
// initial index. The walk is paced and retries on 429 so a rate-limited
// burst doesn't kill the sync.
func (c *connector) Changes(ctx context.Context, acct provider.AccountRef, cursor string) (provider.Changes, error) {
	if cursor != "" {
		// No incremental cursor exists yet; tell the indexer to re-walk.
		_ = cursor
	}
	var all []provider.File
	type frame struct{ parent string }
	queue := []frame{{}}
	for len(queue) > 0 && len(all) < 50000 {
		cur := queue[0]
		queue = queue[1:]
		token := ""
		for {
			files, next, err := c.listWithRetry(ctx, acct, cur.parent, token)
			if err != nil {
				return provider.Changes{}, err
			}
			for _, f := range files {
				all = append(all, f)
				if f.IsDir {
					queue = append(queue, frame{f.RemoteID})
				}
			}
			if next == "" {
				break
			}
			token = next
			if err := mbSleep(ctx, mbWalkPace); err != nil {
				return provider.Changes{}, err
			}
		}
		if err := mbSleep(ctx, mbWalkPace); err != nil {
			return provider.Changes{}, err
		}
	}
	return provider.Changes{Cursor: "walk", Upserted: all}, nil
}

// listWithRetry lists one page, retrying with backoff while the API
// answers 429 (the docs give no thresholds, only the error code).
func (c *connector) listWithRetry(ctx context.Context, acct provider.AccountRef, parent, token string) ([]provider.File, string, error) {
	delay := mb429Backoff
	for try := 0; ; try++ {
		files, next, err := c.List(ctx, acct, parent, token)
		var mb *mbError
		if !errors.As(err, &mb) || mb.Status != http.StatusTooManyRequests || try >= mb429MaxTries {
			return files, next, err
		}
		if err := mbSleep(ctx, delay); err != nil {
			return nil, "", err
		}
		delay *= 2
	}
}

func mbSleep(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *connector) Upload(ctx context.Context, acct provider.AccountRef, parentRemoteID, name string, r io.Reader, size int64, progress provider.ProgressFn) (provider.File, error) {
	body := map[string]any{"fileName": name, "fileSize": size, "isOverwrite": true}
	if parentRemoteID != "" {
		body["parentId"] = parentRemoteID
	}
	var init struct {
		UploadURL string `json:"uploadUrl"`
		Offset    int64  `json:"offset"`
	}
	if _, err := c.do(ctx, acct, http.MethodPost, apiBase+"/v1/drive/files", body, &init); err != nil {
		return provider.File{}, err
	}
	if init.Offset != 0 {
		return provider.File{}, fmt.Errorf("mybox: unexpected resume offset %d for a fresh upload", init.Offset)
	}

	hc, _, err := c.httpClient(ctx, acct)
	if err != nil {
		return provider.File{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, init.UploadURL, r)
	if err != nil {
		return provider.File{}, err
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := hc.Do(req)
	if err != nil {
		return provider.File{}, err
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if err := checkResp(resp, raw); err != nil {
		return provider.File{}, err
	}
	if progress != nil {
		progress(size, size)
	}

	// The storage upload returns minimal info; fetch the metadata via the
	// parent listing filtered by name (the API has no by-name lookup).
	files, _, err := c.List(ctx, acct, parentRemoteID, "")
	if err != nil {
		return provider.File{}, fmt.Errorf("upload succeeded but locating the file failed: %w", err)
	}
	for _, f := range files {
		if f.Name == name && !f.IsDir {
			return f, nil
		}
	}
	return provider.File{Name: name, Size: size}, nil
}

func (c *connector) Open(ctx context.Context, acct provider.AccountRef, remoteID string, progress provider.ProgressFn) (io.ReadCloser, error) {
	var out struct {
		DownloadURL string `json:"downloadUrl"`
		ExpiresIn   int    `json:"expiresIn"`
	}
	if _, err := c.do(ctx, acct, http.MethodGet, apiBase+"/v1/drive/files/"+url.PathEscape(remoteID)+"/download", nil, &out); err != nil {
		return nil, err
	}
	if out.DownloadURL == "" {
		return nil, errors.New("mybox: empty downloadUrl")
	}
	hc, _, err := c.httpClient(ctx, acct)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, out.DownloadURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, &mbError{Status: resp.StatusCode, Msg: "storage download failed"}
	}
	return resp.Body, nil
}

func (c *connector) Mkdir(ctx context.Context, acct provider.AccountRef, parentRemoteID, name string) (provider.File, error) {
	body := map[string]any{"folderName": name}
	if parentRemoteID != "" {
		body["parentId"] = parentRemoteID
	}
	var out struct {
		Name       string `json:"name"`
		ResourceID string `json:"resourceId"`
	}
	if _, err := c.do(ctx, acct, http.MethodPost, apiBase+"/v1/drive/folders", body, &out); err != nil {
		return provider.File{}, err
	}
	return provider.File{RemoteID: out.ResourceID, Name: out.Name, IsDir: true, ParentID: parentRemoteID}, nil
}

func (c *connector) Move(ctx context.Context, acct provider.AccountRef, remoteID, newParentRemoteID, newName string) (provider.File, error) {
	if newName != "" {
		if _, err := c.do(ctx, acct, http.MethodPost,
			apiBase+"/v1/drive/resources/"+url.PathEscape(remoteID)+"/rename",
			map[string]string{"name": newName}, nil); err != nil {
			return provider.File{}, err
		}
	}
	if newParentRemoteID != "" {
		body := map[string]any{"parentId": newParentRemoteID, "isOverwrite": true}
		if _, err := c.do(ctx, acct, http.MethodPost,
			apiBase+"/v1/drive/resources/"+url.PathEscape(remoteID)+"/move", body, nil); err != nil {
			return provider.File{}, err
		}
	}
	var res mbResource
	if _, err := c.do(ctx, acct, http.MethodGet, apiBase+"/v1/drive/resources/"+url.PathEscape(remoteID), nil, &res); err != nil {
		return provider.File{}, err
	}
	return toFile(res), nil
}

func (c *connector) Copy(ctx context.Context, acct provider.AccountRef, remoteID, newParentRemoteID, newName string) (provider.File, error) {
	body := map[string]any{"isOverwrite": true}
	if newParentRemoteID != "" {
		body["parentId"] = newParentRemoteID
	}
	if newName != "" {
		body["name"] = newName
	}
	var out struct {
		Name       string `json:"name"`
		ResourceID string `json:"resourceId"`
	}
	if _, err := c.do(ctx, acct, http.MethodPost,
		apiBase+"/v1/drive/resources/"+url.PathEscape(remoteID)+"/copy", body, &out); err != nil {
		return provider.File{}, err
	}
	return provider.File{RemoteID: out.ResourceID, Name: out.Name, ParentID: newParentRemoteID}, nil
}

func (c *connector) Delete(ctx context.Context, acct provider.AccountRef, remoteID string) error {
	_, err := c.do(ctx, acct, http.MethodDelete, apiBase+"/v1/drive/resources/"+url.PathEscape(remoteID), nil, nil)
	return err
}

// ShareLink: the MyBox Open API exposes no public share-link endpoint yet.
func (c *connector) ShareLink(ctx context.Context, acct provider.AccountRef, remoteID string, create bool) (string, error) {
	return "", provider.ErrUnsupported
}
