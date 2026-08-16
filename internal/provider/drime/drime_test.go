package drime

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

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
	_ = s.Set("test", []byte(`{"pat":"drime_tok"}`))
	return &connector{secrets: s}, provider.AccountRef{ID: "a1", ProviderID: "drime", SecretRef: "test"}
}

// mkEntry builds a Drime FileEntry map.
func mkEntry(id int64, name, typ string, size int64, parent *int64, hash string) map[string]any {
	m := map[string]any{
		"id": id, "name": name, "type": typ, "file_size": size,
		"hash": hash, "mime": "application/pdf",
		"updated_at": "2026-08-14T00:00:00.000000Z",
		"created_at": "2026-08-14T00:00:00.000000Z",
	}
	if parent != nil {
		m["parent_id"] = *parent
	}
	return m
}

func iptr(v int64) *int64 { return &v }

func TestListPagination(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer drime_tok" {
			t.Errorf("auth = %q", got)
		}
		if r.URL.Path != "/drive/file-entries" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		page := r.URL.Query().Get("page")
		if page == "" {
			page = "1"
		}
		parent := r.URL.Query().Get("folderId")
		if parent != "" {
			t.Fatalf("root list must omit folderId, got %q", parent)
		}
		var data []map[string]any
		if page == "1" {
			data = []map[string]any{mkEntry(1, "Docs", "folder", 0, nil, "h1"), mkEntry(2, "a.pdf", "pdf", 42, nil, "h2")}
		} else {
			data = []map[string]any{mkEntry(3, "b.pdf", "pdf", 7, nil, "h3")}
		}
		cur, _ := strconv.Atoi(page)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": data, "current_page": cur, "last_page": 2, "per_page": 100,
		})
	}))
	defer srv.Close()
	apiBase = srv.URL

	files, next, err := c.List(context.Background(), acct, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || !files[0].IsDir || files[0].RemoteID != "1" || files[1].Size != 42 {
		t.Fatalf("files = %+v", files)
	}
	if next != "2" {
		t.Fatalf("next = %q", next)
	}
	files2, next2, err := c.List(context.Background(), acct, "", next)
	if err != nil || len(files2) != 1 || next2 != "" {
		t.Fatalf("page2 = %+v %q err=%v", files2, next2, err)
	}
}

func TestListFolderAndQuotaAndLabel(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/drive/file-entries":
			if r.URL.Query().Get("folderId") != "9" {
				t.Fatalf("folderId = %q", r.URL.Query().Get("folderId"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data":         []map[string]any{mkEntry(2, "a.pdf", "pdf", 42, iptr(9), "h2")},
				"current_page": 1, "last_page": 1,
			})
		case "/user/space-usage":
			_ = json.NewEncoder(w).Encode(map[string]any{"used": 1000, "available": 9000})
		case "/cli/loggedUser":
			_ = json.NewEncoder(w).Encode(map[string]any{"email": "me@drime.cloud", "id": 1})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	apiBase = srv.URL
	ctx := context.Background()

	files, _, err := c.List(ctx, acct, "9", "")
	if err != nil || len(files) != 1 || files[0].ParentID != "9" {
		t.Fatalf("files=%+v err=%v", files, err)
	}
	q, err := c.Quota(ctx, acct)
	if err != nil || q.UsedBytes != 1000 || q.TotalBytes != 10000 {
		t.Fatalf("quota=%+v err=%v", q, err)
	}
	label, err := c.AccountLabel(ctx, acct)
	if err != nil || label != "me@drime.cloud" {
		t.Fatalf("label=%q err=%v", label, err)
	}
}

func TestUploadMultipart(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/uploads" {
			t.Fatalf("unexpected %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Fatalf("content-type = %s", r.Header.Get("Content-Type"))
		}
		_ = r.ParseMultipartForm(1 << 20)
		if r.FormValue("parentId") != "9" {
			t.Fatalf("parentId = %q", r.FormValue("parentId"))
		}
		f, hdr, err := r.FormFile("file")
		if err != nil || hdr.Filename != "up.bin" {
			t.Fatalf("file part: %v %+v", err, hdr)
		}
		b, _ := io.ReadAll(f)
		if string(b) != "hello" {
			t.Fatalf("bytes = %q", b)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":    "success",
			"fileEntry": mkEntry(77, "up.bin", "file", 5, iptr(9), "h77"),
		})
	}))
	defer srv.Close()
	apiBase = srv.URL

	f, err := c.Upload(context.Background(), acct, "9", "up.bin", strings.NewReader("hello"), 5, nil)
	if err != nil || f.RemoteID != "77" || f.ParentID != "9" {
		t.Fatalf("upload=%+v err=%v", f, err)
	}
}

// Download resolves the entry hash first, then streams from download/{hash}.
func TestDownloadViaHash(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/file-entries/2":
			_ = json.NewEncoder(w).Encode(mkEntry(2, "a.pdf", "pdf", 42, nil, "h2"))
		case "/file-entries/download/h2":
			_, _ = w.Write([]byte("content"))
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	apiBase = srv.URL

	rc, err := c.Open(context.Background(), acct, "2", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if string(b) != "content" {
		t.Fatalf("content = %q", b)
	}
}

func TestMutations(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/folders":
			// Spec: null parent = root.
			if body["name"] != "New" || body["parentId"] != nil {
				t.Fatalf("folders = %v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success", "folder": mkEntry(9, "New", "folder", 0, nil, "h9"),
			})
		case "/file-entries/move":
			ids, _ := body["entryIds"].([]any)
			if len(ids) != 1 || ids[0] != float64(2) || body["destinationId"] != float64(9) {
				t.Fatalf("move = %v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success"})
		case "/file-entries/2":
			if r.Method != http.MethodPut {
				t.Fatalf("rename method %s", r.Method)
			}
			if body["name"] != "renamed.pdf" {
				t.Fatalf("rename = %v", body)
			}
			_ = json.NewEncoder(w).Encode(mkEntry(2, "renamed.pdf", "pdf", 42, iptr(9), "h2"))
		case "/file-entries/duplicate":
			ids, _ := body["entryIds"].([]any)
			if len(ids) != 1 || ids[0] != float64(2) {
				t.Fatalf("duplicate = %v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success"})
		case "/file-entries/delete":
			ids, _ := body["entryIds"].([]any)
			if len(ids) != 1 || ids[0] != float64(2) {
				t.Fatalf("delete = %v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success"})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	apiBase = srv.URL
	ctx := context.Background()

	if d, err := c.Mkdir(ctx, acct, "", "New"); err != nil || !d.IsDir || d.RemoteID != "9" {
		t.Fatalf("mkdir=%+v err=%v", d, err)
	}
	if m, err := c.Move(ctx, acct, "2", "9", "renamed.pdf"); err != nil || m.Name != "renamed.pdf" {
		t.Fatalf("move=%+v err=%v", m, err)
	}
	if _, err := c.Copy(ctx, acct, "2", "9", ""); err != nil {
		t.Fatalf("copy err=%v", err)
	}
	if err := c.Delete(ctx, acct, "2"); err != nil {
		t.Fatalf("delete err=%v", err)
	}
	if _, err := c.ShareLink(ctx, acct, "2", true); err != provider.ErrUnsupported {
		t.Fatalf("share err=%v (want ErrUnsupported)", err)
	}
}
