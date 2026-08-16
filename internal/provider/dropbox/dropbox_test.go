package dropbox

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"

	"github.com/gachon-star-want/pleumcloud/internal/provider"
	"github.com/gachon-star-want/pleumcloud/internal/secret"
)

type memSecret map[string][]byte

func (m memSecret) Set(ref string, data []byte) error { m[ref] = data; return nil }
func (m memSecret) Get(ref string) ([]byte, error) {
	d, ok := m[ref]
	if !ok {
		return nil, secret.ErrNotFound
	}
	return d, nil
}
func (m memSecret) Delete(ref string) error { delete(m, ref); return nil }

func fakeAccount(s secret.Store) (*connector, provider.AccountRef) {
	tok, _ := json.Marshal(map[string]any{"token": oauth2.Token{AccessToken: "t"}})
	_ = s.Set("test", tok)
	return &connector{secrets: s}, provider.AccountRef{ID: "a1", ProviderID: "dropbox", SecretRef: "test"}
}

func entry(name string, isDir bool, size int64) map[string]any {
	tag := "file"
	if isDir {
		tag = "folder"
	}
	e := map[string]any{
		".tag":            tag,
		"name":            name,
		"path_lower":      "/" + strings.ToLower(name),
		"id":              "id:" + name,
		"server_modified": "2026-08-14T00:00:00Z",
	}
	if !isDir {
		e["size"] = size
	}
	return e
}

// TestListAndPagination drives list_folder + list_folder/continue.
func TestListAndPagination(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer t" {
			t.Errorf("auth = %q", got)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/2/files/list_folder":
			if body["path"] != "" {
				t.Fatalf("root path = %v", body["path"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"entries":  []map[string]any{entry("Docs", true, 0), entry("A.pdf", false, 42)},
				"cursor":   "cur1",
				"has_more": true,
			})
		case "/2/files/list_folder/continue":
			if body["cursor"] != "cur1" {
				t.Fatalf("continue cursor = %v", body["cursor"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"entries":  []map[string]any{entry("B.txt", false, 7)},
				"has_more": false,
			})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	rpcBase = srv.URL

	files, next, err := c.List(context.Background(), acct, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || !files[0].IsDir || files[0].RemoteID != "docs" || files[1].Size != 42 {
		t.Fatalf("files = %+v", files)
	}
	if next != "cur1" {
		t.Fatalf("next = %q", next)
	}
	files2, next2, err := c.List(context.Background(), acct, "", next)
	if err != nil || len(files2) != 1 || next2 != "" {
		t.Fatalf("page2 = %+v %q err=%v", files2, next2, err)
	}
}

func TestQuotaAndLabel(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2/users/get_space_usage":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"used":       1000000000,
				"allocation": map[string]any{".tag": "individual", "allocated": 2147483648},
			})
		case "/2/users/get_current_account":
			_ = json.NewEncoder(w).Encode(map[string]any{"email": "me@dropbox.com"})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	rpcBase = srv.URL

	q, err := c.Quota(context.Background(), acct)
	if err != nil || q.TotalBytes != 2147483648 || q.UsedBytes != 1000000000 {
		t.Fatalf("quota=%+v err=%v", q, err)
	}
	label, err := c.AccountLabel(context.Background(), acct)
	if err != nil || label != "me@dropbox.com" {
		t.Fatalf("label=%q err=%v", label, err)
	}
}

// TestChangesRecursiveWalk: initial recursive list returns the full tree
// and a cursor; the next call reports changes including deletions.
func TestChangesRecursiveWalk(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Path != "/2/files/list_folder" && r.URL.Path != "/2/files/list_folder/continue" {
			t.Fatalf("unexpected %s", r.URL.Path)
		}
		if r.URL.Path == "/2/files/list_folder" && body["recursive"] != true {
			t.Fatalf("recursive = %v", body["recursive"])
		}
		if body["cursor"] == "walk" || body["cursor"] == "next1" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"entries": []map[string]any{
					{".tag": "deleted", "name": "Gone", "path_lower": "/gone"},
				},
				"cursor":   "later",
				"has_more": false,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"entries":  []map[string]any{entry("Docs", true, 0), entry("Docs/A.pdf", false, 9)},
			"cursor":   "walk",
			"has_more": false,
		})
	}))
	defer srv.Close()
	rpcBase = srv.URL

	ch, err := c.Changes(context.Background(), acct, "")
	if err != nil || len(ch.Upserted) != 2 || ch.Cursor != "walk" {
		t.Fatalf("initial = %+v err=%v", ch, err)
	}
	if ch.Upserted[1].ParentID != "docs" {
		t.Fatalf("parent mapping = %+v", ch.Upserted[1])
	}
	ch, err = c.Changes(context.Background(), acct, "walk")
	if err != nil || len(ch.Deleted) != 1 || ch.Deleted[0] != "gone" {
		t.Fatalf("delta = %+v err=%v", ch, err)
	}
}

func TestUploadAndDownload(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected rpc %s %s", r.Method, r.URL.Path)
	}))
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2/files/upload":
			arg := r.Header.Get("Dropbox-API-Arg")
			if !strings.Contains(arg, `"path":"/docs/up.bin"`) || !strings.Contains(arg, `"mode":{".tag":"overwrite"}`) {
				t.Fatalf("upload arg = %s", arg)
			}
			b, _ := io.ReadAll(r.Body)
			if string(b) != "hello" {
				t.Fatalf("body = %q", b)
			}
			_ = json.NewEncoder(w).Encode(entry("docs/up.bin", false, 5))
		case "/2/files/download":
			arg := r.Header.Get("Dropbox-API-Arg")
			if !strings.Contains(arg, `"path":"/docs/a.bin"`) {
				t.Fatalf("download arg = %s", arg)
			}
			_, _ = w.Write([]byte("content"))
		default:
			t.Fatalf("unexpected content %s %s", r.Method, r.URL.Path)
		}
	}))
	defer rpc.Close()
	defer content.Close()
	rpcBase = rpc.URL
	contentBase = content.URL

	f, err := c.Upload(context.Background(), acct, "docs", "up.bin", strings.NewReader("hello"), 5, nil)
	if err != nil || f.RemoteID != "docs/up.bin" || f.ParentID != "docs" {
		t.Fatalf("upload = %+v err=%v", f, err)
	}
	rc, err := c.Open(context.Background(), acct, "docs/a.bin", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if string(b) != "content" {
		t.Fatalf("content = %q", b)
	}
}

func TestMutationsAndShareLink(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	var shared []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		// list_shared_links returns {links:[{url}...]}
		if r.URL.Path == "/2/sharing/list_shared_links" {
			type linkJSON struct {
				URL string `json:"url"`
			}
			links := make([]linkJSON, 0, len(shared))
			for _, u := range shared {
				links = append(links, linkJSON{URL: u})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"links": links})
			return
		}
		switch r.URL.Path {
		case "/2/files/create_folder_v2":
			if body["path"] != "/New" {
				t.Fatalf("mkdir = %v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metadata": entry("New", true, 0)})
		case "/2/files/move_v2":
			if body["from_path"] != "/a.txt" || body["to_path"] != "/docs/b.txt" {
				t.Fatalf("move = %v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metadata": entry("docs/b.txt", false, 3)})
		case "/2/files/copy_v2":
			if body["from_path"] != "/a.txt" || body["to_path"] != "/docs/c.txt" {
				t.Fatalf("copy = %v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metadata": entry("docs/c.txt", false, 3)})
		case "/2/files/delete_v2":
			if body["path"] != "/docs/x" {
				t.Fatalf("delete = %v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metadata": entry("docs/x", false, 0)})
		case "/2/sharing/create_shared_link_with_settings":
			if body["path"] != "/docs/a.txt" {
				t.Fatalf("share = %v", body)
			}
			shared = append(shared, "https://db.sh/x")
			_ = json.NewEncoder(w).Encode(map[string]any{"url": "https://db.sh/x"})
		case "/2/sharing/revoke_shared_link":
			if body["url"] != "https://db.sh/x" {
				t.Fatalf("revoke = %v", body)
			}
			if len(shared) > 0 {
				shared = shared[1:]
			}
			_ = json.NewEncoder(w).Encode(map[string]any{})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	rpcBase = srv.URL
	contentBase = srv.URL
	ctx := context.Background()

	if d, err := c.Mkdir(ctx, acct, "", "New"); err != nil || !d.IsDir || d.RemoteID != "new" {
		t.Fatalf("mkdir=%+v err=%v", d, err)
	}
	if m, err := c.Move(ctx, acct, "a.txt", "docs", "b.txt"); err != nil || m.RemoteID != "docs/b.txt" {
		t.Fatalf("move=%+v err=%v", m, err)
	}
	if cp, err := c.Copy(ctx, acct, "a.txt", "docs", "c.txt"); err != nil || cp.RemoteID != "docs/c.txt" {
		t.Fatalf("copy=%+v err=%v", cp, err)
	}
	if err := c.Delete(ctx, acct, "docs/x"); err != nil {
		t.Fatalf("delete err=%v", err)
	}
	link, err := c.ShareLink(ctx, acct, "docs/a.txt", true)
	if err != nil || link != "https://db.sh/x" || len(shared) != 1 {
		t.Fatalf("link=%q shared=%v err=%v", link, shared, err)
	}
	if _, err := c.ShareLink(ctx, acct, "docs/a.txt", false); err != nil || len(shared) != 0 {
		t.Fatalf("revoke err=%v shared=%v", err, shared)
	}
}
