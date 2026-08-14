package index

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/pleumcloud/pleumcloud/internal/provider"
	"github.com/pleumcloud/pleumcloud/internal/store"
)

// fakeConn is a scripted connector for indexer tests.
type fakeConn struct{ ch provider.Changes }

func (f *fakeConn) Metadata() provider.Metadata { return provider.Metadata{ID: "fake"} }
func (f *fakeConn) List(ctx context.Context, a provider.AccountRef, parent, token string) ([]provider.File, string, error) {
	return nil, "", nil
}
func (f *fakeConn) Quota(ctx context.Context, a provider.AccountRef) (provider.Quota, error) {
	return provider.Quota{}, nil
}
func (f *fakeConn) Changes(ctx context.Context, a provider.AccountRef, cursor string) (provider.Changes, error) {
	return f.ch, nil
}
func (f *fakeConn) Upload(ctx context.Context, a provider.AccountRef, p, n string, r io.Reader, s int64, pg provider.ProgressFn) (provider.File, error) {
	return provider.File{}, nil
}
func (f *fakeConn) Open(ctx context.Context, a provider.AccountRef, id string, pg provider.ProgressFn) (io.ReadCloser, error) {
	return nil, nil
}
func (f *fakeConn) Mkdir(ctx context.Context, a provider.AccountRef, p, n string) (provider.File, error) {
	return provider.File{}, nil
}
func (f *fakeConn) Move(ctx context.Context, a provider.AccountRef, id, p, n string) (provider.File, error) {
	return provider.File{}, nil
}
func (f *fakeConn) Copy(ctx context.Context, a provider.AccountRef, id, p, n string) (provider.File, error) {
	return provider.File{}, nil
}
func (f *fakeConn) Delete(ctx context.Context, a provider.AccountRef, id string) error { return nil }
func (f *fakeConn) ShareLink(ctx context.Context, a provider.AccountRef, id string, c bool) (string, error) {
	return "", nil
}

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestInitialWalkIndexesWholeTree(t *testing.T) {
	st := newTestStore(t)
	if err := st.AddAccountWithID("acc1", "gdrive", "Test", "oauth2", "acct:acc1"); err != nil {
		t.Fatal(err)
	}
	acctID := "acc1"
	conn := &fakeConn{ch: provider.Changes{Cursor: "tok1", Upserted: []provider.File{
		{RemoteID: "d1", ParentID: "", Name: "Docs", IsDir: true},
		{RemoteID: "f1", ParentID: "d1", Name: "a.pdf", Size: 42, MIME: "application/pdf"},
	}}}
	idx := New(st)

	n, err := idx.Sync(context.Background(), provider.AccountRef{ID: acctID, ProviderID: "gdrive"}, conn)
	if err != nil || n != 2 {
		t.Fatalf("sync=%d err=%v", n, err)
	}
	kids, err := st.ListChildren("", "")
	if err != nil || len(kids) != 1 || kids[0].Name != "Docs" {
		t.Fatalf("root=%+v err=%v", kids, err)
	}
	kids, _ = st.ListChildren("d1", acctID)
	if len(kids) != 1 || kids[0].Size != 42 {
		t.Fatalf("d1=%+v", kids)
	}
	cur, _ := st.LoadCursor(acctID)
	if cur != "tok1" {
		t.Fatalf("cursor=%q", cur)
	}
}

func TestWalkCursorResyncsFromScratch(t *testing.T) {
	st := newTestStore(t)
	if err := st.AddAccountWithID("acc1", "webdav", "Test", "webdav", "acct:acc1"); err != nil {
		t.Fatal(err)
	}
	acctID := "acc1"
	idx := New(st)
	ref := provider.AccountRef{ID: acctID, ProviderID: "webdav"}

	// First walk: two files.
	conn := &fakeConn{ch: provider.Changes{Cursor: "walk", Upserted: []provider.File{
		{RemoteID: "a", ParentID: "", Name: "a"},
		{RemoteID: "b", ParentID: "", Name: "b"},
	}}}
	if _, err := idx.Sync(context.Background(), ref, conn); err != nil {
		t.Fatal(err)
	}
	// Second walk: only one file remains — stale row must disappear.
	conn.ch = provider.Changes{Cursor: "walk", Upserted: []provider.File{
		{RemoteID: "a", ParentID: "", Name: "a"},
	}}
	if _, err := idx.Sync(context.Background(), ref, conn); err != nil {
		t.Fatal(err)
	}
	kids, _ := st.ListChildren("", "")
	if len(kids) != 1 || kids[0].Name != "a" {
		t.Fatalf("after resync=%+v", kids)
	}
}

func TestDeltaAppliesIncrementally(t *testing.T) {
	st := newTestStore(t)
	if err := st.AddAccountWithID("acc1", "gdrive", "T", "oauth2", "acct:acc1"); err != nil {
		t.Fatal(err)
	}
	acctID := "acc1"
	idx := New(st)
	ref := provider.AccountRef{ID: acctID, ProviderID: "gdrive"}

	conn := &fakeConn{ch: provider.Changes{Cursor: "t1", Upserted: []provider.File{
		{RemoteID: "f1", ParentID: "", Name: "a.pdf", Size: 1},
	}}}
	if _, err := idx.Sync(context.Background(), ref, conn); err != nil {
		t.Fatal(err)
	}
	// Delta: f1 renamed, f2 new.
	conn.ch = provider.Changes{Cursor: "t2",
		Upserted: []provider.File{{RemoteID: "f1", ParentID: "", Name: "renamed.pdf", Size: 1}},
		Deleted:  []string{"f1"}, // deletion comes alongside the upsert of the same id
	}
	if _, err := idx.Sync(context.Background(), ref, conn); err != nil {
		t.Fatal(err)
	}
	// Delete-then-upsert of the same id means update; f2 insert:
	conn.ch = provider.Changes{Cursor: "t3", Upserted: []provider.File{
		{RemoteID: "f2", ParentID: "", Name: "b.pdf", Size: 2},
	}, Deleted: []string{"f1"}}
	if _, err := idx.Sync(context.Background(), ref, conn); err != nil {
		t.Fatal(err)
	}
	kids, _ := st.ListChildren("", "")
	if len(kids) != 1 || kids[0].Name != "b.pdf" {
		t.Fatalf("after delta=%+v", kids)
	}
	cur, _ := st.LoadCursor(acctID)
	if cur != "t3" {
		t.Fatalf("cursor=%q", cur)
	}
}

func TestSearchFindsAcrossAccounts(t *testing.T) {
	st := newTestStore(t)
	if err := st.AddAccountWithID("a1", "gdrive", "G", "oauth2", "x"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddAccountWithID("a2", "mybox", "M", "pat", "y"); err != nil {
		t.Fatal(err)
	}
	idx := New(st)
	c1 := &fakeConn{ch: provider.Changes{Cursor: "w", Upserted: []provider.File{{RemoteID: "r1", Name: "vacation.mp4", ParentID: "", Size: 5}}}}
	c2 := &fakeConn{ch: provider.Changes{Cursor: "w", Upserted: []provider.File{{RemoteID: "r2", Name: "vacation-notes.txt", ParentID: "", Size: 1}}}}
	if _, err := idx.Sync(context.Background(), provider.AccountRef{ID: "a1"}, c1); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.Sync(context.Background(), provider.AccountRef{ID: "a2"}, c2); err != nil {
		t.Fatal(err)
	}
	res, err := st.SearchFiles("vacation")
	if err != nil || len(res) != 2 {
		t.Fatalf("search=%+v err=%v", res, err)
	}
	_ = time.Now()
}
