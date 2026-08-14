// File operations API: unified browsing, search, upload with placement,
// streaming downloads, share links, mutations, cross-cloud transfers.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	r.Get("/file/{id}/download", a.download)
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

func (a *API) download(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
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
	rc, err := conn.Open(r.Context(), ref, row.RemoteID, nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, sanitizeHeader(row.Name)))
	w.Header().Set("Content-Type", "application/octet-stream")
	io.Copy(w, rc)
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
