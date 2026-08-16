package pcloud

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
	return &connector{secrets: s}, provider.AccountRef{ID: "a1", ProviderID: "pcloud", SecretRef: "test"}
}

func fileEntry(fileid int64, name string, size int64, parent int64) map[string]any {
	return map[string]any{
		"isfolder": false, "fileid": fileid, "name": name, "size": size,
		"parentfolderid": parent, "contenttype": "image/jpeg",
		"modified": "Thu, 14 Aug 2026 00:00:00 +0000",
		"created":  "Thu, 14 Aug 2026 00:00:00 +0000",
	}
}

func folderEntry(folderid int64, name string, parent int64) map[string]any {
	return map[string]any{
		"isfolder": true, "folderid": folderid, "name": name,
		"parentfolderid": parent,
		"modified":       "Thu, 14 Aug 2026 00:00:00 +0000",
		"created":        "Thu, 14 Aug 2026 00:00:00 +0000",
	}
}

func TestListRootAndFolder(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer t" {
			t.Errorf("auth = %q", got)
		}
		if r.URL.Path == "/listfolder" {
			fid := r.URL.Query().Get("folderid")
			parent := int64(0)
			if fid == "5" {
				parent = 5
			} else if fid != "0" {
				t.Fatalf("folderid = %q", fid)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": 0,
				"metadata": map[string]any{
					"contents": []map[string]any{
						folderEntry(5, "Docs", parent),
						fileEntry(7, "a.jpg", 42, parent),
					},
				},
			})
			return
		}
		t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()
	apiBase = srv.URL

	files, next, err := c.List(context.Background(), acct, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || !files[0].IsDir || files[0].RemoteID != "d5" || files[1].RemoteID != "f7" || files[1].Size != 42 || files[1].MIME != "image/jpeg" {
		t.Fatalf("files = %+v", files)
	}
	if next != "" {
		t.Fatalf("next = %q (pCloud has no pagination)", next)
	}
	if !files[0].ModTime.IsZero() == false && files[0].ModTime.Year() != 2026 {
		t.Fatalf("mtime = %v", files[0].ModTime)
	}
}

func TestQuotaAndLabel(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/userinfo" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": 0, "email": "me@pcloud.com",
				"quota": 10000000000, "usedquota": 1000000000,
			})
			return
		}
		t.Fatalf("unexpected %s", r.URL.Path)
	}))
	defer srv.Close()
	apiBase = srv.URL

	q, err := c.Quota(context.Background(), acct)
	if err != nil || q.TotalBytes != 10000000000 || q.UsedBytes != 1000000000 {
		t.Fatalf("quota=%+v err=%v", q, err)
	}
	label, err := c.AccountLabel(context.Background(), acct)
	if err != nil || label != "me@pcloud.com" {
		t.Fatalf("label=%q err=%v", label, err)
	}
}

// TestChangesRecursive: recursive=1 returns nested contents; the connector
// flattens the whole tree.
func TestChangesRecursive(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/listfolder" {
			if r.URL.Query().Get("recursive") != "1" {
				t.Fatalf("recursive = %q", r.URL.Query().Get("recursive"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": 0,
				"metadata": map[string]any{
					"contents": []map[string]any{
						map[string]any{
							"isfolder": true, "folderid": 5, "name": "Docs", "parentfolderid": 0,
							"contents": []map[string]any{
								fileEntry(7, "a.jpg", 9, 5),
								map[string]any{
									"isfolder": true, "folderid": 6, "name": "Sub", "parentfolderid": 5,
									"contents": []map[string]any{fileEntry(8, "b.jpg", 4, 6)},
								},
							},
						},
						fileEntry(9, "root.jpg", 1, 0),
					},
				},
			})
			return
		}
		t.Fatalf("unexpected %s", r.URL.Path)
	}))
	defer srv.Close()
	apiBase = srv.URL

	ch, err := c.Changes(context.Background(), acct, "")
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, f := range ch.Upserted {
		ids[f.RemoteID] = true
	}
	if len(ch.Upserted) != 5 || !ids["d5"] || !ids["f7"] || !ids["d6"] || !ids["f8"] || !ids["f9"] {
		t.Fatalf("walk = %+v", ch.Upserted)
	}
	if ch.Cursor != "walk" {
		t.Fatalf("cursor = %q", ch.Cursor)
	}
}

func TestUploadMultipartAndDownload(t *testing.T) {
	s := memSecret{}
	c, acct := fakeAccount(s)
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/uploadfile":
			if r.URL.Query().Get("folderid") != "5" || r.URL.Query().Get("filename") != "up.bin" {
				t.Fatalf("upload query = %s", r.URL.RawQuery)
			}
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				t.Fatalf("content-type = %s", r.Header.Get("Content-Type"))
			}
			file, hdr, err := r.FormFile("file")
			if err != nil || hdr.Filename != "up.bin" {
				t.Fatalf("form file: %v %+v", err, hdr)
			}
			b, _ := io.ReadAll(file)
			if string(b) != "hello" {
				t.Fatalf("bytes = %q", b)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": 0, "fileids": []int{77},
				"metadata": []map[string]any{fileEntry(77, "up.bin", 5, 5)},
			})
		case "/getfilelink":
			if r.URL.Query().Get("fileid") != "7" {
				t.Fatalf("fileid = %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result":  0,
				"hosts":   []string{srvURL},
				"path":    "/hash/a.jpg",
				"expires": "Fri, 15 Aug 2026 00:00:00 +0000",
			})
		case "/hash/a.jpg":
			_, _ = w.Write([]byte("content"))
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL
	apiBase = srv.URL

	f, err := c.Upload(context.Background(), acct, "d5", "up.bin", strings.NewReader("hello"), 5, nil)
	if err != nil || f.RemoteID != "f77" || f.ParentID != "d5" {
		t.Fatalf("upload=%+v err=%v", f, err)
	}
	rc, err := c.Open(context.Background(), acct, "f7", nil)
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/createfolder":
			q := r.URL.Query()
			if q.Get("folderid") != "0" || q.Get("name") != "New" {
				t.Fatalf("createfolder = %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result":   0,
				"metadata": folderEntry(9, "New", 0),
			})
		case "/renamefile":
			q := r.URL.Query()
			if q.Get("fileid") != "7" || q.Get("tofolderid") != "5" || q.Get("toname") != "renamed.jpg" {
				t.Fatalf("renamefile = %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result":   0,
				"metadata": fileEntry(7, "renamed.jpg", 5, 5),
			})
		case "/renamefolder":
			q := r.URL.Query()
			if q.Get("folderid") != "6" || q.Get("tofolderid") != "5" || q.Get("toname") != "Moved" {
				t.Fatalf("renamefolder = %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result":   0,
				"metadata": folderEntry(6, "Moved", 5),
			})
		case "/copyfile":
			q := r.URL.Query()
			if q.Get("fileid") != "7" || q.Get("tofolderid") != "5" || q.Get("toname") != "copy.jpg" {
				t.Fatalf("copyfile = %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result":   0,
				"metadata": fileEntry(78, "copy.jpg", 5, 5),
			})
		case "/deletefile":
			if r.URL.Query().Get("fileid") != "7" {
				t.Fatalf("deletefile = %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result":   0,
				"metadata": fileEntry(7, "a.jpg", 5, 5),
			})
		case "/getfilepublink":
			if r.URL.Query().Get("fileid") != "7" {
				t.Fatalf("publink = %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": 0, "linkid": 12, "code": "XyZ",
				"link": "https://my.pcloud.com/#page=publink&code=XyZ",
			})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	apiBase = srv.URL
	ctx := context.Background()

	if d, err := c.Mkdir(ctx, acct, "", "New"); err != nil || !d.IsDir || d.RemoteID != "d9" {
		t.Fatalf("mkdir=%+v err=%v", d, err)
	}
	if m, err := c.Move(ctx, acct, "f7", "d5", "renamed.jpg"); err != nil || m.RemoteID != "f7" {
		t.Fatalf("move=%+v err=%v", m, err)
	}
	if m, err := c.Move(ctx, acct, "d6", "d5", "Moved"); err != nil || m.RemoteID != "d6" {
		t.Fatalf("movefolder=%+v err=%v", m, err)
	}
	if cp, err := c.Copy(ctx, acct, "f7", "d5", "copy.jpg"); err != nil || cp.RemoteID != "f78" {
		t.Fatalf("copy=%+v err=%v", cp, err)
	}
	if err := c.Delete(ctx, acct, "f7"); err != nil {
		t.Fatalf("delete err=%v", err)
	}
	link, err := c.ShareLink(ctx, acct, "f7", true)
	if err != nil || link != "https://my.pcloud.com/#page=publink&code=XyZ" {
		t.Fatalf("link=%q err=%v", link, err)
	}
}
