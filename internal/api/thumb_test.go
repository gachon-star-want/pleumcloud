package api

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pleumcloud/pleumcloud/internal/auth"
	"github.com/pleumcloud/pleumcloud/internal/index"
	"github.com/pleumcloud/pleumcloud/internal/oauthflow"
	"github.com/pleumcloud/pleumcloud/internal/provider"
	"github.com/pleumcloud/pleumcloud/internal/secret"
	"github.com/pleumcloud/pleumcloud/internal/store"
)

// bytesConn serves a fixed blob for every Open; enough for thumb tests.
type bytesConn struct{ blob []byte }

func (c *bytesConn) Metadata() provider.Metadata { return provider.Metadata{ID: "thumbfake"} }
func (c *bytesConn) List(ctx context.Context, a provider.AccountRef, p, tok string) ([]provider.File, string, error) {
	return nil, "", nil
}
func (c *bytesConn) Quota(ctx context.Context, a provider.AccountRef) (provider.Quota, error) {
	return provider.Quota{}, nil
}
func (c *bytesConn) Changes(ctx context.Context, a provider.AccountRef, cursor string) (provider.Changes, error) {
	return provider.Changes{}, nil
}
func (c *bytesConn) Upload(ctx context.Context, a provider.AccountRef, p, n string, r io.Reader, s int64, pg provider.ProgressFn) (provider.File, error) {
	return provider.File{}, nil
}
func (c *bytesConn) Open(ctx context.Context, a provider.AccountRef, id string, pg provider.ProgressFn) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(c.blob)), nil
}
func (c *bytesConn) Mkdir(ctx context.Context, a provider.AccountRef, p, n string) (provider.File, error) {
	return provider.File{}, nil
}
func (c *bytesConn) Move(ctx context.Context, a provider.AccountRef, id, p, n string) (provider.File, error) {
	return provider.File{}, nil
}
func (c *bytesConn) Copy(ctx context.Context, a provider.AccountRef, id, p, n string) (provider.File, error) {
	return provider.File{}, nil
}
func (c *bytesConn) Delete(ctx context.Context, a provider.AccountRef, id string) error { return nil }
func (c *bytesConn) ShareLink(ctx context.Context, a provider.AccountRef, id string, create bool) (string, error) {
	return "", provider.ErrUnsupported
}

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

// newThumbTestAPI wires a local-mode API with one fake account and returns
// it plus the handler to exercise.
func newThumbTestAPI(t *testing.T, blob []byte) (http.Handler, *store.Store, string, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	provider.RegisterFactory("thumbfake", func(provider.Deps) provider.Connector {
		return &bytesConn{blob: blob}
	})
	local, err := st.EnsureUser("local", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddAccountWithIDForUser("tacc", local, "thumbfake", "Fake", "pat", "acct:tacc"); err != nil {
		t.Fatal(err)
	}

	secrets := memSecret{}
	tokens, err := auth.LoadOrCreateTokenKey(filepath.Join(dir, "auth.key"))
	if err != nil {
		t.Fatal(err)
	}
	a := New(st, secrets, oauthflow.NewManager(secrets), index.New(st), tokens, false, "test")
	if err := a.InitLocalUser(); err != nil {
		t.Fatal(err)
	}
	a.SetDataDir(dir)
	return a.Routes(), st, local, dir
}

// insertImage stores one image row and returns its file handle.
func insertImage(t *testing.T, st *store.Store, local, name, mime string) string {
	t.Helper()
	if err := st.UpsertFile("tacc", "r:"+name, "", name, false, int64(len(name)), mime, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	rows, err := st.ListChildrenForUser("", "tacc", local)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range rows {
		if f.Name == name {
			return f.ID
		}
	}
	t.Fatalf("row for %q not found", name)
	return ""
}

func tinyJPEG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// heifHead builds a minimal HEIF-looking header (ftyp box with a heic brand).
func heifHead() []byte {
	b := make([]byte, 64)
	copy(b[4:8], "ftyp")
	copy(b[8:12], "heic")
	copy(b[12:16], "mif1")
	return b
}

func getThumb(h http.Handler, id string, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/file/"+id+"/thumb"+query, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestThumbJPEG(t *testing.T) {
	h, st, local, _ := newThumbTestAPI(t, tinyJPEG(t))
	id := insertImage(t, st, local, "a.jpg", "image/jpeg")

	rec := getThumb(h, id, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %q", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("content-type = %q", ct)
	}
	if _, err := jpeg.Decode(bytes.NewReader(rec.Body.Bytes())); err != nil {
		t.Fatalf("response is not JPEG: %v", err)
	}
}

func TestThumbHEICUsesConverter(t *testing.T) {
	jpg := tinyJPEG(t)
	h, st, local, _ := newThumbTestAPI(t, heifHead())

	// Stand in for sips/heif-convert: write a JPEG to the output path.
	orig := heicConvert
	heicConvert = func(ctx context.Context, in, out string, size int) error {
		return os.WriteFile(out, jpg, 0o600)
	}
	t.Cleanup(func() { heicConvert = orig })

	id := insertImage(t, st, local, "IMG_1.HEIC", "image/heic")
	rec := getThumb(h, id, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %q", rec.Code, rec.Body.String())
	}
	if _, err := jpeg.Decode(bytes.NewReader(rec.Body.Bytes())); err != nil {
		t.Fatalf("response is not JPEG: %v", err)
	}
}

func TestThumbCachesPerSize(t *testing.T) {
	h, st, local, dir := newThumbTestAPI(t, tinyJPEG(t))
	id := insertImage(t, st, local, "b.png", "image/png")

	if rec := getThumb(h, id, "?size=64"); rec.Code != http.StatusOK {
		t.Fatalf("size=64 status = %d", rec.Code)
	}
	if rec := getThumb(h, id, ""); rec.Code != http.StatusOK {
		t.Fatalf("default size status = %d", rec.Code)
	}
	for _, name := range []string{id + "_64.jpg", id + "_384.jpg"} {
		if _, err := os.Stat(filepath.Join(dir, "thumbs", name)); err != nil {
			t.Fatalf("cache file %s missing: %v", name, err)
		}
	}
}

func TestThumbRejectsNonImage(t *testing.T) {
	h, st, local, _ := newThumbTestAPI(t, []byte("notes"))
	id := insertImage(t, st, local, "notes.txt", "text/plain")
	if rec := getThumb(h, id, ""); rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", rec.Code)
	}
}

func TestThumbHEICNameAcceptedWithoutMIME(t *testing.T) {
	jpg := tinyJPEG(t)
	h, st, local, _ := newThumbTestAPI(t, heifHead())
	orig := heicConvert
	heicConvert = func(ctx context.Context, in, out string, size int) error {
		return os.WriteFile(out, jpg, 0o600)
	}
	t.Cleanup(func() { heicConvert = orig })

	id := insertImage(t, st, local, "IMG_2.heic", "")
	if rec := getThumb(h, id, ""); rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %q", rec.Code, rec.Body.String())
	}
}
