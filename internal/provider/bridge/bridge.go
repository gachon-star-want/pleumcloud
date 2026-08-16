// Package bridge rides long-tail providers on the rclone binary. Accounts
// store an rclone remote config; operations shell out to `rclone` with a
// dedicated config file under the data directory. This keeps experimental
// providers (MEGA, Box, Yandex, …) working while their native connectors
// are promoted one by one.
package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gachon-star-want/pleumcloud/internal/provider"
	"github.com/gachon-star-want/pleumcloud/internal/secret"
)

func init() {
	for _, id := range []string{"mega", "box", "yandex", "hidrive", "jottacloud", "filen", "internxt", "protondrive"} {
		provider.RegisterFactory(id, New(id))
	}
}

type connector struct {
	id      string
	secrets secret.Store
	// runCmd is injectable for tests; production uses the rclone binary.
	runCmd func(ctx context.Context, args ...string) ([]byte, error)
	// confPath is where the rclone config lives.
	confPath string
}

// New builds the bridge connector for a provider ID.
func New(id string) func(provider.Deps) provider.Connector {
	return func(deps provider.Deps) provider.Connector {
		c := &connector{id: id, secrets: deps.Secrets}
		c.confPath = filepath.Join(os.TempDir(), "pleumcloud-rclone.conf")
		c.runCmd = c.execRclone
		return c
	}
}

func (c *connector) Metadata() provider.Metadata {
	names := map[string]string{
		"mega": "MEGA", "box": "Box", "yandex": "Yandex Disk",
		"hidrive": "STRATO HiDrive", "jottacloud": "Jottacloud",
		"filen": "Filen", "internxt": "Internxt", "protondrive": "Proton Drive",
	}
	frees := map[string]int{"mega": 20, "box": 10, "yandex": 5, "jottacloud": 5, "filen": 10, "internxt": 10, "protondrive": 5}
	return provider.Metadata{
		ID: c.id, Name: names[c.id], AuthKind: provider.AuthBridge,
		Tier: provider.TierExperimental, FreeTierGB: frees[c.id],
		DocsURL: "https://rclone.org/" + c.id + "/",
	}
}

// ---- config ----

type credBundle struct {
	Remote  string            `json:"remote"` // rclone remote name, e.g. pleum-<id>
	Type    string            `json:"type"`   // rclone backend type
	Options map[string]string `json:"options"`
}

func (c *connector) creds(acct provider.AccountRef) (credBundle, error) {
	var cb credBundle
	if err := secret.GetJSON(c.secrets, acct.SecretRef, &cb); err != nil {
		return cb, fmt.Errorf("load credentials: %w", err)
	}
	if cb.Remote == "" || cb.Type == "" {
		return cb, fmt.Errorf("bridge: incomplete rclone config")
	}
	// Materialize the config file section (idempotent append).
	if err := c.writeConfig(cb); err != nil {
		return cb, err
	}
	return cb, nil
}

func (c *connector) writeConfig(cb credBundle) error {
	section := "[" + cb.Remote + "]\ntype = " + cb.Type + "\n"
	keys := make([]string, 0, len(cb.Options))
	for k := range cb.Options {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		section += k + " = " + cb.Options[k] + "\n"
	}
	existing := []byte{}
	if b, err := os.ReadFile(c.confPath); err == nil {
		existing = b
	}
	marker := "[" + cb.Remote + "]"
	if bytes.Contains(existing, []byte(marker)) {
		return nil // section already present
	}
	f, err := os.OpenFile(c.confPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("bridge: rclone config: %w", err)
	}
	defer f.Close()
	_, err = f.WriteString(section)
	return err
}

// ---- execution ----

type runError struct {
	args   []string
	exit   string
	stderr string
}

func (e *runError) Error() string {
	return fmt.Sprintf("rclone %s: %s: %s", strings.Join(e.args, " "), e.exit, e.stderr)
}

func (c *connector) execRclone(ctx context.Context, args ...string) ([]byte, error) {
	full := append(append([]string{}, "--config", c.confPath, "--quiet"), args...)
	cmd := exec.CommandContext(ctx, "rclone", full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, &runError{args: args, exit: err.Error(), stderr: stderr.String()}
	}
	return stdout.Bytes(), nil
}

// rpath builds remote-qualified paths.
func rpath(remote, p string) string { return remote + ":" + p }

// ---- model ----

type lsEntry struct {
	Path    string `json:"Path"`
	Name    string `json:"Name"`
	Size    int64  `json:"Size"`
	IsDir   bool   `json:"IsDir"`
	ModTime string `json:"ModTime"`
	Mime    string `json:"MimeType"`
}

func toFile(parent string, e lsEntry) provider.File {
	mt, _ := time.Parse(time.RFC3339, e.ModTime)
	return provider.File{
		RemoteID: e.Path,
		ParentID: parent,
		Name:     e.Name,
		IsDir:    e.IsDir,
		Size:     e.Size,
		MIME:     e.Mime,
		ModTime:  mt,
	}
}

// ---- Connector ----

func (c *connector) List(ctx context.Context, acct provider.AccountRef, parentRemoteID, pageToken string) ([]provider.File, string, error) {
	cb, err := c.creds(acct)
	if err != nil {
		return nil, "", err
	}
	out, err := c.runCmd(ctx, "lsjson", rpath(cb.Remote, parentRemoteID))
	if err != nil {
		return nil, "", err
	}
	var entries []lsEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, "", fmt.Errorf("bridge: lsjson decode: %w", err)
	}
	files := make([]provider.File, 0, len(entries))
	for _, e := range entries {
		files = append(files, toFile(parentRemoteID, e))
	}
	return files, "", nil
}

func (c *connector) Quota(ctx context.Context, acct provider.AccountRef) (provider.Quota, error) {
	cb, err := c.creds(acct)
	if err != nil {
		return provider.Quota{}, err
	}
	out, err := c.runCmd(ctx, "about", cb.Remote+":", "--json")
	if err != nil {
		return provider.Quota{}, err
	}
	var about struct {
		Total int64 `json:"total"`
		Used  int64 `json:"used"`
	}
	if err := json.Unmarshal(out, &about); err != nil {
		return provider.Quota{}, fmt.Errorf("bridge: about decode: %w", err)
	}
	return provider.Quota{TotalBytes: about.Total, UsedBytes: about.Used}, nil
}

func (c *connector) AccountLabel(ctx context.Context, acct provider.AccountRef) (string, error) {
	cb, err := c.creds(acct)
	if err != nil {
		return "", err
	}
	if u, ok := cb.Options["user"]; ok && u != "" {
		return u, nil
	}
	if u, ok := cb.Options["username"]; ok && u != "" {
		return u, nil
	}
	return cb.Type + " (rclone)", nil
}

// Changes: no delta — recursive lsjson walk.
func (c *connector) Changes(ctx context.Context, acct provider.AccountRef, cursor string) (provider.Changes, error) {
	cb, err := c.creds(acct)
	if err != nil {
		return provider.Changes{}, err
	}
	out, err := c.runCmd(ctx, "lsjson", cb.Remote+":", "--recursive")
	if err != nil {
		return provider.Changes{}, err
	}
	var entries []lsEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return provider.Changes{}, err
	}
	var all []provider.File
	for _, e := range entries {
		parent := ""
		if i := strings.LastIndex(e.Path, "/"); i >= 0 {
			parent = e.Path[:i]
		}
		all = append(all, toFile(parent, e))
	}
	return provider.Changes{Cursor: "walk", Upserted: all}, nil
}

// Upload streams stdin through `rclone rcat`.
func (c *connector) Upload(ctx context.Context, acct provider.AccountRef, parentRemoteID, name string, r io.Reader, size int64, progress provider.ProgressFn) (provider.File, error) {
	cb, err := c.creds(acct)
	if err != nil {
		return provider.File{}, err
	}
	dst := name
	if parentRemoteID != "" {
		dst = parentRemoteID + "/" + name
	}
	full := append([]string{}, "--config", c.confPath, "--quiet", "rcat", rpath(cb.Remote, dst))
	cmd := exec.CommandContext(ctx, "rclone", full...)
	cmd.Stdin = r
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return provider.File{}, &runError{args: full[2:], exit: err.Error(), stderr: stderr.String()}
	}
	if progress != nil {
		progress(size, size)
	}
	return provider.File{RemoteID: dst, ParentID: parentRemoteID, Name: name, Size: size}, nil
}

// Open streams stdout from `rclone cat`.
func (c *connector) Open(ctx context.Context, acct provider.AccountRef, remoteID string, progress provider.ProgressFn) (io.ReadCloser, error) {
	cb, err := c.creds(acct)
	if err != nil {
		return nil, err
	}
	full := append([]string{}, "--config", c.confPath, "--quiet", "cat", rpath(cb.Remote, remoteID))
	cmd := exec.CommandContext(ctx, "rclone", full...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("bridge: rclone cat: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return out, nil
}

func (c *connector) Mkdir(ctx context.Context, acct provider.AccountRef, parentRemoteID, name string) (provider.File, error) {
	cb, err := c.creds(acct)
	if err != nil {
		return provider.File{}, err
	}
	p := name
	if parentRemoteID != "" {
		p = parentRemoteID + "/" + name
	}
	if _, err := c.runCmd(ctx, "mkdir", rpath(cb.Remote, p)); err != nil {
		return provider.File{}, err
	}
	return provider.File{RemoteID: p, ParentID: parentRemoteID, Name: name, IsDir: true}, nil
}

func (c *connector) Move(ctx context.Context, acct provider.AccountRef, remoteID, newParentRemoteID, newName string) (provider.File, error) {
	cb, err := c.creds(acct)
	if err != nil {
		return provider.File{}, err
	}
	if newName == "" {
		newName = remoteID
		if i := strings.LastIndex(newName, "/"); i >= 0 {
			newName = newName[i+1:]
		}
	}
	to := newName
	if newParentRemoteID != "" {
		to = newParentRemoteID + "/" + newName
	}
	if _, err := c.runCmd(ctx, "moveto", rpath(cb.Remote, remoteID), rpath(cb.Remote, to)); err != nil {
		return provider.File{}, err
	}
	return provider.File{RemoteID: to, ParentID: newParentRemoteID, Name: newName}, nil
}

func (c *connector) Copy(ctx context.Context, acct provider.AccountRef, remoteID, newParentRemoteID, newName string) (provider.File, error) {
	cb, err := c.creds(acct)
	if err != nil {
		return provider.File{}, err
	}
	if newName == "" {
		newName = remoteID
		if i := strings.LastIndex(newName, "/"); i >= 0 {
			newName = newName[i+1:]
		}
	}
	to := newName
	if newParentRemoteID != "" {
		to = newParentRemoteID + "/" + newName
	}
	if _, err := c.runCmd(ctx, "copyto", rpath(cb.Remote, remoteID), rpath(cb.Remote, to)); err != nil {
		return provider.File{}, err
	}
	return provider.File{RemoteID: to, ParentID: newParentRemoteID, Name: newName}, nil
}

func (c *connector) Delete(ctx context.Context, acct provider.AccountRef, remoteID string) error {
	cb, err := c.creds(acct)
	if err != nil {
		return err
	}
	_, err = c.runCmd(ctx, "deletefile", rpath(cb.Remote, remoteID))
	return err
}

// ShareLink: not portable across rclone backends.
func (c *connector) ShareLink(ctx context.Context, acct provider.AccountRef, remoteID string, create bool) (string, error) {
	return "", provider.ErrUnsupported
}

// ParseConfigSnippet turns a user-pasted rclone config section into a
// bundle. Lines are `key = value`; `type` is mandatory. No name needed —
// the caller assigns one.
func ParseConfigSnippet(snippet string) (name, typ string, options map[string]string, err error) {
	options = map[string]string{}
	for _, line := range strings.Split(snippet, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name = strings.Trim(line, "[]")
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return "", "", nil, fmt.Errorf("bridge: bad config line %q", line)
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k == "type" {
			typ = v
			continue
		}
		options[k] = v
	}
	if typ == "" {
		return "", "", nil, fmt.Errorf("bridge: config snippet needs a `type = ...` line")
	}
	return name, typ, options, nil
}
