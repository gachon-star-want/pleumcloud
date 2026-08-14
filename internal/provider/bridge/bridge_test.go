package bridge

import (
	"context"
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

// fakeRunner records calls and answers with canned payloads.
type fakeRunner struct {
	calls  [][]string
	stdout string
	failOn string
}

func (f *fakeRunner) run(ctx context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	if f.failOn != "" && strings.Contains(strings.Join(args, " "), f.failOn) {
		return nil, &runError{args: args, exit: "exit status 1", stderr: "boom"}
	}
	return []byte(f.stdout), nil
}

func fakeAcct(s secret.Store) (*connector, provider.AccountRef, *fakeRunner) {
	_ = s.Set("t", []byte(`{"remote":"pleum-1","type":"mega","options":{"user":"u","pass":"p"}}`))
	r := &fakeRunner{}
	c := New("mega")(provider.Deps{Secrets: s}).(*connector)
	c.runCmd = r.run
	return c, provider.AccountRef{ID: "a", ProviderID: "mega", SecretRef: "t"}, r
}

func TestListUsesLsjson(t *testing.T) {
	s := memSecret{}
	c, acct, r := fakeAcct(s)
	r.stdout = `[{"Path":"Docs","Name":"Docs","IsDir":true,"Size":0,"ModTime":"2026-08-14T00:00:00Z"},
{"Path":"a.pdf","Name":"a.pdf","IsDir":false,"Size":42,"ModTime":"2026-08-14T00:00:00Z"}]`
	files, next, err := c.List(context.Background(), acct, "Docs", "")
	if err != nil || next != "" {
		t.Fatalf("err=%v next=%q", err, next)
	}
	if len(files) != 2 || !files[0].IsDir || files[0].RemoteID != "Docs" || files[1].Size != 42 {
		t.Fatalf("files=%+v", files)
	}
	call := strings.Join(r.calls[0], " ")
	if !strings.Contains(call, "lsjson") || !strings.Contains(call, "pleum-1:Docs") {
		t.Fatalf("call = %s", call)
	}
}

func TestOperationsMapToRcloneCommands(t *testing.T) {
	s := memSecret{}
	c, acct, r := fakeAcct(s)
	r.stdout = `{}`
	ctx := context.Background()

	_, _ = c.Mkdir(ctx, acct, "d", "New")
	_, _ = c.Move(ctx, acct, "d/a.txt", "d2", "b.txt")
	_, _ = c.Copy(ctx, acct, "d/a.txt", "d2", "c.txt")
	_ = c.Delete(ctx, acct, "d/a.txt")

	want := [][]string{
		{"mkdir", "pleum-1:d/New"},
		{"moveto", "pleum-1:d/a.txt", "pleum-1:d2/b.txt"},
		{"copyto", "pleum-1:d/a.txt", "pleum-1:d2/c.txt"},
		{"deletefile", "pleum-1:d/a.txt"},
	}
	if len(r.calls) != len(want) {
		t.Fatalf("calls = %+v", r.calls)
	}
	for i, w := range want {
		for _, part := range w {
			if !contains(r.calls[i], part) {
				t.Fatalf("call %d missing %q: %v", i, part, r.calls[i])
			}
		}
	}
}

func contains(arr []string, s string) bool {
	for _, a := range arr {
		if a == s {
			return true
		}
	}
	return false
}

func TestQuotaFromAbout(t *testing.T) {
	s := memSecret{}
	c, acct, r := fakeAcct(s)
	r.stdout = `{"total":1000,"used":400,"free":600}`
	q, err := c.Quota(context.Background(), acct)
	if err != nil || q.TotalBytes != 1000 || q.UsedBytes != 400 {
		t.Fatalf("q=%+v err=%v", q, err)
	}
	if !contains(r.calls[0], "about") {
		t.Fatalf("call = %v", r.calls[0])
	}
}

func TestErrorsCarryRcloneStderr(t *testing.T) {
	s := memSecret{}
	c, acct, r := fakeAcct(s)
	r.failOn = "lsjson"
	_, _, err := c.List(context.Background(), acct, "", "")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v (want rclone stderr surfaced)", err)
	}
}

func TestShareLinkUnsupported(t *testing.T) {
	s := memSecret{}
	c, acct, _ := fakeAcct(s)
	if _, err := c.ShareLink(context.Background(), acct, "x", true); err != provider.ErrUnsupported {
		t.Fatalf("err = %v", err)
	}
}

func TestConfigSnippetRoundTrip(t *testing.T) {
	snippet := "type = mega\nuser = me@example.com\npass = secret"
	name, type_, opts, err := ParseConfigSnippet(snippet)
	if err != nil || name != "" || type_ != "mega" || opts["user"] != "me@example.com" {
		t.Fatalf("parse = %q %q %v err=%v", name, type_, opts, err)
	}
}
