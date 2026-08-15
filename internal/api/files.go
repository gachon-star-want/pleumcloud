// File operations API: unified browsing, search, upload with placement,
// streaming downloads, share links, mutations, cross-cloud transfers.
package api

import (
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
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

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
	r.Get("/tree", a.tree)
	r.Get("/search", a.search)
	r.Get("/usage", a.usage)
	r.Post("/sync", a.syncNow)
	r.Post("/upload", a.upload)
	r.Get("/jobs", a.jobs)
	r.Post("/transfer", a.transfer)
	r.Get("/rules", a.listRules)
	r.Post("/rules", a.addRule)
	r.Delete("/rules/{id}", a.deleteRule)
	r.Post("/ops", a.ops)
	r.Get("/file/{id}/download", a.downloadInline)
	r.Get("/file/{id}/thumb", a.thumb)
	r.Post("/file/{id}/share", a.share)
}

// deps builds connector instances for the account's provider.
func (a *API) deps() provider.Deps { return provider.Deps{Secrets: a.secrets} }

func (a *API) connFor(accountID string) (provider.Connector, provider.AccountRef, error) {
	row, err := a.store.GetAccount(accountID)
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
	files, err := a.store.ListChildren(parent, account)
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
	res, err := a.store.SearchFiles(q)
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
	entries := a.usageEntries(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"usage": entries})
}

func (a *API) syncNow(w http.ResponseWriter, r *http.Request) {
	errs := a.indexer.SyncAll(r.Context(), a.deps(), provider.Build)
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
		free := a.liveFreeBytes(r.Context())
		var cands []placement.Account
		for acctID, f := range free {
			cands = append(cands, placement.Account{ID: acctID, Free: f})
		}
		chosen = placement.Decide(placement.FileInfo{Name: hdr.Filename, Size: size, MIME: mime}, cands, a.rulesForEngine())
	}
	if chosen == "" {
		writeErr(w, http.StatusUnprocessableEntity, fmt.Errorf("no account can hold this file"))
		return
	}

	conn, ref, err := a.connFor(chosen)
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

// usageEntries fetches live quotas for every account in parallel.
func (a *API) usageEntries(ctx context.Context) []usageEntry {
	accts, err := a.store.ListAccountsWithSecrets()
	if err != nil {
		return nil
	}
	entries := make([]usageEntry, len(accts))
	var wg sync.WaitGroup
	for i, ac := range accts {
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
func (a *API) liveFreeBytes(ctx context.Context) map[string]int64 {
	out := map[string]int64{}
	for _, e := range a.usageEntries(ctx) {
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

func (a *API) rulesForEngine() []placement.Rule {
	rows, err := a.store.ListRules()
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
	row, err := a.store.GetFile(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("file not found"))
		return
	}
	conn, ref, err := a.connFor(row.AccountID)
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
		conn, ref, err := a.connFor(req.Account)
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
		row, err := a.store.GetFile(req.ID)
		if err != nil {
			writeErr(w, http.StatusNotFound, fmt.Errorf("file not found"))
			return
		}
		conn, ref, err := a.connFor(row.AccountID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		newParentRemote, newName := "", req.Name
		if req.NewParentID != "" {
			dst, err := a.store.GetFile(req.NewParentID)
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
	dst, err := a.store.GetAccount(req.DstAccount)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("destination account not found"))
		return
	}
	jobID, err := a.store.AddJob("transfer", row.Name, row.AccountID, row.RemoteID, req.DstAccount, dst.ProviderID, row.Size)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": jobID})
}

func (a *API) jobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := a.store.ListJobs(50)
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
	rules, err := a.store.ListRules()
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
	id, err := a.store.AddRule(req.Priority, enabled, req.Field, req.Op, req.Value, req.Target)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (a *API) deleteRule(w http.ResponseWriter, r *http.Request) {
	_ = a.store.DeleteRule(chi.URLParam(r, "id"))
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
	row, err := a.store.GetFile(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("file not found"))
		return
	}
	conn, ref, err := a.connFor(row.AccountID)
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

// thumb generates (once) and serves a local JPEG thumbnail for image files,
// uniformly across every provider: the source is fetched through the
// connector, downscaled in-process (pure stdlib), and cached on disk.
func (a *API) thumb(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row, err := a.store.GetFile(id)
	if err != nil || row.IsDir {
		writeErr(w, http.StatusNotFound, fmt.Errorf("file not found"))
		return
	}
	if !strings.HasPrefix(row.MIME, "image/") && !isImageName(row.Name) {
		writeErr(w, http.StatusUnsupportedMediaType, fmt.Errorf("not an image"))
		return
	}
	cachePath := filepath.Join(a.thumbDir, id+".jpg")
	if b, err := os.ReadFile(cachePath); err == nil {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "private, max-age=86400")
		w.Write(b)
		return
	}

	conn, ref, err := a.connFor(row.AccountID)
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
	img, _, err := image.Decode(rc)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, fmt.Errorf("decode image: %v", err))
		return
	}
	thumb := scaleDown(img, 384)
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

func isImageName(name string) bool {
	switch {
	case strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"),
		strings.HasSuffix(name, ".png"), strings.HasSuffix(name, ".gif"),
		strings.HasSuffix(name, ".webp"), strings.HasSuffix(name, ".bmp"):
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
