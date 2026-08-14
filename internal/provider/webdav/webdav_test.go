package webdav

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/net/webdav"

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

func newTestServer(t *testing.T) (*connector, provider.AccountRef, func()) {
	t.Helper()
	fs := webdav.NewMemFS()
	srv := httptest.NewServer(&webdav.Handler{
		FileSystem: fs,
		LockSystem: webdav.NewMemLS(),
	})
	s := memSecret{}
	_ = s.Set("test", []byte(`{"url":"`+srv.URL+`","username":"u","password":"p"}`))
	c := &connector{secrets: s}
	acct := provider.AccountRef{ID: "a1", ProviderID: "webdav", SecretRef: "test"}
	return c, acct, srv.Close
}

func TestWebDAVRoundTrip(t *testing.T) {
	c, acct, done := newTestServer(t)
	defer done()
	ctx := context.Background()

	// mkdir + upload
	dir, err := c.Mkdir(ctx, acct, "", "docs")
	if err != nil || !dir.IsDir || dir.RemoteID != "docs" {
		t.Fatalf("mkdir = %+v err=%v", dir, err)
	}
	f, err := c.Upload(ctx, acct, "docs", "hello.txt", strings.NewReader("hi"), 2, nil)
	if err != nil || f.RemoteID != "docs/hello.txt" {
		t.Fatalf("upload = %+v err=%v", f, err)
	}

	// list
	files, _, err := c.List(ctx, acct, "docs", "")
	if err != nil || len(files) != 1 || files[0].Name != "hello.txt" || files[0].Size != 2 {
		t.Fatalf("list = %+v err=%v", files, err)
	}

	// download
	rc, err := c.Open(ctx, acct, "docs/hello.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "hi" {
		t.Fatalf("content = %q", b)
	}

	// move within tree
	moved, err := c.Move(ctx, acct, "docs/hello.txt", "", "renamed.txt")
	if err != nil || moved.RemoteID != "renamed.txt" {
		t.Fatalf("move = %+v err=%v", moved, err)
	}
	files, _, _ = c.List(ctx, acct, "", "")
	if len(files) != 2 { // docs/ + renamed.txt
		t.Fatalf("root after move = %+v", files)
	}

	// copy
	cp, err := c.Copy(ctx, acct, "renamed.txt", "docs", "copy.txt")
	if err != nil || cp.RemoteID != "docs/copy.txt" {
		t.Fatalf("copy = %+v err=%v", cp, err)
	}

	// changes walk
	ch, err := c.Changes(ctx, acct, "")
	if err != nil || len(ch.Upserted) != 3 { // docs, docs/copy.txt, renamed.txt
		t.Fatalf("changes = %+v err=%v", ch.Upserted, err)
	}

	// delete
	if err := c.Delete(ctx, acct, "docs/copy.txt"); err != nil {
		t.Fatal(err)
	}
	files, _, _ = c.List(ctx, acct, "docs", "")
	if len(files) != 0 {
		t.Fatalf("docs after delete = %+v", files)
	}

	// quota/share unsupported
	if _, err := c.Quota(ctx, acct); err != provider.ErrUnsupported {
		t.Fatalf("quota err = %v", err)
	}
	if _, err := c.ShareLink(ctx, acct, "x", true); err != provider.ErrUnsupported {
		t.Fatalf("share err = %v", err)
	}
}
