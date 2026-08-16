// File operations API: unified browsing, search, upload with placement,
// streaming downloads, share links, mutations, cross-cloud transfers.
package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

	"github.com/pleumcloud/pleumcloud/internal/placement"
	"github.com/pleumcloud/pleumcloud/internal/provider"
	"github.com/pleumcloud/pleumcloud/internal/store"
)

type (
	storeFile = store.FileRow
	jobRow    = store.JobRow
	ruleRow   = store.RuleRow
)

func jsonDecode(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func (a *API) registerFileRoutes(r chi.Router) {
	r.Get("/tree", a.requireUser(a.tree))
	r.Get("/search", a.requireUser(a.search))
	r.Get("/usage", a.requireUser(a.usage))
	r.Post("/sync", a.requireUser(a.syncNow))
	r.Post("/upload", a.requireUser(a.upload))
	r.Get("/jobs", a.requireUser(a.jobs))
	r.Post("/transfer", a.requireUser(a.transfer))
	r.Get("/rules", a.requireUser(a.listRules))
	r.Post("/rules", a.requireUser(a.addRule))
	r.Delete("/rules/{id}", a.requireUser(a.deleteRule))
	r.Post("/ops", a.requireUser(a.ops))
	r.Get("/file/{id}/download", a.requireUser(a.downloadInline))
	r.Get("/file/{id}/thumb", a.requireUser(a.thumb))
	r.Post("/file/{id}/share", a.requireUser(a.share))
}

// deps builds connector instances for the account's provider.
func (a *API) deps() provider.Deps { return provider.Deps{Secrets: a.secrets} }

func (a *API) connForUser(accountID, user string) (provider.Connector, provider.AccountRef, error) {
	row, err := a.store.GetAccountForUser(accountID, user)
	if err != nil {
		return nil, provider.AccountRef{}, fmt.Errorf("account not found")
	}
	conn, ok := provider.Build(row.ProviderID, a.deps())
	if !ok {
		return nil, provider.AccountRef{}, fmt.Errorf("no connector for %s", row.ProviderID)
	}
	return conn, provider.AccountRef{ID: row.ID, ProviderID: row.ProviderID, SecretRef: row.SecretRef}, nil
}

func (a *API) tree(w http.ResponseWriter, r *http.Request) {
	parent := r.URL.Query().Get("parent")
	account := r.URL.Query().Get("account")
	files, err := a.store.ListChildrenForUser(parent, account, userID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if files == nil {
		files = []storeFile{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}

func (a *API) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"results": []storeFile{}})
		return
	}
	res, err := a.store.SearchFilesForUser(q, userID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if res == nil {
		res = []storeFile{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": res})
}

// usage returns live quota per account, computed in parallel.
func (a *API) usage(w http.ResponseWriter, r *http.Request) {
	entries := a.usageEntries(r.Context(), userID(r))
	writeJSON(w, http.StatusOK, map[string]any{"usage": entries})
}

func (a *API) syncNow(w http.ResponseWriter, r *http.Request) {
	errs := map[string]error{}
	accts, err := a.store.ListAccountsWithSecrets()
	if err != nil {
		errs["*"] = err
	} else {
		for _, acct := range accts {
			owner, err2 := a.store.GetAccountForUser(acct.ID, userID(r))
			if err2 != nil {
				continue // not this user's account
			}
			if conn, ok := provider.Build(acct.ProviderID, a.deps()); ok {
				if _, err := a.indexer.Sync(r.Context(), provider.AccountRef{ID: acct.ID, ProviderID: acct.ProviderID, SecretRef: owner.SecretRef}, conn); err != nil {
					errs[acct.ID] = err
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"synced": true, "errors": errs})
}

// upload receives a multipart file, decides placement (target override or
// rules or most-free), stores it, and indexes the result.
func (a *API) upload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("multipart: %w", err))
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("file part missing"))
		return
	}
	defer file.Close()

	parentAccount := r.FormValue("parentAccount") // account owning the destination folder
	parentRemote := r.FormValue("parentRemote")   // "" = root
	targetOverride := r.FormValue("targetAccount")

	size := hdr.Size
	mime := hdr.Header.Get("Content-Type")

	var chosen string
	switch {
	case targetOverride != "":
		chosen = targetOverride
	case parentAccount != "":
		chosen = parentAccount // uploading into a specific folder targets its account
	default:
		free := a.liveFreeBytes(r.Context(), userID(r))
		var cands []placement.Account
		for acctID, f := range free {
			cands = append(cands, placement.Account{ID: acctID, Free: f})
		}
		chosen = placement.Decide(placement.FileInfo{Name: hdr.Filename, Size: size, MIME: mime}, cands, a.rulesForEngineFor(userID(r)))
	}
	if chosen == "" {
		writeErr(w, http.StatusUnprocessableEntity, fmt.Errorf("no account can hold this file"))
		return
	}

	conn, ref, err := a.connForUser(chosen, userID(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	f, err := conn.Upload(r.Context(), ref, parentRemote, hdr.Filename, file, size, nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	// Connectors without MIME round-trips (plain WebDAV) keep the multipart type.
	if f.MIME == "" && mime != "" && mime != "application/octet-stream" {
		f.MIME = mime
	}
	if err := a.store.UpsertFile(chosen, f.RemoteID, f.ParentID, f.Name, f.IsDir, f.Size, f.MIME, f.ModTime.Unix()); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"stored": f, "account": chosen})
}

type usageEntry struct {
	AccountID  string `json:"accountId"`
	ProviderID string `json:"providerId"`
	TotalBytes int64  `json:"totalBytes"`
	UsedBytes  int64  `json:"usedBytes"`
	Error      string `json:"error,omitempty"`
}

// usageEntries fetches live quotas for the user's accounts in parallel.
func (a *API) usageEntries(ctx context.Context, user string) []usageEntry {
	accts, err := a.store.ListAccountsWithSecrets()
	if err != nil {
		return nil
	}
	owned := map[string]store.AccountRow{}
	for _, ac := range accts {
		if row, err := a.store.GetAccountForUser(ac.ID, user); err == nil {
			owned[ac.ID] = row
		}
	}
	var filtered []store.AccountRow
	for _, ac := range accts {
		if row, ok := owned[ac.ID]; ok {
			filtered = append(filtered, row)
		}
	}
	entries := make([]usageEntry, len(filtered))
	var wg sync.WaitGroup
	for i, ac := range filtered {
		wg.Add(1)
		go func(i int, acctID, providerID, secretRef string) {
			defer wg.Done()
			e := usageEntry{AccountID: acctID, ProviderID: providerID}
			conn, ok := provider.Build(providerID, a.deps())
			if !ok {
				e.Error = "no connector"
			} else {
				cctx, cancel := context.WithTimeout(ctx, 12*time.Second)
				defer cancel()
				q, err := conn.Quota(cctx, provider.AccountRef{ID: acctID, ProviderID: providerID, SecretRef: secretRef})
				switch {
				case err == nil:
					e.TotalBytes, e.UsedBytes = q.TotalBytes, q.UsedBytes
				case errors.Is(err, provider.ErrUnsupported):
					// No quota API (plain WebDAV): capacity unknown, not broken.
				default:
					e.Error = err.Error()
				}
			}
			entries[i] = e
		}(i, ac.ID, ac.ProviderID, ac.SecretRef)
	}
	wg.Wait()
	return entries
}

// unknownFree stands in for providers without a quota API: capacity is
// unknown, so placement treats them as generously sized (but ranked below
// every known-size account).
const unknownFree = 1 << 40

// liveFreeBytes returns free space per account (placement input).
func (a *API) liveFreeBytes(ctx context.Context, user string) map[string]int64 {
	out := map[string]int64{}
	for _, e := range a.usageEntries(ctx, user) {
		if e.Error != "" {
			continue
		}
		if e.TotalBytes <= 0 {
			out[e.AccountID] = unknownFree
			continue
		}
		out[e.AccountID] = e.TotalBytes - e.UsedBytes
	}
	return out
}

func (a *API) rulesForEngineFor(user string) []placement.Rule {
	rows, err := a.store.ListRulesForUser(user)
	if err != nil {
		return nil
	}
	var out []placement.Rule
	for _, r := range rows {
		if !r.Enabled {
			continue
		}
		out = append(out, placement.Rule{Priority: r.Priority, Field: r.Field, Op: r.Op, Value: r.Value, Target: r.Target})
	}
	return out
}

func sanitizeHeader(s string) string {
	out := make([]rune, 0, len(s))
	for _, c := range s {
		if c == '"' || c == '\r' || c == '\n' || c == '\\' {
			continue
		}
		out = append(out, c)
	}
	return string(out)
}

func (a *API) share(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Create bool `json:"create"`
	}
	_ = jsonDecode(r, &req)
	row, err := a.store.GetFileForUser(id, userID(r))
	if err != nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("file not found"))
		return
	}
	conn, ref, err := a.connForUser(row.AccountID, userID(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	link, err := conn.ShareLink(r.Context(), ref, row.RemoteID, req.Create)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"link": link, "revoked": !req.Create})
}

// ops handles unified mutations on indexed files.
func (a *API) ops(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Op           string `json:"op"`
		ID           string `json:"id"`           // file handle (mutations)
		Account      string `json:"account"`      // mkdir
		ParentRemote string `json:"parentRemote"` // mkdir parent
		Name         string `json:"name"`         // mkdir/rename
		NewParentID  string `json:"newParentId"`  // move/copy destination folder handle
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	ctx := r.Context()

	switch req.Op {
	case "mkdir":
		conn, ref, err := a.connForUser(req.Account, userID(r))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		f, err := conn.Mkdir(ctx, ref, req.ParentRemote, req.Name)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err)
			return
		}
		_ = a.store.UpsertFile(req.Account, f.RemoteID, f.ParentID, f.Name, true, 0, "", 0)
		writeJSON(w, http.StatusCreated, map[string]any{"created": f})

	case "rename", "move", "copy", "delete":
		row, err := a.store.GetFileForUser(req.ID, userID(r))
		if err != nil {
			writeErr(w, http.StatusNotFound, fmt.Errorf("file not found"))
			return
		}
		conn, ref, err := a.connForUser(row.AccountID, userID(r))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		newParentRemote, newName := "", req.Name
		if req.NewParentID != "" {
			dst, err := a.store.GetFileForUser(req.NewParentID, userID(r))
			if err != nil {
				writeErr(w, http.StatusBadRequest, fmt.Errorf("destination not found"))
				return
			}
			newParentRemote = dst.RemoteID
		}
		switch req.Op {
		case "rename":
			if newName == "" {
				writeErr(w, http.StatusBadRequest, fmt.Errorf("name required"))
				return
			}
			if _, err := conn.Move(ctx, ref, row.RemoteID, row.ParentRemoteID, newName); err != nil {
				writeErr(w, http.StatusBadGateway, err)
				return
			}
		case "move":
			if newParentRemote == "" {
				writeErr(w, http.StatusBadRequest, fmt.Errorf("newParentId required"))
				return
			}
			if _, err := conn.Move(ctx, ref, row.RemoteID, newParentRemote, ""); err != nil {
				writeErr(w, http.StatusBadGateway, err)
				return
			}
		case "copy":
			if newParentRemote == "" && newName == "" {
				writeErr(w, http.StatusBadRequest, fmt.Errorf("copy needs a destination"))
				return
			}
			if _, err := conn.Copy(ctx, ref, row.RemoteID, newParentRemote, newName); err != nil {
				writeErr(w, http.StatusBadGateway, err)
				return
			}
		case "delete":
			if err := conn.Delete(ctx, ref, row.RemoteID); err != nil {
				writeErr(w, http.StatusBadGateway, err)
				return
			}
			_ = a.store.DeleteFileByRemote(row.AccountID, row.RemoteID)
		}
		// Mutations changed remote state; refresh this account's index soon.
		go a.syncAccount(row.AccountID)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeErr(w, http.StatusBadRequest, fmt.Errorf("unknown op %q", req.Op))
	}
}

func (a *API) syncAccount(accountID string) {
	acct, err := a.store.GetAccount(accountID)
	if err != nil {
		return
	}
	conn, ok := provider.Build(acct.ProviderID, a.deps())
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	_, _ = a.indexer.Sync(ctx, provider.AccountRef{ID: acct.ID, ProviderID: acct.ProviderID, SecretRef: acct.SecretRef}, conn)
}

// transfer queues a cross-cloud copy as a server-side job.
func (a *API) transfer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FileID     string `json:"fileId"`
		DstAccount string `json:"dstAccount"`
		DstParent  string `json:"dstParent"` // dst account's folder remote id ("" root)
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	row, err := a.store.GetFile(req.FileID)
	if err != nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("file not found"))
		return
	}
	dst, err := a.store.GetAccountForUser(req.DstAccount, userID(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("destination account not found"))
		return
	}
	jobID, err := a.store.AddJobForUser(userID(r), "transfer", row.Name, row.AccountID, row.RemoteID, req.DstAccount, dst.ProviderID, row.Size)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": jobID})
}

func (a *API) jobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := a.store.ListJobsForUser(userID(r), 50)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if jobs == nil {
		jobs = []jobRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (a *API) listRules(w http.ResponseWriter, r *http.Request) {
	rules, err := a.store.ListRulesForUser(userID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if rules == nil {
		rules = []ruleRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": rules})
}

func (a *API) addRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Priority int    `json:"priority"`
		Enabled  *bool  `json:"enabled"`
		Field    string `json:"field"`
		Op       string `json:"op"`
		Value    string `json:"value"`
		Target   string `json:"target"`
	}
	if err := jsonDecode(r, &req); err != nil || req.Field == "" || req.Value == "" || req.Target == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("field, value and target are required"))
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.Priority == 0 {
		req.Priority = 100
	}
	id, err := a.store.AddRuleForUser(userID(r), req.Priority, enabled, req.Field, req.Op, req.Value, req.Target)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (a *API) deleteRule(w http.ResponseWriter, r *http.Request) {
	_ = a.store.DeleteRuleForUser(chi.URLParam(r, "id"), userID(r))
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// ---- preview & streaming ----

// RangeOpener is implemented by connectors that can serve byte ranges
// natively (enables video seeking without full downloads).
type RangeOpener interface {
	OpenRange(ctx context.Context, acct provider.AccountRef, remoteID string, start, length int64) (io.ReadCloser, error)
}

// download serves file bytes; ?inline=1 streams with the real MIME type so
// browsers can preview/play. Range requests are honored through
// RangeOpener-capable connectors.
func (a *API) downloadInline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	inline := r.URL.Query().Get("inline") == "1"
	row, err := a.store.GetFileForUser(id, userID(r))
	if err != nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("file not found"))
		return
	}
	conn, ref, err := a.connForUser(row.AccountID, userID(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	ctype := row.MIME
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	if !inline {
		ctype = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "private, max-age=0")

	if rng, ok := parseRange(r.Header.Get("Range"), row.Size); ok {
		if ro, ok := conn.(RangeOpener); ok {
			rc, err := ro.OpenRange(r.Context(), ref, row.RemoteID, rng.start, rng.length)
			if err == nil {
				defer rc.Close()
				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", rng.start, rng.start+rng.length-1, row.Size))
				w.Header().Set("Accept-Ranges", "bytes")
				w.Header().Set("Content-Length", strconv.FormatInt(rng.length, 10))
				w.WriteHeader(http.StatusPartialContent)
				io.Copy(w, rc)
				return
			}
			// Range failed upstream: fall through to a full 200 response.
		}
	}

	rc, err := conn.Open(r.Context(), ref, row.RemoteID, nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	defer rc.Close()
	w.Header().Set("Accept-Ranges", "bytes")
	if row.Size > 0 && !inline {
		w.Header().Set("Content-Length", strconv.FormatInt(row.Size, 10))
	}
	if !inline {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, sanitizeHeader(row.Name)))
	}
	io.Copy(w, rc)
}

type httpRange struct{ start, length int64 }

// parseRange understands single-range requests: bytes=a-b, bytes=a-, bytes=-n.
func parseRange(h string, size int64) (httpRange, bool) {
	if h == "" || size <= 0 {
		return httpRange{}, false
	}
	spec, ok := strings.CutPrefix(h, "bytes=")
	if !ok || strings.Contains(spec, ",") {
		return httpRange{}, false
	}
	starts, lens, found := strings.Cut(spec, "-")
	if !found {
		return httpRange{}, false
	}
	if starts == "" { // suffix: last N bytes
		n, err := strconv.ParseInt(lens, 10, 64)
		if err != nil || n <= 0 || n > size {
			return httpRange{}, false
		}
		return httpRange{start: size - n, length: n}, true
	}
	start, err := strconv.ParseInt(starts, 10, 64)
	if err != nil || start < 0 || start >= size {
		return httpRange{}, false
	}
	if lens == "" {
		return httpRange{start: start, length: size - start}, true
	}
	end, err := strconv.ParseInt(lens, 10, 64)
	if err != nil || end < start {
		return httpRange{}, false
	}
	if end >= size {
		end = size - 1
	}
	return httpRange{start: start, length: end - start + 1}, true
}

// ---- thumbnails ----

// heicConvert shells out to the OS's HEIC→JPEG converter: sips ships with
// macOS; heif-convert comes from libheif-examples on Linux. Var so tests
// can substitute a fake.
var heicConvert = func(ctx context.Context, in, out string, size int) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.CommandContext(ctx, "sips", "-s", "format", "jpeg", "--resampleWidth", strconv.Itoa(size), in, "--out", out)
	} else {
		cmd = exec.CommandContext(ctx, "heif-convert", in, out)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("HEIC converter unavailable (install libheif-examples for heif-convert): %w", err)
	}
	return nil
}

// thumb generates (once) and serves a local JPEG thumbnail for image files,
// uniformly across every provider: the source is fetched through the
// connector, downscaled in-process, and cached on disk. HEIC/HEIF sources —
// opaque both to the Go decoder and to most browsers — go through the OS
// converter first. ?size= picks the long-edge cap (default 384).
func (a *API) thumb(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row, err := a.store.GetFileForUser(id, userID(r))
	if err != nil || row.IsDir {
		writeErr(w, http.StatusNotFound, fmt.Errorf("file not found"))
		return
	}
	if !strings.HasPrefix(row.MIME, "image/") && !isImageName(row.Name) {
		writeErr(w, http.StatusUnsupportedMediaType, fmt.Errorf("not an image"))
		return
	}
	size := thumbSize(r.URL.Query().Get("size"))
	cachePath := filepath.Join(a.thumbDir, fmt.Sprintf("%s_%d.jpg", id, size))
	if b, err := os.ReadFile(cachePath); err == nil {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "private, max-age=86400")
		w.Write(b)
		return
	}

	conn, ref, err := a.connForUser(row.AccountID, userID(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	rc, err := conn.Open(r.Context(), ref, row.RemoteID, nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	defer rc.Close()
	img, err := decodeThumb(r.Context(), rc, size)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, fmt.Errorf("decode image: %v", err))
		return
	}
	thumb := scaleDown(img, size)
	if err := os.MkdirAll(a.thumbDir, 0o700); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, thumb, &jpeg.Options{Quality: 80}); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	_ = os.WriteFile(cachePath, buf.Bytes(), 0o600)
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Write(buf.Bytes())
}

// thumbSize clamps ?size= to a sane long-edge range.
func thumbSize(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 64 {
		return 384
	}
	if n > 2048 {
		return 2048
	}
	return n
}

// isHEIFHead reports whether the 12-byte prefix is an ISO-BMFF ftyp box
// with a HEIF-family brand (iPhone photos say "heic").
func isHEIFHead(b []byte) bool {
	if len(b) < 12 || !bytes.Equal(b[4:8], []byte("ftyp")) {
		return false
	}
	switch string(b[8:12]) {
	case "heic", "heix", "hevc", "heif", "mif1", "msf1":
		return true
	}
	return false
}

// decodeThumb decodes an image stream, routing HEIF payloads through the
// OS converter (they land as JPEG, which the stdlib then handles).
func decodeThumb(ctx context.Context, src io.Reader, size int) (image.Image, error) {
	br := bufio.NewReader(src)
	if head, err := br.Peek(12); err == nil && isHEIFHead(head) {
		return decodeHEIF(ctx, br, size)
	}
	img, _, err := image.Decode(br)
	return img, err
}

// decodeHEIF spools the HEIC stream to a temp file, converts it with the
// OS converter and decodes the resulting JPEG.
func decodeHEIF(ctx context.Context, src io.Reader, size int) (image.Image, error) {
	in, err := os.CreateTemp("", "pleumcloud-*.heic")
	if err != nil {
		return nil, err
	}
	inName := in.Name()
	defer os.Remove(inName)
	outName := inName[:len(inName)-len(".heic")] + ".jpg"
	defer os.Remove(outName)

	if _, err := io.Copy(in, src); err != nil {
		in.Close()
		return nil, err
	}
	if err := in.Close(); err != nil {
		return nil, err
	}
	if err := heicConvert(ctx, inName, outName, size); err != nil {
		return nil, err
	}
	f, err := os.Open(outName)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

func isImageName(name string) bool {
	switch {
	case strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"),
		strings.HasSuffix(name, ".png"), strings.HasSuffix(name, ".gif"),
		strings.HasSuffix(name, ".webp"), strings.HasSuffix(name, ".bmp"),
		strings.HasSuffix(name, ".heic"), strings.HasSuffix(name, ".heif"):
		return true
	}
	return false
}

// scaleDown shrinks img to fit max on the long edge (nearest neighbor).
func scaleDown(img image.Image, max int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= max && h <= max {
		return img
	}
	scale := float64(max) / float64(maxInt(w, h))
	nw, nh := int(float64(w)*scale), int(float64(h)*scale)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		sy := b.Min.Y + y*h/nh
		for x := 0; x < nw; x++ {
			sx := b.Min.X + x*w/nw
			dst.Set(x, y, img.At(sx, sy))
		}
	}
	return dst
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
