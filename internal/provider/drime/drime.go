// Package drime implements the Drime connector over its public API v1
// (spec: docs.drime.cloud/openapi.yaml). Auth is a personal access token
// from Account Settings → Developers.
package drime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gachon-star-want/pleumcloud/internal/provider"
	"github.com/gachon-star-want/pleumcloud/internal/secret"
)

// apiBase is a var so tests can point it at a fake server.
var apiBase = "https://app.drime.cloud/api/v1"

func init() {
	provider.RegisterFactory("drime", New)
}

type connector struct{ secrets secret.Store }

// New builds the Drime connector.
func New(deps provider.Deps) provider.Connector { return &connector{secrets: deps.Secrets} }

func (c *connector) Metadata() provider.Metadata {
	return provider.Metadata{
		ID: "drime", Name: "Drime", AuthKind: provider.AuthPAT,
		Tier: provider.TierNative, FreeTierGB: 20,
		DocsURL: "https://docs.drime.cloud",
	}
}

type patBundle struct {
	PAT string `json:"pat"`
}

func (c *connector) httpClient(acct provider.AccountRef) (*http.Client, error) {
	var pb patBundle
	if err := secret.GetJSON(c.secrets, acct.SecretRef, &pb); err != nil {
		return nil, fmt.Errorf("load credentials: %w", err)
	}
	if pb.PAT == "" {
		return nil, fmt.Errorf("empty Drime token")
	}
	return &http.Client{}, nil
}

// ---- helpers ----

type drError struct {
	Status int
	Msg    string
}

func (e *drError) Error() string {
	return fmt.Sprintf("drime: HTTP %d: %s", e.Status, e.Msg)
}

// do performs an authenticated JSON request. body (any) marshals to JSON;
// out decodes the response when non-nil.
func (c *connector) do(ctx context.Context, acct provider.AccountRef, method, u string, body any, out any) error {
	pb, err := c.pat(acct)
	if err != nil {
		return err
	}
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
	req.Header.Set("Authorization", "Bearer "+pb.PAT)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return &drError{Status: resp.StatusCode, Msg: strings.TrimSpace(string(raw))}
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func (c *connector) pat(acct provider.AccountRef) (patBundle, error) {
	var pb patBundle
	if err := secret.GetJSON(c.secrets, acct.SecretRef, &pb); err != nil {
		return pb, fmt.Errorf("load credentials: %w", err)
	}
	if pb.PAT == "" {
		return pb, fmt.Errorf("empty Drime token")
	}
	return pb, nil
}

// ---- model ----

type drEntry struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"` // image | folder | file | text | audio | video | pdf
	FileSize int64  `json:"file_size"`
	ParentID *int64 `json:"parent_id"`
	MIME     string `json:"mime"`
	Hash     string `json:"hash"`
	Updated  string `json:"updated_at"`
}

func toFile(e drEntry) provider.File {
	mt, _ := time.Parse("2006-01-02T15:04:05.000000Z", e.Updated)
	parent := ""
	if e.ParentID != nil {
		parent = strconv.FormatInt(*e.ParentID, 10)
	}
	return provider.File{
		RemoteID: strconv.FormatInt(e.ID, 10),
		ParentID: parent,
		Name:     e.Name,
		IsDir:    e.Type == "folder",
		Size:     e.FileSize,
		MIME:     e.MIME,
		ModTime:  mt,
	}
}

// ---- Connector ----

func (c *connector) listURL(parentRemoteID, pageToken string) string {
	u := apiBase + "/drive/file-entries?perPage=100"
	if parentRemoteID != "" {
		u += "&folderId=" + parentRemoteID
	}
	if pageToken != "" {
		u += "&page=" + pageToken
	}
	return u
}

func (c *connector) List(ctx context.Context, acct provider.AccountRef, parentRemoteID, pageToken string) ([]provider.File, string, error) {
	var out struct {
		Data        []drEntry `json:"data"`
		CurrentPage int       `json:"current_page"`
		LastPage    int       `json:"last_page"`
	}
	if err := c.do(ctx, acct, http.MethodGet, c.listURL(parentRemoteID, pageToken), nil, &out); err != nil {
		return nil, "", err
	}
	files := make([]provider.File, 0, len(out.Data))
	for _, e := range out.Data {
		files = append(files, toFile(e))
	}
	next := ""
	if out.CurrentPage < out.LastPage {
		next = strconv.Itoa(out.CurrentPage + 1)
	}
	return files, next, nil
}

func (c *connector) Quota(ctx context.Context, acct provider.AccountRef) (provider.Quota, error) {
	var out struct {
		Used      int64 `json:"used"`
		Available int64 `json:"available"`
	}
	if err := c.do(ctx, acct, http.MethodGet, apiBase+"/user/space-usage", nil, &out); err != nil {
		return provider.Quota{}, err
	}
	return provider.Quota{TotalBytes: out.Used + out.Available, UsedBytes: out.Used}, nil
}

// AccountLabel returns the account email for display.
func (c *connector) AccountLabel(ctx context.Context, acct provider.AccountRef) (string, error) {
	var out struct {
		Email string `json:"email"`
	}
	if err := c.do(ctx, acct, http.MethodGet, apiBase+"/cli/loggedUser", nil, &out); err != nil {
		return "", err
	}
	return out.Email, nil
}

// Changes: no delta feed — BFS walk over folders stands in.
func (c *connector) Changes(ctx context.Context, acct provider.AccountRef, cursor string) (provider.Changes, error) {
	var all []provider.File
	queue := []string{""}
	for len(queue) > 0 && len(all) < 50000 {
		cur := queue[0]
		queue = queue[1:]
		token := ""
		for {
			files, next, err := c.List(ctx, acct, cur, token)
			if err != nil {
				return provider.Changes{}, err
			}
			for _, f := range files {
				all = append(all, f)
				if f.IsDir {
					queue = append(queue, f.RemoteID)
				}
			}
			if next == "" {
				break
			}
			token = next
		}
	}
	return provider.Changes{Cursor: "walk", Upserted: all}, nil
}

func (c *connector) Upload(ctx context.Context, acct provider.AccountRef, parentRemoteID, name string, r io.Reader, size int64, progress provider.ProgressFn) (provider.File, error) {
	pb, err := c.pat(acct)
	if err != nil {
		return provider.File{}, err
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("parentId", parentRemoteID)
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/uploads", &body)
	if err != nil {
		return provider.File{}, err
	}
	req.Header.Set("Authorization", "Bearer "+pb.PAT)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return provider.File{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return provider.File{}, &drError{Status: resp.StatusCode, Msg: string(raw)}
	}
	var out struct {
		FileEntry drEntry `json:"fileEntry"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return provider.File{}, err
	}
	return toFile(out.FileEntry), nil
}

// Open resolves the entry hash first (downloads are keyed by hash), then
// streams from /file-entries/download/{hash}.
func (c *connector) Open(ctx context.Context, acct provider.AccountRef, remoteID string, progress provider.ProgressFn) (io.ReadCloser, error) {
	var entry drEntry
	if err := c.do(ctx, acct, http.MethodGet, apiBase+"/file-entries/"+remoteID, nil, &entry); err != nil {
		return nil, err
	}
	if entry.Hash == "" {
		return nil, fmt.Errorf("drime: entry %s has no hash", remoteID)
	}
	pb, err := c.pat(acct)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/file-entries/download/"+entry.Hash, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+pb.PAT)
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, &drError{Status: resp.StatusCode, Msg: string(body)}
	}
	return resp.Body, nil
}

func (c *connector) Mkdir(ctx context.Context, acct provider.AccountRef, parentRemoteID, name string) (provider.File, error) {
	body := map[string]any{"name": name, "parentId": nil}
	if parentRemoteID != "" {
		if id, err := strconv.ParseInt(parentRemoteID, 10, 64); err == nil {
			body["parentId"] = id
		}
	}
	var out struct {
		Folder drEntry `json:"folder"`
	}
	if err := c.do(ctx, acct, http.MethodPost, apiBase+"/folders", body, &out); err != nil {
		return provider.File{}, err
	}
	return toFile(out.Folder), nil
}

func (c *connector) Move(ctx context.Context, acct provider.AccountRef, remoteID, newParentRemoteID, newName string) (provider.File, error) {
	id, _ := strconv.ParseInt(remoteID, 10, 64)
	if newParentRemoteID != "" {
		dst, _ := strconv.ParseInt(newParentRemoteID, 10, 64)
		if err := c.do(ctx, acct, http.MethodPost, apiBase+"/file-entries/move",
			map[string]any{"entryIds": []int64{id}, "destinationId": dst}, nil); err != nil {
			return provider.File{}, err
		}
	}
	if newName != "" {
		var entry drEntry
		if err := c.do(ctx, acct, http.MethodPut, apiBase+"/file-entries/"+remoteID,
			map[string]string{"name": newName}, &entry); err != nil {
			return provider.File{}, err
		}
		return toFile(entry), nil
	}
	var entry drEntry
	if err := c.do(ctx, acct, http.MethodGet, apiBase+"/file-entries/"+remoteID, nil, &entry); err != nil {
		return provider.File{}, err
	}
	return toFile(entry), nil
}

// Copy duplicates an entry (Drime's duplicate API keeps the name; a
// follow-up rename applies newName when given).
func (c *connector) Copy(ctx context.Context, acct provider.AccountRef, remoteID, newParentRemoteID, newName string) (provider.File, error) {
	id, _ := strconv.ParseInt(remoteID, 10, 64)
	body := map[string]any{"entryIds": []int64{id}}
	if newParentRemoteID != "" {
		dst, _ := strconv.ParseInt(newParentRemoteID, 10, 64)
		body["destinationId"] = dst
	}
	if err := c.do(ctx, acct, http.MethodPost, apiBase+"/file-entries/duplicate", body, nil); err != nil {
		return provider.File{}, err
	}
	return provider.File{Name: newName, ParentID: newParentRemoteID}, nil
}

func (c *connector) Delete(ctx context.Context, acct provider.AccountRef, remoteID string) error {
	id, _ := strconv.ParseInt(remoteID, 10, 64)
	return c.do(ctx, acct, http.MethodPost, apiBase+"/file-entries/delete",
		map[string]any{"entryIds": []int64{id}}, nil)
}

// ShareLink: the API exposes shareable links keyed by hash but the spec
// does not document the public URL format; revisit once pinned.
func (c *connector) ShareLink(ctx context.Context, acct provider.AccountRef, remoteID string, create bool) (string, error) {
	return "", provider.ErrUnsupported
}
