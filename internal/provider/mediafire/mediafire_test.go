package mediafire

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
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

const (
	testEmail = "me@example.com"
	testPass  = "pw"
	testAppID = "12345"
	testKey   = "67890"
)

var acctSeq int

func fakeAccount(s secret.Store) (*connector, provider.AccountRef) {
	acctSeq++
	id := "a" + strconv.Itoa(acctSeq) // unique per test: session cache keys on ID
	_ = s.Set("test", []byte(`{"email":"`+testEmail+`","password":"`+testPass+`","appId":"`+testAppID+`","apiKey":"`+testKey+`"}`))
	return &connector{secrets: s}, provider.AccountRef{ID: id, ProviderID: "mediafire", SecretRef: "test"}
}

// TestLoginSignature pins the documented SHA1(email+password+appId+apiKey).
func TestLoginSignature(t *testing.T) {
	want := sha1.Sum([]byte(testEmail + testPass + testAppID + testKey))
	if got := loginSignature(testEmail, testPass, testAppID, testKey); got != hex.EncodeToString(want[:]) {
		t.Fatalf("sig = %s, want %s", got, hex.EncodeToString(want[:]))
	}
}

func mfServer(t *testing.T, session string, h func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *connector, provider.AccountRef) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "get_session_token") {
			if r.URL.Query().Get("signature") != loginSignature(testEmail, testPass, testAppID, testKey) {
				t.Errorf("bad signature %q", r.URL.Query().Get("signature"))
			}
			okJSON(w, map[string]any{"session_token": session})
			return
		}
		// Every op call must carry the (fake) session token.
		if r.URL.Query().Get("session_token") != session {
			t.Errorf("session_token = %q", r.URL.Query().Get("session_token"))
		}
		if r.URL.Query().Get("response_format") != "json" {
			t.Errorf("response_format = %q", r.URL.Query().Get("response_format"))
		}
		h(w, r)
	}))
	apiBase = srv.URL
	s := memSecret{}
	c, acct := fakeAccount(s)
	return srv, c, acct
}

func okJSON(w http.ResponseWriter, body map[string]any) {
	resp := map[string]any{"response": map[string]any{"result": "Success"}}
	for k, v := range body {
		resp["response"].(map[string]any)[k] = v
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func TestListCombinesFoldersAndFiles(t *testing.T) {
	srv, c, acct := mfServer(t, "tok1", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/folder/get_content.php") {
			t.Fatalf("unexpected %s", r.URL.Path)
		}
		switch r.URL.Query().Get("content_type") {
		case "folders":
			okJSON(w, map[string]any{"folder_content": map[string]any{
				"folders":     []map[string]any{{"folderkey": "fk1", "name": "Docs", "created": "2026-08-14 00:00:00"}},
				"more_chunks": "no",
			}})
		case "files":
			okJSON(w, map[string]any{"folder_content": map[string]any{
				"files":       []map[string]any{{"quickkey": "qk1", "filename": "a.pdf", "size": "42", "mimetype": "application/pdf", "created": "2026-08-14 00:00:00"}},
				"more_chunks": "no",
			}})
		default:
			t.Fatalf("content_type = %q", r.URL.Query().Get("content_type"))
		}
	})
	defer srv.Close()
	files, _, err := c.List(context.Background(), acct, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || !files[0].IsDir || files[0].RemoteID != "fk1" || files[1].RemoteID != "qk1" || files[1].Size != 42 {
		t.Fatalf("files = %+v", files)
	}
}

func TestQuotaAndLabel(t *testing.T) {
	srv, c, acct := mfServer(t, "tok2", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/user/get_info.php") {
			okJSON(w, map[string]any{"user_info": map[string]any{
				"used_storage_size": "1000", "storage_limit": "9000", "email": testEmail,
			}})
			return
		}
		t.Fatalf("unexpected %s", r.URL.Path)
	})
	defer srv.Close()
	ctx := context.Background()
	q, err := c.Quota(ctx, acct)
	if err != nil || q.UsedBytes != 1000 || q.TotalBytes != 9000 {
		t.Fatalf("q=%+v err=%v", q, err)
	}
	if l, err := c.AccountLabel(ctx, acct); err != nil || l != testEmail {
		t.Fatalf("label=%q err=%v", l, err)
	}
}

func TestUploadSimpleAndPoll(t *testing.T) {
	srv, c, acct := mfServer(t, "tok3", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "get_session_token"):
			okJSON(w, map[string]any{"session_token": "tok3"})
		case strings.Contains(r.URL.Path, "/upload/simple.php"):
			if r.URL.Query().Get("folder_key") != "fk1" {
				t.Fatalf("folder_key = %s", r.URL.RawQuery)
			}
			if r.Header.Get("X-Filesize") != "5" {
				t.Fatalf("X-Filesize = %s", r.Header.Get("X-Filesize"))
			}
			if !strings.Contains(r.Header.Get("Content-Disposition"), "up.bin") {
				t.Fatalf("disposition = %s", r.Header.Get("Content-Disposition"))
			}
			b, _ := io.ReadAll(r.Body)
			if string(b) != "hello" {
				t.Fatalf("body = %q", b)
			}
			okJSON(w, map[string]any{"doupload": map[string]any{"result": "0", "key": "k9"}})
		case strings.Contains(r.URL.Path, "/upload/poll_upload.php"):
			if r.URL.Query().Get("key") != "k9" {
				t.Fatalf("key = %s", r.URL.RawQuery)
			}
			okJSON(w, map[string]any{"do_upload": map[string]any{"status": "99", "quickkey": "qk77", "size": "5"}})
		default:
			t.Fatalf("unexpected %s", r.URL.Path)
		}
	})
	defer srv.Close()
	f, err := c.Upload(context.Background(), acct, "fk1", "up.bin", strings.NewReader("hello"), 5, nil)
	if err != nil || f.RemoteID != "qk77" || f.ParentID != "fk1" {
		t.Fatalf("upload=%+v err=%v", f, err)
	}
}

func srvURL(r *http.Request) string { return "http://" + r.Host }

func TestDownloadAndShareViaGetLinks(t *testing.T) {
	var direct string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "get_session_token"):
			okJSON(w, map[string]any{"session_token": "tok4"})
		case strings.Contains(r.URL.Path, "/file/get_links.php"):
			qk := r.URL.Query().Get("quick_key")
			if r.URL.Query().Get("link_type") == "direct_download" {
				direct = srvURL(r) + "/bytes"
				okJSON(w, map[string]any{"links": []map[string]any{{"direct_download": direct}}})
				return
			}
			okJSON(w, map[string]any{"links": []map[string]any{{"view": "https://www.mediafire.com/view/" + qk}}})
		case r.URL.Path == "/bytes":
			_, _ = w.Write([]byte("content"))
		default:
			t.Fatalf("unexpected %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	apiBase = srv.URL
	s := memSecret{}
	c, acct := fakeAccount(s)
	ctx := context.Background()

	rc, err := c.Open(ctx, acct, "qk5", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if string(b) != "content" {
		t.Fatalf("content=%q", b)
	}
	link, err := c.ShareLink(ctx, acct, "qk5", true)
	if err != nil || link != "https://www.mediafire.com/view/qk5" {
		t.Fatalf("link=%q err=%v", link, err)
	}
	if _, err := c.ShareLink(ctx, acct, "qk5", false); err != provider.ErrUnsupported {
		t.Fatalf("revoke err=%v (unsupported)", err)
	}
}

func TestMutations(t *testing.T) {
	srv, c, acct := mfServer(t, "tok5", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "get_session_token"):
			okJSON(w, map[string]any{"session_token": "tok5"})
		case strings.Contains(r.URL.Path, "/folder/create.php"):
			if r.URL.Query().Get("foldername") != "New" || r.URL.Query().Get("parent_key") != "fk1" {
				t.Fatalf("create = %s", r.URL.RawQuery)
			}
			okJSON(w, map[string]any{"folder_key": "fk9", "name": "New"})
		case strings.Contains(r.URL.Path, "/file/update.php"):
			if r.URL.Query().Get("quick_key") != "qk1" || r.URL.Query().Get("filename") != "renamed.pdf" {
				t.Fatalf("update = %s", r.URL.RawQuery)
			}
			okJSON(w, map[string]any{"quick_key": "qk1", "filename": "renamed.pdf"})
		case strings.Contains(r.URL.Path, "/file/move.php"):
			if r.URL.Query().Get("quick_key") != "qk1" || r.URL.Query().Get("folder_key") != "fk2" {
				t.Fatalf("move = %s", r.URL.RawQuery)
			}
			okJSON(w, map[string]any{})
		case strings.Contains(r.URL.Path, "/file/copy.php"):
			if r.URL.Query().Get("quick_key") != "qk1" {
				t.Fatalf("copy = %s", r.URL.RawQuery)
			}
			okJSON(w, map[string]any{"new_quickkey": "qk88"})
		case strings.Contains(r.URL.Path, "/file/delete.php"):
			if r.URL.Query().Get("quick_key") != "qk1" {
				t.Fatalf("delete = %s", r.URL.RawQuery)
			}
			okJSON(w, map[string]any{})
		default:
			t.Fatalf("unexpected %s", r.URL.Path)
		}
	})
	defer srv.Close()
	ctx := context.Background()

	if d, err := c.Mkdir(ctx, acct, "fk1", "New"); err != nil || !d.IsDir || d.RemoteID != "fk9" {
		t.Fatalf("mkdir=%+v err=%v", d, err)
	}
	if m, err := c.Move(ctx, acct, "qk1", "fk2", "renamed.pdf"); err != nil || m.Name != "renamed.pdf" {
		t.Fatalf("move=%+v err=%v", m, err)
	}
	if cp, err := c.Copy(ctx, acct, "qk1", "fk2", ""); err != nil || cp.RemoteID != "qk88" {
		t.Fatalf("copy=%+v err=%v", cp, err)
	}
	if err := c.Delete(ctx, acct, "qk1"); err != nil {
		t.Fatalf("delete err=%v", err)
	}
	_ = strconv.Itoa
}
