// Package index keeps the unified file index fresh: it applies provider
// change feeds into the files table and tracks per-account cursors.
package index

import (
	"context"
	"fmt"

	"github.com/pleumcloud/pleumcloud/internal/provider"
	"github.com/pleumcloud/pleumcloud/internal/store"
)

// Indexer applies connector change feeds to the store.
type Indexer struct {
	st *store.Store
}

// New builds the indexer.
func New(st *store.Store) *Indexer { return &Indexer{st: st} }

// Sync pulls one round of changes for an account and applies it.
// Cursor semantics: "" (first sync) and "walk" (providers without delta
// feeds) trigger a full replace; anything else is an incremental delta.
func (x *Indexer) Sync(ctx context.Context, acct provider.AccountRef, conn provider.Connector) (int, error) {
	cursor, err := x.st.LoadCursor(acct.ID)
	if err != nil {
		return 0, err
	}
	ch, err := conn.Changes(ctx, acct, cursor)
	if err != nil {
		return 0, fmt.Errorf("changes: %w", err)
	}

	if cursor == "" || cursor == "walk" {
		if err := x.st.ClearAccountFiles(acct.ID); err != nil {
			return 0, err
		}
	} else {
		for _, id := range ch.Deleted {
			if err := x.st.DeleteFileByRemote(acct.ID, id); err != nil {
				return 0, err
			}
		}
	}

	for _, f := range ch.Upserted {
		if err := x.st.UpsertFile(acct.ID, f.RemoteID, f.ParentID, f.Name, f.IsDir, f.Size, f.MIME, f.ModTime.Unix()); err != nil {
			return 0, err
		}
	}
	if err := x.st.SaveCursor(acct.ID, ch.Cursor); err != nil {
		return 0, err
	}
	if err := x.st.SetAccountSynced(acct.ID); err != nil {
		return 0, err
	}
	return len(ch.Upserted), nil
}

// SyncAll syncs every account (used by the background loop).
func (x *Indexer) SyncAll(ctx context.Context, deps provider.Deps, build func(id string, deps provider.Deps) (provider.Connector, bool)) map[string]error {
	accts, err := x.st.ListAccountsWithSecrets()
	if err != nil {
		return map[string]error{"*": err}
	}
	errs := map[string]error{}
	for _, a := range accts {
		conn, ok := build(a.ProviderID, deps)
		if !ok {
			errs[a.ID] = fmt.Errorf("no connector for %s", a.ProviderID)
			continue
		}
		if _, err := x.Sync(ctx, provider.AccountRef{ID: a.ID, ProviderID: a.ProviderID, SecretRef: a.SecretRef}, conn); err != nil {
			errs[a.ID] = err
		}
	}
	return errs
}
