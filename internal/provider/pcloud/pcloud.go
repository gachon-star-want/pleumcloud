// Package pcloud implements the pCloud connector over the HTTP JSON API.
// All methods answer {result: 0, ...} — non-zero results are errors.
// Remote IDs use pCloud's own "f<fileid>" / "d<folderid>" string format.
package pcloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/gachon-star-want/pleumcloud/internal/oauthflow"
	"github.com/gachon-star-want/pleumcloud/internal/provider"
	"github.com/gachon-star-want/pleumcloud/internal/secret"
)

// apiBase is a var so tests can point it at a fake server. EU accounts
// use eapi.pcloud.com; account-region discovery can come later.
var apiBase = "https://api.pcloud.com"

// pCloud serves HTTP dates like "Thu, 14 Aug 2026 00:00:00 +0000".
const httpDate = "Mon, 02 Jan 2006 15:04:05 -0700"

func init() {
	provider.RegisterFactory("pcloud", New)
}

type connector struct{ secrets secret.Store }

// New builds the pCloud connector.
func New(deps provider.Deps) provider.Connector { return &connector{secrets: deps.Secrets} }

func (c *connector) Metadata() provider.Metadata {
	return provider.Metadata{
		ID: "pcloud", Name: "pCloud", AuthKind: provider.AuthOAuth2,
		Tier: provider.TierNative, FreeTierGB: 10,
		DocsURL: "https://docs.pcloud.com",
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
	spec := oauthflow.Specs["pcloud"]
	conf := &oauth2.Config{Endpoint: oauth2.Endpoint{AuthURL: spec.AuthURL, TokenURL: spec.TokenURL}}
	return oauthflow.NewClient(ctx, c.secrets, acct.SecretRef, conf, &tb.Token), nil
}

// ---- helpers ----

type pcError struct {
	Result int
	Msg    string
}

func (e *pcError) Error() string {
	return fmt.Sprintf("pcloud: result %d: %s", e.Result, e.Msg)
}

// call performs an authenticated request with query parameters and decodes
// the JSON envelope, converting non-zero results to errors.
func call(ctx context.Context, hc *http.Client, method, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 500 {
		return &pcError{Result: resp.StatusCode, Msg: strings.TrimSpace(string(raw))}
	}
	var envelope struct {
		Result int    `json:"result"`
		Error  string `json:"error"`
	}
	_ = json.Unmarshal(raw, &envelope)
	if envelope.Result != 0 {
		return &pcError{Result: envelope.Result, Msg: envelope.Error}
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// folderID converts a remote ID ("d5" or "" root) to the numeric param.
func folderID(id string) string {
	if id == "" {
		return "0"
	}
	return strings.TrimPrefix(id, "d")
}

// numeric extracts the id behind an f/d prefix.
func numeric(id string) (string, string, error) {
	switch {
	case strings.HasPrefix(id, "f"):
		return "fileid", strings.TrimPrefix(id, "f"), nil
	case strings.HasPrefix(id, "d"):
		return "folderid", strings.TrimPrefix(id, "d"), nil
	default:
		return "", "", fmt.Errorf("pcloud: unrecognized id %q", id)
	}
}

// ---- model ----

type pcEntry struct {
	IsFolder     bool      `json:"isfolder"`
	FileID       int64     `json:"fileid"`
	FolderID     int64     `json:"folderid"`
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	ParentFolder int64     `json:"parentfolderid"`
	ContentType  string    `json:"contenttype"`
	Modified     string    `json:"modified"`
	Contents     []pcEntry `json:"contents"`
}

func toFile(e pcEntry) provider.File {
	id, parent := "", ""
	if e.IsFolder {
		id = "d" + strconv.FormatInt(e.FolderID, 10)
	} else {
		id = "f" + strconv.FormatInt(e.FileID, 10)
	}
	if e.ParentFolder != 0 {
		parent = "d" + strconv.FormatInt(e.ParentFolder, 10)
	}
	mt, _ := time.Parse(httpDate, e.Modified)
	return provider.File{
		RemoteID: id,
		ParentID: parent,
		Name:     e.Name,
		IsDir:    e.IsFolder,
		Size:     e.Size,
		MIME:     e.ContentType,
		ModTime:  mt,
	}
}

// ---- Connector ----

func (c *connector) List(ctx context.Context, acct provider.AccountRef, parentRemoteID, pageToken string) ([]provider.File, string, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return nil, "", err
	}
	q := url.Values{"folderid": {folderID(parentRemoteID)}}
	var out struct {
		Metadata pcEntry `json:"metadata"`
	}
	if err := call(ctx, hc, http.MethodGet, apiBase+"/listfolder?"+q.Encode(), &out); err != nil {
		return nil, "", err
	}
	files := make([]provider.File, 0, len(out.Metadata.Contents))
	for _, e := range out.Metadata.Contents {
		files = append(files, toFile(e))
	}
	return files, "", nil
}

func (c *connector) Quota(ctx context.Context, acct provider.AccountRef) (provider.Quota, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.Quota{}, err
	}
	var out struct {
		Quota     int64 `json:"quota"`
		UsedQuota int64 `json:"usedquota"`
	}
	if err := call(ctx, hc, http.MethodGet, apiBase+"/userinfo", &out); err != nil {
		return provider.Quota{}, err
	}
	return provider.Quota{TotalBytes: out.Quota, UsedBytes: out.UsedQuota}, nil
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
	if err := call(ctx, hc, http.MethodGet, apiBase+"/userinfo", &out); err != nil {
		return "", err
	}
	return out.Email, nil
}

// Changes: pCloud has no delta feed; recursive listing of the root is
// explicitly cheap per the docs, so it stands in for the initial index.
func (c *connector) Changes(ctx context.Context, acct provider.AccountRef, cursor string) (provider.Changes, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.Changes{}, err
	}
	q := url.Values{"folderid": {"0"}, "recursive": {"1"}}
	var out struct {
		Metadata pcEntry `json:"metadata"`
	}
	if err := call(ctx, hc, http.MethodGet, apiBase+"/listfolder?"+q.Encode(), &out); err != nil {
		return provider.Changes{}, err
	}
	var all []provider.File
	var walk func(entries []pcEntry)
	walk = func(entries []pcEntry) {
		for _, e := range entries {
			all = append(all, toFile(e))
			if e.IsFolder && len(e.Contents) > 0 {
				walk(e.Contents)
			}
		}
	}
	walk(out.Metadata.Contents)
	return provider.Changes{Cursor: "walk", Upserted: all}, nil
}

func (c *connector) Upload(ctx context.Context, acct provider.AccountRef, parentRemoteID, name string, r io.Reader, size int64, progress provider.ProgressFn) (provider.File, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.File{}, err
	}
	q := url.Values{
		"folderid": {folderID(parentRemoteID)},
		"filename": {name},
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/uploadfile?"+q.Encode(), &body)
	if err != nil {
		return provider.File{}, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := hc.Do(req)
	if err != nil {
		return provider.File{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var envelope struct {
		Result   int       `json:"result"`
		Error    string    `json:"error"`
		Metadata []pcEntry `json:"metadata"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return provider.File{}, err
	}
	if envelope.Result != 0 {
		return provider.File{}, &pcError{Result: envelope.Result, Msg: envelope.Error}
	}
	if len(envelope.Metadata) == 0 {
		return provider.File{}, fmt.Errorf("pcloud: upload returned no metadata")
	}
	return toFile(envelope.Metadata[0]), nil
}

func (c *connector) Open(ctx context.Context, acct provider.AccountRef, remoteID string, progress provider.ProgressFn) (io.ReadCloser, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return nil, err
	}
	key, id, err := numeric(remoteID)
	if err != nil {
		return nil, err
	}
	q := url.Values{key: {id}}
	var out struct {
		Hosts []string `json:"hosts"`
		Path  string   `json:"path"`
	}
	if err := call(ctx, hc, http.MethodGet, apiBase+"/getfilelink?"+q.Encode(), &out); err != nil {
		return nil, err
	}
	if len(out.Hosts) == 0 || out.Path == "" {
		return nil, fmt.Errorf("pcloud: empty getfilelink response")
	}
	// The link is host+path, valid until `expires`. Real responses carry
	// bare hostnames; tolerate absolute URLs too (tests, proxies).
	host := out.Hosts[0]
	if !strings.Contains(host, "://") {
		host = "https://" + host
	}
	resp, err := hc.Get(host + out.Path)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, &pcError{Result: resp.StatusCode, Msg: string(body)}
	}
	return resp.Body, nil
}

func (c *connector) Mkdir(ctx context.Context, acct provider.AccountRef, parentRemoteID, name string) (provider.File, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.File{}, err
	}
	q := url.Values{"folderid": {folderID(parentRemoteID)}, "name": {name}}
	var out struct {
		Metadata pcEntry `json:"metadata"`
	}
	if err := call(ctx, hc, http.MethodGet, apiBase+"/createfolder?"+q.Encode(), &out); err != nil {
		return provider.File{}, err
	}
	return toFile(out.Metadata), nil
}

func (c *connector) Move(ctx context.Context, acct provider.AccountRef, remoteID, newParentRemoteID, newName string) (provider.File, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.File{}, err
	}
	key, id, err := numeric(remoteID)
	if err != nil {
		return provider.File{}, err
	}
	method := "renamefile"
	if strings.HasPrefix(remoteID, "d") {
		method = "renamefolder"
	}
	q := url.Values{key: {id}}
	if newParentRemoteID != "" {
		q.Set("tofolderid", folderID(newParentRemoteID))
	}
	if newName != "" {
		q.Set("toname", newName)
	}
	var out struct {
		Metadata pcEntry `json:"metadata"`
	}
	if err := call(ctx, hc, http.MethodGet, apiBase+"/"+method+"?"+q.Encode(), &out); err != nil {
		return provider.File{}, err
	}
	return toFile(out.Metadata), nil
}

func (c *connector) Copy(ctx context.Context, acct provider.AccountRef, remoteID, newParentRemoteID, newName string) (provider.File, error) {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return provider.File{}, err
	}
	key, id, err := numeric(remoteID)
	if err != nil {
		return provider.File{}, err
	}
	q := url.Values{key: {id}}
	if newParentRemoteID != "" {
		q.Set("tofolderid", folderID(newParentRemoteID))
	}
	if newName != "" {
		q.Set("toname", newName)
	}
	var out struct {
		Metadata pcEntry `json:"metadata"`
	}
	if err := call(ctx, hc, http.MethodGet, apiBase+"/copyfile?"+q.Encode(), &out); err != nil {
		// Folder copies exist too, but are rarer; retry with copyfolder.
		if strings.HasPrefix(remoteID, "d") {
			q.Del(key)
			q.Set("folderid", id)
			if err2 := call(ctx, hc, http.MethodGet, apiBase+"/copyfolder?"+q.Encode(), &out); err2 != nil {
				return provider.File{}, err2
			}
			return toFile(out.Metadata), nil
		}
		return provider.File{}, err
	}
	return toFile(out.Metadata), nil
}

func (c *connector) Delete(ctx context.Context, acct provider.AccountRef, remoteID string) error {
	hc, err := c.client(ctx, acct)
	if err != nil {
		return err
	}
	key, id, err := numeric(remoteID)
	if err != nil {
		return err
	}
	method := "deletefile"
	if strings.HasPrefix(remoteID, "d") {
		method = "deletefolder"
	}
	q := url.Values{key: {id}}
	return call(ctx, hc, http.MethodGet, apiBase+"/"+method+"?"+q.Encode(), nil)
}

func (c *connector) ShareLink(ctx context.Context, acct provider.AccountRef, remoteID string, create bool) (string, error) {
	if !create {
		// Revocation needs the linkid of an existing publink; there is no
		// pinned list method, so revoke is unsupported until we pin
		// deletepublink plumbing.
		return "", provider.ErrUnsupported
	}
	hc, err := c.client(ctx, acct)
	if err != nil {
		return "", err
	}
	key, id, err := numeric(remoteID)
	if err != nil {
		return "", err
	}
	q := url.Values{key: {id}}
	var out struct {
		Link string `json:"link"`
	}
	if err := call(ctx, hc, http.MethodGet, apiBase+"/getfilepublink?"+q.Encode(), &out); err != nil {
		return "", err
	}
	return out.Link, nil
}
