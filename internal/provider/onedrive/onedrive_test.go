package onedrive

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
	return &connector{secrets: s}, provider.AccountRef{ID: "a1", ProviderID: "onedrive", SecretRef: "test"}
}

// item builds a Graph drive item.
func item(id, name, parent string, isDir bool, size int64) map[string]any {
	m := map[string]any{
		"id": id, "name": name, "size": size,
		"lastModifiedDateTime": "2026-08-14T00:00:00Z",
		"parentReference":      map[string]any{"id": parent},
	}
	if isDir {
		m["folder"] = map[string]any{"childCount": 2}
	} else {
		m["file"] = map[string]any{"mimeType": "application/pdf"}
	}
	return m
}

func TestListRootAndFolder(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer t" {
			t.Errorf("auth = %q", got)
		}
		switch r.URL.Path {
		case "/me/drive/root/children":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"value":           []map[string]any{item("d1", "Docs", "root", true, 0), item("f1", "a.pdf", "root", false, 42)},
				"@odata.nextLink": "https://example.invalid/next?$skiptoken=PAGE2",
			})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	graphBase = srv.URL

	files, next, err := c.List(context.Background(), acct, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || !files[0].IsDir || files[0].RemoteID != "d1" || files[1].Size != 42 {
		t.Fatalf("files = %+v", files)
	}
	if next != "PAGE2" {
		t.Fatalf("next = %q (want skiptoken extracted from nextLink)", next)
	}
}

func TestListSubfolderUsesItemsPath(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/me/drive/items/d1/children" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"value": []map[string]any{item("f2", "b.txt", "d1", false, 7)},
			})
			return
		}
		t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()
	graphBase = srv.URL

	files, _, err := c.List(context.Background(), acct, "d1", "")
	if err != nil || len(files) != 1 || files[0].RemoteID != "f2" {
		t.Fatalf("files=%+v err=%v", files, err)
	}
}

func TestQuotaAndLabel(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/me/drive":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"quota": map[string]any{"total": 5000000000, "used": 1000000000},
				"owner": map[string]any{"user": map[string]any{"email": "me@outlook.com"}},
			})
		case "/me":
			_ = json.NewEncoder(w).Encode(map[string]any{"userPrincipalName": "me@outlook.com"})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	graphBase = srv.URL

	q, err := c.Quota(context.Background(), acct)
	if err != nil || q.TotalBytes != 5000000000 || q.UsedBytes != 1000000000 {
		t.Fatalf("quota=%+v err=%v", q, err)
	}
	label, err := c.AccountLabel(context.Background(), acct)
	if err != nil || label != "me@outlook.com" {
		t.Fatalf("label=%q err=%v", label, err)
	}
}

func TestChangesDeltaFlow(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/delta") {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		switch r.URL.Query().Get("token") {
		case "":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"value":            []map[string]any{item("f1", "a.pdf", "root", false, 1)},
				"@odata.deltaLink": "https://example.invalid/delta?$deltatoken=D1",
			})
		case "D1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"value": []map[string]any{
					item("f2", "b.pdf", "root", false, 2),
					map[string]any{"id": "f1", "deleted": map[string]any{}},
				},
				"@odata.deltaLink": "https://example.invalid/delta?$deltatoken=D2",
			})
		default:
			t.Fatalf("unexpected token %q", r.URL.Query().Get("token"))
		}
	}))
	defer srv.Close()
	graphBase = srv.URL

	ch, err := c.Changes(context.Background(), acct, "")
	if err != nil || ch.Cursor != "D1" || len(ch.Upserted) != 1 {
		t.Fatalf("initial delta = %+v err=%v", ch, err)
	}
	ch, err = c.Changes(context.Background(), acct, "D1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ch.Deleted) != 1 || ch.Deleted[0] != "f1" || len(ch.Upserted) != 1 || ch.Cursor != "D2" {
		t.Fatalf("delta = %+v", ch)
	}
}

func TestUploadSimpleAndDownload(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/root:/up.bin:/content"):
			b, _ := io.ReadAll(r.Body)
			if string(b) != "hello" {
				t.Fatalf("upload body = %q", b)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(item("nf", "up.bin", "root", false, 5))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/f9/content"):
			_, _ = w.Write([]byte("content"))
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	graphBase = srv.URL

	f, err := c.Upload(context.Background(), acct, "", "up.bin", strings.NewReader("hello"), 5, nil)
	if err != nil || f.RemoteID != "nf" {
		t.Fatalf("upload = %+v err=%v", f, err)
	}
	rc, err := c.Open(context.Background(), acct, "f9", nil)
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
	var perms []map[string]any
	var linkID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/children"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["name"] != "New" {
				t.Fatalf("mkdir body = %v", body)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(item("nd", "New", "root", true, 0))
		case r.Method == http.MethodPatch:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["name"] != "renamed" {
				t.Fatalf("patch body = %v", body)
			}
			_ = json.NewEncoder(w).Encode(item("f1", "renamed", "d2", false, 3))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/copy"):
			if r.Header.Get("Content-Type") != "application/json" {
				t.Fatal("copy missing JSON")
			}
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/f1"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/createLink"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"link": map[string]any{"webUrl": "https://1drv.ms/x"},
			})
			perms = append(perms, map[string]any{"id": "p1", "link": map[string]any{"scope": "anonymous"}, "grantedToIdentities": nil})
			linkID = "p1"
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/permissions"):
			_ = json.NewEncoder(w).Encode(map[string]any{"value": perms})
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/permissions/"):
			if !strings.HasSuffix(r.URL.Path, "/"+linkID) {
				t.Fatalf("deleted wrong perm: %s", r.URL.Path)
			}
			perms = nil
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	graphBase = srv.URL
	ctx := context.Background()

	if d, err := c.Mkdir(ctx, acct, "", "New"); err != nil || !d.IsDir || d.RemoteID != "nd" {
		t.Fatalf("mkdir=%+v err=%v", d, err)
	}
	if m, err := c.Move(ctx, acct, "f1", "d2", "renamed"); err != nil || m.Name != "renamed" {
		t.Fatalf("move=%+v err=%v", m, err)
	}
	if _, err := c.Copy(ctx, acct, "f1", "d2", "copy.pdf"); err != nil {
		t.Fatalf("copy err=%v", err)
	}
	if err := c.Delete(ctx, acct, "f1"); err != nil {
		t.Fatalf("delete err=%v", err)
	}
	link, err := c.ShareLink(ctx, acct, "f1", true)
	if err != nil || link != "https://1drv.ms/x" {
		t.Fatalf("link=%q err=%v", link, err)
	}
	if _, err := c.ShareLink(ctx, acct, "f1", false); err != nil || len(perms) != 0 {
		t.Fatalf("revoke err=%v perms=%v", err, perms)
	}
}
