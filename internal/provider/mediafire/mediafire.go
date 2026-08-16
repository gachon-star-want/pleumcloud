// Package mediafire implements the MediaFire connector over the Core API
// (docs pinned from mediafire.com/developers, API 1.1/1.5). Auth uses a
// session token v1 (no per-call signatures): login signature is
// SHA1(email+password+application_id+api_key); tokens last ~10 minutes.
package mediafire

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gachon-star-want/pleumcloud/internal/provider"
	"github.com/gachon-star-want/pleumcloud/internal/secret"
)

// apiBase is a var so tests can point it at a fake server.
var apiBase = "https://www.mediafire.com/api/1.5"

func init() {
	provider.RegisterFactory("mediafire", New)
}

type connector struct{ secrets secret.Store }

// New builds the MediaFire connector.
func New(deps provider.Deps) provider.Connector { return &connector{secrets: deps.Secrets} }

func (c *connector) Metadata() provider.Metadata {
	return provider.Metadata{
		ID: "mediafire", Name: "MediaFire", AuthKind: provider.AuthPAT,
		Tier: provider.TierNative, FreeTierGB: 10,
		DocsURL: "https://www.mediafire.com/developers/",
	}
}

type credBundle struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	AppID    string `json:"appId"`
	APIKey   string `json:"apiKey"`
}

func (c *connector) creds(acct provider.AccountRef) (credBundle, error) {
	var cb credBundle
	if err := secret.GetJSON(c.secrets, acct.SecretRef, &cb); err != nil {
		return cb, fmt.Errorf("load credentials: %w", err)
	}
	if cb.Email == "" || cb.Password == "" || cb.AppID == "" || cb.APIKey == "" {
		return cb, fmt.Errorf("mediafire: email, password, app id and api key required")
	}
	return cb, nil
}

// loginSignature = SHA1(email + password + application_id + api_key).
func loginSignature(email, password, appID, apiKey string) string {
	h := sha1.Sum([]byte(email + password + appID + apiKey))
	return hex.EncodeToString(h[:])
}

// ---- session handling ----

type mfError struct {
	Code int
	Msg  string
}

func (e *mfError) Error() string { return fmt.Sprintf("mediafire: error %d: %s", e.Code, e.Msg) }

type sessionCache struct {
	mu    sync.Mutex
	token string
	from  time.Time
}

var sessions sync.Map // AccountRef.ID → *sessionCache

func (c *connector) session(ctx context.Context, acct provider.AccountRef) (string, error) {
	cb, err := c.creds(acct)
	if err != nil {
		return "", err
	}
	if sc, ok := sessions.Load(acct.ID); ok {
		cache := sc.(*sessionCache)
		cache.mu.Lock()
		fresh := cache.token != "" && time.Since(cache.from) < 8*time.Minute
		cache.mu.Unlock()
		if fresh {
			return cache.token, nil
		}
	}
	q := url.Values{
		"email":           {cb.Email},
		"password":        {cb.Password},
		"application_id":  {cb.AppID},
		"signature":       {loginSignature(cb.Email, cb.Password, cb.AppID, cb.APIKey)},
		"token_version":   {"1"},
		"response_format": {"json"},
	}
	var out struct {
		Response struct {
			Result       string `json:"result"`
			Message      string `json:"message"`
			SessionToken string `json:"session_token"`
		} `json:"response"`
	}
	if err := c.get(ctx, apiBase+"/user/get_session_token.php?"+q.Encode(), nil, &out); err != nil {
		return "", err
	}
	if out.Response.SessionToken == "" {
		return "", &mfError{Msg: "login failed: " + out.Response.Message}
	}
	sc, _ := sessions.LoadOrStore(acct.ID, &sessionCache{})
	cache := sc.(*sessionCache)
	cache.mu.Lock()
	cache.token, cache.from = out.Response.SessionToken, time.Now()
	cache.mu.Unlock()
	return out.Response.SessionToken, nil
}

// get performs a GET (ops add session_token + json format).
func (c *connector) get(ctx context.Context, u string, tok *string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 500 {
		return &mfError{Code: resp.StatusCode, Msg: strings.TrimSpace(string(raw))}
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("mediafire: decode: %w", err)
		}
	}
	return nil
}

// call runs an authenticated op and decodes {response:{...}} envelopes.
func (c *connector) call(ctx context.Context, acct provider.AccountRef, path string, params url.Values, out any) error {
	tok, err := c.session(ctx, acct)
	if err != nil {
		return err
	}
	params.Set("session_token", tok)
	params.Set("response_format", "json")
	var envelope struct {
		Response json.RawMessage `json:"response"`
	}
	if err := c.get(ctx, apiBase+path+"?"+params.Encode(), &tok, &envelope); err != nil {
		return err
	}
	var head struct {
		Result  string `json:"result"`
		Message string `json:"message"`
		Error   int    `json:"error"`
	}
	_ = json.Unmarshal(envelope.Response, &head)
	if head.Result == "Error" {
		return &mfError{Code: head.Error, Msg: head.Message}
	}
	if out != nil {
		if err := json.Unmarshal(envelope.Response, out); err != nil {
			return fmt.Errorf("mediafire: decode op: %w", err)
		}
	}
	return nil
}

// ---- model ----

type mfFile struct {
	QuickKey string `json:"quickkey"`
	Filename string `json:"filename"`
	Size     string `json:"size"`
	MIME     string `json:"mimetype"`
	Created  string `json:"created"`
}

type mfFolder struct {
	FolderKey string `json:"folderkey"`
	Name      string `json:"name"`
	Created   string `json:"created"`
}

func fileToFile(parent string, f mfFile) provider.File {
	n, _ := strconv.ParseInt(f.Size, 10, 64)
	return provider.File{RemoteID: f.QuickKey, ParentID: parent, Name: f.Filename, Size: n, MIME: f.MIME}
}

func folderToFile(parent string, f mfFolder) provider.File {
	return provider.File{RemoteID: f.FolderKey, ParentID: parent, Name: f.Name, IsDir: true}
}

// ---- Connector ----

func (c *connector) List(ctx context.Context, acct provider.AccountRef, parentRemoteID, pageToken string) ([]provider.File, string, error) {
	chunk := pageToken
	if chunk == "" {
		chunk = "1"
	}
	list := func(contentType string) (files []provider.File, more bool, err error) {
		q := url.Values{"content_type": {contentType}, "chunk": {chunk}}
		if parentRemoteID != "" {
			q.Set("folder_key", parentRemoteID)
		}
		var out struct {
			FolderContent struct {
				Files      []mfFile   `json:"files"`
				Folders    []mfFolder `json:"folders"`
				MoreChunks string     `json:"more_chunks"`
			} `json:"folder_content"`
		}
		if err := c.call(ctx, acct, "/folder/get_content.php", q, &out); err != nil {
			return nil, false, err
		}
		for _, f := range out.FolderContent.Folders {
			files = append(files, folderToFile(parentRemoteID, f))
		}
		for _, f := range out.FolderContent.Files {
			files = append(files, fileToFile(parentRemoteID, f))
		}
		return files, out.FolderContent.MoreChunks == "yes", nil
	}

	dirs, moreD, err := list("folders")
	if err != nil {
		return nil, "", err
	}
	files, moreF, err := list("files")
	if err != nil {
		return nil, "", err
	}
	all := append(dirs, files...)
	next := ""
	if moreD || moreF {
		n, _ := strconv.Atoi(chunk)
		next = strconv.Itoa(n + 1)
	}
	return all, next, nil
}

func (c *connector) Quota(ctx context.Context, acct provider.AccountRef) (provider.Quota, error) {
	var out struct {
		UserInfo struct {
			UsedStorageSize string `json:"used_storage_size"`
			StorageLimit    string `json:"storage_limit"`
		} `json:"user_info"`
	}
	if err := c.call(ctx, acct, "/user/get_info.php", url.Values{}, &out); err != nil {
		return provider.Quota{}, err
	}
	used, _ := strconv.ParseInt(out.UserInfo.UsedStorageSize, 10, 64)
	limit, _ := strconv.ParseInt(out.UserInfo.StorageLimit, 10, 64)
	return provider.Quota{TotalBytes: limit, UsedBytes: used}, nil
}

// AccountLabel returns the account email for display.
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
	tok, err := c.session(ctx, acct)
	if err != nil {
		return provider.File{}, err
	}
	q := url.Values{"action_on_duplicate": {"replace"}}
	if parentRemoteID != "" {
		q.Set("folder_key", parentRemoteID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/upload/simple.php?"+q.Encode(), r)
	if err != nil {
		return provider.File{}, err
	}
	req.Header.Set("X-Filesize", strconv.FormatInt(size, 10))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	req.URL.RawQuery = req.URL.RawQuery + "&session_token=" + url.QueryEscape(tok) + "&response_format=json"

	resp, err := (&http.Client{Timeout: 30 * time.Minute}).Do(req)
	if err != nil {
		return provider.File{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var up struct {
		Response struct {
			Doupload struct {
				Result string `json:"result"`
				Key    string `json:"key"`
			} `json:"doupload"`
		} `json:"response"`
	}
	if err := json.Unmarshal(raw, &up); err != nil || up.Response.Doupload.Key == "" {
		return provider.File{}, fmt.Errorf("mediafire: upload start failed: %s", strings.TrimSpace(string(raw)))
	}
	if progress != nil {
		progress(size, size)
	}

	// Poll until the upload settles (status 99).
	for i := 0; i < 60; i++ {
		// c.call already unwraps the {response:...} envelope.
		var poll struct {
			DoUpload struct {
				Status    string `json:"status"`
				QuickKey  string `json:"quickkey"`
				Fileerror int    `json:"fileerror"`
			} `json:"do_upload"`
		}
		if err := c.call(ctx, acct, "/upload/poll_upload.php", url.Values{"key": {up.Response.Doupload.Key}}, &poll); err != nil {
			return provider.File{}, err
		}
		if poll.DoUpload.Status == "99" {
			return provider.File{RemoteID: poll.DoUpload.QuickKey, ParentID: parentRemoteID, Name: name, Size: size}, nil
		}
		if poll.DoUpload.Fileerror != 0 && poll.DoUpload.Status == "-80" {
			return provider.File{}, &mfError{Code: poll.DoUpload.Fileerror, Msg: "upload rejected"}
		}
		time.Sleep(2 * time.Second)
	}
	return provider.File{}, fmt.Errorf("mediafire: upload poll timed out")
}

func (c *connector) Open(ctx context.Context, acct provider.AccountRef, remoteID string, progress provider.ProgressFn) (io.ReadCloser, error) {
	var out struct {
		Links []struct {
			DirectDownload string `json:"direct_download"`
		} `json:"links"`
	}
	q := url.Values{"quick_key": {remoteID}, "link_type": {"direct_download"}}
	if err := c.call(ctx, acct, "/file/get_links.php", q, &out); err != nil {
		return nil, err
	}
	if len(out.Links) == 0 || out.Links[0].DirectDownload == "" {
		return nil, fmt.Errorf("mediafire: no direct download link")
	}
	resp, err := (&http.Client{Timeout: 0}).Get(out.Links[0].DirectDownload)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, &mfError{Code: resp.StatusCode, Msg: string(body)}
	}
	return resp.Body, nil
}

func (c *connector) Mkdir(ctx context.Context, acct provider.AccountRef, parentRemoteID, name string) (provider.File, error) {
	q := url.Values{"foldername": {name}}
	if parentRemoteID != "" {
		q.Set("parent_key", parentRemoteID)
	}
	var out struct {
		FolderKey string `json:"folder_key"`
		Name      string `json:"name"`
	}
	if err := c.call(ctx, acct, "/folder/create.php", q, &out); err != nil {
		return provider.File{}, err
	}
	return provider.File{RemoteID: out.FolderKey, ParentID: parentRemoteID, Name: name, IsDir: true}, nil
}

func (c *connector) Move(ctx context.Context, acct provider.AccountRef, remoteID, newParentRemoteID, newName string) (provider.File, error) {
	isFolder := strings.HasPrefix(remoteID, "fk")
	if newName != "" {
		path, key := "/file/update.php", "quick_key"
		q := url.Values{"filename": {newName}}
		if isFolder {
			path, key = "/folder/update.php", "folder_key"
		}
		q.Set(key, remoteID)
		if err := c.call(ctx, acct, path, q, nil); err != nil {
			return provider.File{}, err
		}
	}
	if newParentRemoteID != "" {
		if isFolder {
			q := url.Values{"folder_key_src": {remoteID}, "folder_key_dst": {newParentRemoteID}}
			if err := c.call(ctx, acct, "/folder/move.php", q, nil); err != nil {
				return provider.File{}, err
			}
		} else {
			q := url.Values{"quick_key": {remoteID}, "folder_key": {newParentRemoteID}}
			if err := c.call(ctx, acct, "/file/move.php", q, nil); err != nil {
				return provider.File{}, err
			}
		}
	}
	name := newName
	if name == "" {
		name = remoteID
	}
	return provider.File{RemoteID: remoteID, ParentID: newParentRemoteID, Name: name}, nil
}

func (c *connector) Copy(ctx context.Context, acct provider.AccountRef, remoteID, newParentRemoteID, newName string) (provider.File, error) {
	isFolder := strings.HasPrefix(remoteID, "fk")
	var out struct {
		NewQuickKey  string `json:"new_quickkey"`
		NewFolderKey string `json:"new_folderkey"`
	}
	if isFolder {
		q := url.Values{"folder_key_src": {remoteID}}
		if newParentRemoteID != "" {
			q.Set("folder_key_dst", newParentRemoteID)
		}
		if err := c.call(ctx, acct, "/folder/copy.php", q, &out); err != nil {
			return provider.File{}, err
		}
		return provider.File{RemoteID: out.NewFolderKey, ParentID: newParentRemoteID, IsDir: true}, nil
	}
	q := url.Values{"quick_key": {remoteID}}
	if newParentRemoteID != "" {
		q.Set("folder_key", newParentRemoteID)
	}
	if err := c.call(ctx, acct, "/file/copy.php", q, &out); err != nil {
		return provider.File{}, err
	}
	return provider.File{RemoteID: out.NewQuickKey, ParentID: newParentRemoteID}, nil
}

func (c *connector) Delete(ctx context.Context, acct provider.AccountRef, remoteID string) error {
	if strings.HasPrefix(remoteID, "fk") {
		return c.call(ctx, acct, "/folder/delete.php", url.Values{"folder_key": {remoteID}}, nil)
	}
	return c.call(ctx, acct, "/file/delete.php", url.Values{"quick_key": {remoteID}}, nil)
}

// ShareLink hands out the public view link (revocation unsupported by API).
func (c *connector) ShareLink(ctx context.Context, acct provider.AccountRef, remoteID string, create bool) (string, error) {
	if !create {
		return "", provider.ErrUnsupported
	}
	var out struct {
		Links []struct {
			View string `json:"view"`
		} `json:"links"`
	}
	q := url.Values{"quick_key": {remoteID}, "link_type": {"view"}}
	if err := c.call(ctx, acct, "/file/get_links.php", q, &out); err != nil {
		return "", err
	}
	if len(out.Links) == 0 {
		return "", fmt.Errorf("mediafire: no view link")
	}
	return out.Links[0].View, nil
}
