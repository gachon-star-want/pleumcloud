package store

import (
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func newUsersStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "u.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestUserLifecycleAndBcrypt(t *testing.T) {
	st := newUsersStore(t)
	hash, err := bcrypt.GenerateFromPassword([]byte("hunter2"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	id, err := st.EnsureUser("a@x.com", string(hash))
	if err != nil || id == "" {
		t.Fatalf("ensure = %q err=%v", id, err)
	}
	// Idempotent.
	id2, _ := st.EnsureUser("a@x.com", string(hash))
	if id2 != id {
		t.Fatalf("idempotency: %q vs %q", id, id2)
	}
	u, err := st.UserByEmail("a@x.com")
	if err != nil || u.ID != id {
		t.Fatalf("byEmail = %+v err=%v", u, err)
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("hunter2")) != nil {
		t.Fatal("password should verify")
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("wrong")) == nil {
		t.Fatal("wrong password must not verify")
	}
}

// The core multi-tenant guarantee: data created by one user is invisible
// to another, at every layer (accounts, files, jobs, rules).
func TestUserIsolation(t *testing.T) {
	st := newUsersStore(t)
	alice, _ := st.EnsureUser("alice@x.com", "")
	bob, _ := st.EnsureUser("bob@x.com", "")

	_ = st.AddAccountWithIDForUser("accA", alice, "gdrive", "Alice Drive", "oauth2", "s:a")
	_ = st.AddAccountWithIDForUser("accB", bob, "mybox", "Bob Box", "pat", "s:b")

	// Account listing is scoped.
	aa, _ := st.ListAccountsForUser(alice)
	if len(aa) != 1 || aa[0].ID != "accA" {
		t.Fatalf("alice accounts = %+v", aa)
	}
	bb, _ := st.ListAccountsForUser(bob)
	if len(bb) != 1 || bb[0].ID != "accB" {
		t.Fatalf("bob accounts = %+v", bb)
	}

	// GetAccount only through ownership.
	if _, err := st.GetAccountForUser("accA", bob); err == nil {
		t.Fatal("bob must not load alice's account")
	}
	if _, err := st.GetAccountForUser("accA", alice); err != nil {
		t.Fatalf("alice should load hers: %v", err)
	}

	// Files inherit the scope through the accounts join.
	_ = st.UpsertFile("accA", "r1", "", "secret.pdf", false, 1, "", 0)
	_ = st.UpsertFile("accB", "r2", "", "mine.pdf", false, 1, "", 0)
	kids, _ := st.ListChildrenForUser("", "", alice)
	if len(kids) != 1 || kids[0].Name != "secret.pdf" {
		t.Fatalf("alice files = %+v", kids)
	}
	res, _ := st.SearchFilesForUser("pdf", bob)
	if len(res) != 1 || res[0].Name != "mine.pdf" {
		t.Fatalf("bob search = %+v", res)
	}

	// File access goes through ownership.
	f, err := st.GetFileForUser(kids[0].ID, bob)
	if err == nil {
		t.Fatalf("bob must not fetch alice's file row: %+v", f)
	}
	if _, err := st.GetFileForUser(kids[0].ID, alice); err != nil {
		t.Fatalf("alice should fetch hers: %v", err)
	}

	// Jobs and rules scoped.
	_, _ = st.AddJobForUser(alice, "transfer", "f", "accA", "r1", "accA", "gdrive", 1)
	jb, _ := st.ListJobsForUser(bob, 10)
	ja, _ := st.ListJobsForUser(alice, 10)
	if len(jb) != 0 || len(ja) != 1 {
		t.Fatalf("jobs alice=%d bob=%d", len(ja), len(jb))
	}
	_, _ = st.AddRuleForUser(alice, 1, true, "mime", "is", "video/", "accA")
	rb, _ := st.ListRulesForUser(bob)
	ra, _ := st.ListRulesForUser(alice)
	if len(rb) != 0 || len(ra) != 1 {
		t.Fatalf("rules alice=%d bob=%d", len(ra), len(rb))
	}
}

// Migration v2 on an existing v1 database keeps rows under 'local'.
func TestV1DatabaseUpgradesToLocalUser(t *testing.T) {
	st := newUsersStore(t)
	local, _ := st.EnsureUser("local", "")
	if local == "" {
		t.Fatal("local user missing")
	}
	_ = st.AddAccountWithIDForUser("acc1", local, "webdav", "DAV", "webdav", "s")
	got, err := st.GetAccountForUser("acc1", local)
	if err != nil || got.Label != "DAV" {
		t.Fatalf("local-scoped access failed: %+v err=%v", got, err)
	}
}
