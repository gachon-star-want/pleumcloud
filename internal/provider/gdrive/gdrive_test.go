package gdrive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"

	"github.com/gachon-star-want/pleumcloud/internal/provider"
	"github.com/gachon-star-want/pleumcloud/internal/secret"
)

// memSecret is an in-memory secret.Store for tests.
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
	return &connector{secrets: s}, provider.AccountRef{ID: "a1", ProviderID: "gdrive", SecretRef: "test"}
}

func TestListPaginationAndMapping(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer t" {
			t.Errorf("auth header = %q", got)
		}
		if !strings.HasPrefix(r.URL.Path, "/drive/v3/files") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		page++
		resp := map[string]any{"files": []map[string]any{{
			"id": fmt.Sprintf("f%d", page), "name": "a.pdf",
			"mimeType": "application/pdf", "size": "42",
			"modifiedTime": "2026-08-14T00:00:00Z", "parents": []string{"root"},
		}}}
		if page == 1 {
			resp["nextPageToken"] = "p2"
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()
	apiBase = srv.URL + "/drive/v3"

	files, next, err := c.List(context.Background(), acct, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name != "a.pdf" || files[0].Size != 42 {
		t.Fatalf("mapping wrong: %+v", files)
	}
	if next != "p2" {
		t.Fatalf("next = %q", next)
	}
	files2, next2, _ := c.List(context.Background(), acct, "", next)
	if len(files2) != 1 || next2 != "" {
		t.Fatalf("second page wrong: %d %q", len(files2), next2)
	}
}

func TestQuota(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"storageQuota": map[string]string{"limit": "15000000000", "usage": "5000000000"},
			"user":         map[string]string{"emailAddress": "me@example.com"},
		})
	}))
	defer srv.Close()
	apiBase = srv.URL + "/drive/v3"

	q, err := c.Quota(context.Background(), acct)
	if err != nil {
		t.Fatal(err)
	}
	if q.TotalBytes != 15_000_000_000 || q.UsedBytes != 5_000_000_000 {
		t.Fatalf("quota = %+v", q)
	}
	label, err := c.AccountLabel(context.Background(), acct)
	if err != nil || label != "me@example.com" {
		t.Fatalf("label = %q err=%v", label, err)
	}
}

func TestChangesMarksTrashedAsDeleted(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/changes/startPageToken") {
			_ = json.NewEncoder(w).Encode(map[string]string{"startPageToken": "s1"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"changes": []map[string]any{
				{"fileId": "gone", "removed": true},
				{"fileId": "trashed", "file": map[string]any{"id": "trashed", "name": "x", "trashed": true}},
				{"fileId": "live", "file": map[string]any{"id": "live", "name": "y", "mimeType": "application/pdf"}},
			},
			"newStartPageToken": "s2",
		})
	}))
	defer srv.Close()
	apiBase = srv.URL + "/drive/v3"

	ch, err := c.Changes(context.Background(), acct, "")
	if err != nil || ch.Cursor != "s1" || len(ch.Upserted) != 0 {
		t.Fatalf("start page: %+v err=%v", ch, err)
	}
	ch, err = c.Changes(context.Background(), acct, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ch.Deleted) != 2 || len(ch.Upserted) != 1 || ch.Cursor != "s2" {
		t.Fatalf("changes = %+v", ch)
	}
}

func TestUploadResumableFlow(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	var gotBytes []byte
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/upload/drive/v3/files") && r.Method == http.MethodPost:
			w.Header().Set("Location", srvURL+"/session")
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/session":
			gotBytes, _ = io.ReadAll(r.Body)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "newfile", "name": "up.bin", "mimeType": "application/octet-stream", "size": "5",
			})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL
	apiBase = srv.URL + "/drive/v3"
	uploadBase = srv.URL + "/upload/drive/v3"

	f, err := c.Upload(context.Background(), acct, "", "up.bin", strings.NewReader("hello"), 5, nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.RemoteID != "newfile" || string(gotBytes) != "hello" {
		t.Fatalf("upload = %+v bytes=%q", f, gotBytes)
	}
}

func TestShareLinkFlow(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	var shared bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/permissions") && r.Method == http.MethodPost:
			shared = true
			w.WriteHeader(http.StatusCreated)
		case strings.HasSuffix(r.URL.Path, "/permissions/anyone") && r.Method == http.MethodDelete:
			shared = false
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Query().Get("fields") == "webViewLink":
			_ = json.NewEncoder(w).Encode(map[string]string{"webViewLink": "https://drive.google.com/file/d/x/view"})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	apiBase = srv.URL + "/drive/v3"

	link, err := c.ShareLink(context.Background(), acct, "x", true)
	if err != nil || link != "https://drive.google.com/file/d/x/view" || !shared {
		t.Fatalf("link=%q err=%v shared=%v", link, err, shared)
	}
	if _, err := c.ShareLink(context.Background(), acct, "x", false); err != nil || shared {
		t.Fatalf("revoke err=%v shared=%v", err, shared)
	}
}
