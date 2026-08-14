package koofr

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pleumcloud/pleumcloud/internal/provider"
	"github.com/pleumcloud/pleumcloud/internal/secret"
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
	_ = s.Set("test", []byte(`{"email":"me@koofr.net","pat":"tok"}`))
	return &connector{secrets: s}, provider.AccountRef{ID: "a1", ProviderID: "koofr", SecretRef: "test"}
}

func finfo(name, typ, path string, size int64, modified int64) map[string]any {
	return map[string]any{"name": name, "type": typ, "path": path, "size": size,
		"modified": modified, "contentType": "image/jpeg"}
}

// The connector resolves the primary mount once, then works in paths.
func TestListAndQuotaAndLabel(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "me@koofr.net" || pass != "tok" {
			t.Errorf("basic auth = %q:%q", user, pass)
		}
		switch r.URL.Path {
		case "/api/v2/mounts":
			_ = json.NewEncoder(w).Encode(map[string]any{"mounts": []map[string]any{
				{"id": "shared", "name": "Shared", "origin": "dropbox", "spaceTotal": 0, "spaceUsed": 0},
				{"id": "m1", "name": "Koofr", "origin": "koofr", "spaceTotal": 10485760, "spaceUsed": 1048576,
					"owner": map[string]any{"email": "me@koofr.net"}},
			}})
		case "/api/v2/mounts/m1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "m1", "spaceTotal": 10485760, "spaceUsed": 1048576,
				"owner": map[string]any{"email": "me@koofr.net"},
			})
		case "/api/v2/mounts/m1/files/list":
			if r.URL.Query().Get("path") != "/" {
				t.Fatalf("path = %q", r.URL.Query().Get("path"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{
					finfo("Docs", "dir", "/Docs", 0, 1755120000),
					finfo("a.jpg", "file", "/a.jpg", 42, 1755120000),
				},
			})
		case "/api/v2/user":
			_ = json.NewEncoder(w).Encode(map[string]any{"email": "me@koofr.net"})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	apiBase = srv.URL

	files, next, err := c.List(context.Background(), acct, "", "")
	if err != nil || next != "" {
		t.Fatalf("err=%v next=%q", err, next)
	}
	if len(files) != 2 || !files[0].IsDir || files[0].RemoteID != "Docs" || files[1].Size != 42 {
		t.Fatalf("files=%+v", files)
	}
	// Quota comes from the primary mount details.
	q, err := c.Quota(context.Background(), acct)
	if err != nil || q.TotalBytes != 10485760 || q.UsedBytes != 1048576 {
		t.Fatalf("quota=%+v err=%v", q, err)
	}
	label, err := c.AccountLabel(context.Background(), acct)
	if err != nil || label != "me@koofr.net" {
		t.Fatalf("label=%q err=%v", label, err)
	}
}

func TestUploadAndDownload(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/mounts":
			_ = json.NewEncoder(w).Encode(map[string]any{"mounts": []map[string]any{
				{"id": "m1", "origin": "koofr", "spaceTotal": 1, "spaceUsed": 0},
			}})
		case "/content/api/v2/mounts/m1/files/put":
			q := r.URL.Query()
			if q.Get("path") != "/Docs" || q.Get("filename") != "up.bin" || q.Get("info") != "true" {
				t.Fatalf("put query = %s", r.URL.RawQuery)
			}
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				t.Fatalf("content-type = %s", r.Header.Get("Content-Type"))
			}
			b, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(b), "hello") {
				t.Fatalf("body missing bytes")
			}
			_ = json.NewEncoder(w).Encode(finfo("up.bin", "file", "/Docs/up.bin", 5, 1755120000))
		case "/content/api/v2/mounts/m1/files/get":
			if r.URL.Query().Get("path") != "/a.bin" {
				t.Fatalf("get path = %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte("content"))
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	apiBase = srv.URL
	ctx := context.Background()

	f, err := c.Upload(ctx, acct, "Docs", "up.bin", strings.NewReader("hello"), 5, nil)
	if err != nil || f.RemoteID != "Docs/up.bin" || f.ParentID != "Docs" {
		t.Fatalf("upload=%+v err=%v", f, err)
	}
	rc, err := c.Open(ctx, acct, "a.bin", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if string(b) != "content" {
		t.Fatalf("content=%q", b)
	}
}

func TestMutations(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/mounts" {
			_ = json.NewEncoder(w).Encode(map[string]any{"mounts": []map[string]any{
				{"id": "m1", "origin": "koofr", "spaceTotal": 1, "spaceUsed": 0},
			}})
			return
		}
		var body map[string]any
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		switch {
		case r.URL.Path == "/api/v2/mounts/m1/files/folder":
			q := r.URL.Query()
			if q.Get("path") != "/Docs" || q.Get("name") != "New" {
				t.Fatalf("folder query = %s", r.URL.RawQuery)
			}
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/v2/mounts/m1/files/move":
			if r.URL.Query().Get("path") != "/a.txt" || body["toMountId"] != "m1" || body["toPath"] != "/Docs/b.txt" {
				t.Fatalf("move = %s %v", r.URL.RawQuery, body)
			}
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/v2/mounts/m1/files/copy":
			if r.URL.Query().Get("path") != "/a.txt" || body["toPath"] != "/Docs/c.txt" {
				t.Fatalf("copy = %s %v", r.URL.RawQuery, body)
			}
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/v2/mounts/m1/files/remove":
			if r.URL.Query().Get("path") != "/Docs/x" {
				t.Fatalf("remove = %s", r.URL.RawQuery)
			}
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/v2/mounts/m1/files/info":
			_ = json.NewEncoder(w).Encode(finfo("b.txt", "file", "/Docs/b.txt", 3, 1755120000))
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	apiBase = srv.URL
	ctx := context.Background()

	if d, err := c.Mkdir(ctx, acct, "Docs", "New"); err != nil || !d.IsDir || d.RemoteID != "Docs/New" {
		t.Fatalf("mkdir=%+v err=%v", d, err)
	}
	if m, err := c.Move(ctx, acct, "a.txt", "Docs", "b.txt"); err != nil || m.RemoteID != "Docs/b.txt" {
		t.Fatalf("move=%+v err=%v", m, err)
	}
	if _, err := c.Copy(ctx, acct, "a.txt", "Docs", "c.txt"); err != nil {
		t.Fatalf("copy err=%v", err)
	}
	if err := c.Delete(ctx, acct, "Docs/x"); err != nil {
		t.Fatalf("delete err=%v", err)
	}
	if _, err := c.ShareLink(ctx, acct, "a.txt", true); err != provider.ErrUnsupported {
		t.Fatalf("share err=%v", err)
	}
}
