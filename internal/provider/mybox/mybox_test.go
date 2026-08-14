package mybox

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
	_ = s.Set("test", []byte(`{"pat":"mbx_pat_test"}`))
	return &connector{secrets: s}, provider.AccountRef{ID: "a1", ProviderID: "mybox", SecretRef: "test"}
}

func TestQuotaAndList(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer mbx_pat_test" {
			t.Errorf("auth = %q", got)
		}
		switch r.URL.Path {
		case "/v1/drive/storage":
			_ = json.NewEncoder(w).Encode(map[string]any{"quotaBytes": 30000000000, "usedBytes": 1000000000})
		case "/v1/drive/resources":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resources": []map[string]any{
					{"resourceId": "d1", "name": "Photos", "type": "FOLDER", "size": 0, "parentId": "root"},
					{"resourceId": "f1", "name": "a.jpg", "type": "FILE", "size": 100, "parentId": "root", "category": "image"},
				},
				"responseMetaData": map[string]any{"nextCursor": "c2"},
			})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	apiBase = srv.URL

	q, err := c.Quota(context.Background(), acct)
	if err != nil || q.TotalBytes != 30000000000 || q.UsedBytes != 1000000000 {
		t.Fatalf("quota=%+v err=%v", q, err)
	}
	files, next, err := c.List(context.Background(), acct, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || !files[0].IsDir || files[0].Name != "Photos" || files[1].Size != 100 || next != "c2" {
		t.Fatalf("files=%+v next=%q", files, next)
	}
}

func TestUploadPresignFlow(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	var uploaded string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/drive/files":
			if r.Method != http.MethodPost {
				t.Fatalf("init method %s", r.Method)
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["fileName"] != "u.bin" || body["fileSize"] != float64(5) {
				t.Fatalf("init body = %v", body)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"uploadUrl": srvURL(r) + "/storage-upload",
				"offset":    0,
			})
		case "/storage-upload":
			if r.Method != http.MethodPut {
				t.Fatalf("upload method %s", r.Method)
			}
			b, _ := io.ReadAll(r.Body)
			uploaded = string(b)
			w.WriteHeader(http.StatusOK)
		case "/v1/drive/folders/fld/resources":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resources": []map[string]any{
					{"resourceId": "nf1", "name": "u.bin", "type": "FILE", "size": 5, "parentId": "fld"},
				},
			})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	apiBase = srv.URL

	f, err := c.Upload(context.Background(), acct, "fld", "u.bin", strings.NewReader("hello"), 5, nil)
	if err != nil {
		t.Fatal(err)
	}
	if uploaded != "hello" || f.RemoteID != "nf1" {
		t.Fatalf("upload=%+v uploaded=%q", f, uploaded)
	}
}

func TestDownloadPresignFlow(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/drive/files/f1/download":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"downloadUrl": srvURL(r) + "/storage-download",
				"expiresIn":   600,
			})
		case "/storage-download":
			_, _ = w.Write([]byte("content"))
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	apiBase = srv.URL

	rc, err := c.Open(context.Background(), acct, "f1", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if string(b) != "content" {
		t.Fatalf("content = %q", b)
	}
}

func TestShareLinkUnsupported(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	if _, err := c.ShareLink(context.Background(), acct, "x", true); err != provider.ErrUnsupported {
		t.Fatalf("err = %v", err)
	}
}

func srvURL(r *http.Request) string {
	return "http://" + r.Host
}
