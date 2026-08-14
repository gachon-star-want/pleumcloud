// Package store manages the local SQLite database: accounts, the unified
// file index, the transfer job queue and placement rules.
package store

import (
	crand "crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Store wraps the SQLite handle.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the database at path and applies migrations.
func Open(path string) (*Store, error) {
	// _pragma WAL and busy_timeout keep concurrent indexer/job workers safe.
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // modernc sqlite is happiest with a single writer
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Ping verifies database connectivity.
func (s *Store) Ping() error { return s.db.Ping() }

// Account is a connected cloud account (one per provider login; the same
// provider may appear multiple times with different labels).
type Account struct {
	ID           string     `json:"id"`
	ProviderID   string     `json:"providerId"`
	Label        string     `json:"label"`
	CreatedAt    time.Time  `json:"createdAt"`
	LastSyncedAt *time.Time `json:"lastSyncedAt,omitempty"`
}

type migration struct {
	version int
	stmts   []string
}

var migrations = []migration{
	{
		version: 1,
		stmts: []string{
			`CREATE TABLE accounts (
				id            TEXT PRIMARY KEY,
				provider_id   TEXT NOT NULL,
				label         TEXT NOT NULL DEFAULT '',
				auth_kind     TEXT NOT NULL,          -- oauth2 | pat | webdav
				secret_ref    TEXT NOT NULL,          -- keychain entry or fallback-file reference
				created_at    INTEGER NOT NULL,
				last_synced_at INTEGER
			)`,
			`CREATE INDEX idx_accounts_provider ON accounts(provider_id)`,

			`CREATE TABLE files (
				id               TEXT PRIMARY KEY,
				account_id       TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
				remote_id        TEXT NOT NULL,
				parent_remote_id TEXT NOT NULL DEFAULT '', -- '' = account root
				name             TEXT NOT NULL,
				is_dir           INTEGER NOT NULL DEFAULT 0,
				size             INTEGER NOT NULL DEFAULT 0,
				mime             TEXT NOT NULL DEFAULT '',
				mtime            INTEGER NOT NULL DEFAULT 0,
				thumb_state      TEXT NOT NULL DEFAULT 'none', -- none | cached | unsupported
				updated_at       INTEGER NOT NULL DEFAULT 0,
				UNIQUE(account_id, remote_id)
			)`,
			`CREATE INDEX idx_files_parent ON files(account_id, parent_remote_id)`,
			`CREATE INDEX idx_files_dir_first ON files(parent_remote_id, is_dir DESC, name)`,

			`CREATE VIRTUAL TABLE files_fts USING fts5(name, content='files', content_rowid='rowid', tokenize='unicode61')`,
			`CREATE TRIGGER files_ai AFTER INSERT ON files BEGIN
				INSERT INTO files_fts(rowid, name) VALUES (new.rowid, new.name);
			END`,
			`CREATE TRIGGER files_ad AFTER DELETE ON files BEGIN
				INSERT INTO files_fts(files_fts, rowid, name) VALUES ('delete', old.rowid, old.name);
			END`,
			`CREATE TRIGGER files_au AFTER UPDATE ON files BEGIN
				INSERT INTO files_fts(files_fts, rowid, name) VALUES ('delete', old.rowid, old.name);
				INSERT INTO files_fts(rowid, name) VALUES (new.rowid, new.name);
			END`,

			`CREATE TABLE sync_cursors (
				account_id TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
				cursor     TEXT NOT NULL DEFAULT '',
				updated_at INTEGER NOT NULL
			)`,

			`CREATE TABLE jobs (
				id             TEXT PRIMARY KEY,
				kind           TEXT NOT NULL,             -- upload | download | transfer | delete
				state          TEXT NOT NULL DEFAULT 'queued', -- queued | running | paused | done | failed
				src_account_id TEXT NOT NULL DEFAULT '',
				src_remote_id  TEXT NOT NULL DEFAULT '',
				src_path       TEXT NOT NULL DEFAULT '',
				dst_account_id TEXT NOT NULL DEFAULT '',
				dst_remote_id  TEXT NOT NULL DEFAULT '',
				dst_path       TEXT NOT NULL DEFAULT '',
				total_bytes    INTEGER NOT NULL DEFAULT 0,
				done_bytes     INTEGER NOT NULL DEFAULT 0,
				error          TEXT NOT NULL DEFAULT '',
				created_at     INTEGER NOT NULL,
				updated_at     INTEGER NOT NULL
			)`,
			`CREATE INDEX idx_jobs_state ON jobs(state, created_at)`,

			`CREATE TABLE rules (
				id          TEXT PRIMARY KEY,
				priority    INTEGER NOT NULL,
				enabled     INTEGER NOT NULL DEFAULT 1,
				match_field TEXT NOT NULL,   -- mime | name | size
				match_op    TEXT NOT NULL,   -- is | startswith | contains | gt | lt
				match_value TEXT NOT NULL,
				target      TEXT NOT NULL    -- provider account id, or 'most_free'
			)`,

			`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		},
	},
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	var current int
	row := s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`)
	if err := row.Scan(&current); err != nil {
		return err
	}
	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		for _, stmt := range m.stmts {
			if _, err := tx.Exec(stmt); err != nil {
				tx.Rollback()
				return fmt.Errorf("migration %d: %w", m.version, err)
			}
		}
		if _, err := tx.Exec(`INSERT INTO schema_version (version) VALUES (?)`, m.version); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// ListAccounts returns all connected accounts.
func (s *Store) ListAccounts() ([]Account, error) {
	rows, err := s.db.Query(`SELECT id, provider_id, label, created_at, last_synced_at FROM accounts ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		var a Account
		var created int64
		var syncedNull any
		if err := rows.Scan(&a.ID, &a.ProviderID, &a.Label, &created, &syncedNull); err != nil {
			return nil, err
		}
		a.CreatedAt = time.Unix(created, 0).UTC()
		if v, ok := syncedNull.(int64); ok {
			t := time.Unix(v, 0).UTC()
			a.LastSyncedAt = &t
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AccountRow is the full account record including secret location.
type AccountRow struct {
	Account
	AuthKind  string
	SecretRef string
}

// GetAccount returns one account by ID.
func (s *Store) GetAccount(id string) (AccountRow, error) {
	var row AccountRow
	var created int64
	var synced any
	err := s.db.QueryRow(
		`SELECT id, provider_id, label, created_at, last_synced_at, auth_kind, secret_ref
		 FROM accounts WHERE id = ?`, id,
	).Scan(&row.ID, &row.ProviderID, &row.Label, &created, &synced, &row.AuthKind, &row.SecretRef)
	if err != nil {
		return row, err
	}
	row.CreatedAt = time.Unix(created, 0).UTC()
	if v, ok := synced.(int64); ok {
		t := time.Unix(v, 0).UTC()
		row.LastSyncedAt = &t
	}
	return row, nil
}

// AddAccountWithID records a connected account under a caller-chosen ID
// (the API layer derives the secret ref from the same ID).
func (s *Store) AddAccountWithID(id, providerID, label, authKind, secretRef string) error {
	_, err := s.db.Exec(
		`INSERT INTO accounts (id, provider_id, label, auth_kind, secret_ref, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, providerID, label, authKind, secretRef, time.Now().Unix(),
	)
	return err
}

// DeleteAccount removes an account row (callers delete the secret too).
func (s *Store) DeleteAccount(id string) error {
	_, err := s.db.Exec(`DELETE FROM accounts WHERE id = ?`, id)
	return err
}

// SetAccountLabel updates the display label.
func (s *Store) SetAccountLabel(id, label string) error {
	_, err := s.db.Exec(`UPDATE accounts SET label = ? WHERE id = ?`, label, id)
	return err
}

// SetAccountSynced stamps the last successful sync time.
func (s *Store) SetAccountSynced(id string) error {
	_, err := s.db.Exec(`UPDATE accounts SET last_synced_at = ? WHERE id = ?`, time.Now().Unix(), id)
	return err
}

// SaveMeta writes a key/value setting.
func (s *Store) SaveMeta(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// LoadMeta reads a key/value setting; empty string when absent.
func (s *Store) LoadMeta(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// NewAccountID generates a random account ID.
func NewAccountID() (string, error) {
	b := make([]byte, 12)
	if _, err := crand.Read(b); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// ---- file index ----

// FileRow is an indexed file with its owning account (for UI badges).
type FileRow struct {
	ID             string `json:"id"`
	AccountID      string `json:"accountId"`
	ProviderID     string `json:"providerId"`
	AccountLabel   string `json:"accountLabel"`
	RemoteID       string `json:"remoteId"`
	ParentRemoteID string `json:"parentRemoteId"`
	Name           string `json:"name"`
	IsDir          bool   `json:"isDir"`
	Size           int64  `json:"size"`
	MIME           string `json:"mime,omitempty"`
	Mtime          int64  `json:"mtime"`
}

const fileCols = `f.id, f.account_id, a.provider_id, a.label, f.remote_id, f.parent_remote_id, f.name, f.is_dir, f.size, f.mime, f.mtime`

func scanFileRow(scan func(...any) error) (FileRow, error) {
	var r FileRow
	var isDir int
	err := scan(&r.ID, &r.AccountID, &r.ProviderID, &r.AccountLabel, &r.RemoteID, &r.ParentRemoteID, &r.Name, &isDir, &r.Size, &r.MIME, &r.Mtime)
	r.IsDir = isDir == 1
	return r, err
}

// UpsertFile inserts or updates an indexed entry.
func (s *Store) UpsertFile(accountID, remoteID, parentRemoteID, name string, isDir bool, size int64, mime string, mtime int64) error {
	id := newIDForFile()
	_, err := s.db.Exec(`INSERT INTO files (id, account_id, remote_id, parent_remote_id, name, is_dir, size, mime, mtime, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_id, remote_id) DO UPDATE SET
		  parent_remote_id = excluded.parent_remote_id,
		  name = excluded.name, is_dir = excluded.is_dir, size = excluded.size,
		  mime = excluded.mime, mtime = excluded.mtime, updated_at = excluded.updated_at`,
		id, accountID, remoteID, parentRemoteID, name, isDir, size, mime, mtime, time.Now().Unix())
	return err
}

func newIDForFile() string {
	b := make([]byte, 12)
	crand.Read(b)
	return "f" + hex.EncodeToString(b)
}

// DeleteFileByRemote removes one entry.
func (s *Store) DeleteFileByRemote(accountID, remoteID string) error {
	_, err := s.db.Exec(`DELETE FROM files WHERE account_id = ? AND remote_id = ?`, accountID, remoteID)
	return err
}

// ClearAccountFiles drops the whole index for an account (full re-walks).
func (s *Store) ClearAccountFiles(accountID string) error {
	_, err := s.db.Exec(`DELETE FROM files WHERE account_id = ?`, accountID)
	return err
}

// SaveCursor records a provider sync cursor.
func (s *Store) SaveCursor(accountID, cursor string) error {
	_, err := s.db.Exec(`INSERT INTO sync_cursors (account_id, cursor, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(account_id) DO UPDATE SET cursor = excluded.cursor, updated_at = excluded.updated_at`,
		accountID, cursor, time.Now().Unix())
	return err
}

// LoadCursor returns the saved cursor ("" when never synced).
func (s *Store) LoadCursor(accountID string) (string, error) {
	var c string
	err := s.db.QueryRow(`SELECT cursor FROM sync_cursors WHERE account_id = ?`, accountID).Scan(&c)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return c, err
}

// ListChildren returns indexed children of a folder. Empty parent = root.
// Empty account = unified view across accounts.
func (s *Store) ListChildren(parentRemoteID, accountID string) ([]FileRow, error) {
	q := `SELECT ` + fileCols + ` FROM files f JOIN accounts a ON a.id = f.account_id
		WHERE f.parent_remote_id = ?`
	args := []any{parentRemoteID}
	if accountID != "" {
		q += ` AND f.account_id = ?`
		args = append(args, accountID)
	}
	q += ` ORDER BY f.is_dir DESC, f.name COLLATE NOCASE`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FileRow
	for rows.Next() {
		r, err := scanFileRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SearchFiles runs an FTS5 query over all indexed names.
func (s *Store) SearchFiles(query string) ([]FileRow, error) {
	rows, err := s.db.Query(`SELECT `+fileCols+` FROM files_fts
		JOIN files f ON f.rowid = files_fts.rowid
		JOIN accounts a ON a.id = f.account_id
		WHERE files_fts MATCH ? ORDER BY rank LIMIT 200`, query+"*")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FileRow
	for rows.Next() {
		r, err := scanFileRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetFile fetches one indexed row by handle.
func (s *Store) GetFile(id string) (FileRow, error) {
	return scanFileRow(s.db.QueryRow(`SELECT `+fileCols+` FROM files f
		JOIN accounts a ON a.id = f.account_id WHERE f.id = ?`, id).Scan)
}

// ListAccountsWithSecrets returns accounts including secret references
// (for the sync loop; secret contents stay in the secret store).
func (s *Store) ListAccountsWithSecrets() ([]AccountRow, error) {
	rows, err := s.db.Query(`SELECT id, provider_id, label, created_at, COALESCE(last_synced_at, 0), auth_kind, secret_ref FROM accounts ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccountRow
	for rows.Next() {
		var r AccountRow
		var created, synced int64
		if err := rows.Scan(&r.ID, &r.ProviderID, &r.Label, &created, &synced, &r.AuthKind, &r.SecretRef); err != nil {
			return nil, err
		}
		r.CreatedAt = time.Unix(created, 0).UTC()
		t := time.Unix(synced, 0).UTC()
		r.LastSyncedAt = &t
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---- jobs ----

type JobRow struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	State       string `json:"state"`
	FileName    string `json:"fileName"`
	SrcAccount  string `json:"srcAccount"`
	DstAccount  string `json:"dstAccount"`
	DstProvider string `json:"dstProvider"`
	SrcRemote   string `json:"srcRemote"`
	TotalBytes  int64  `json:"totalBytes"`
	DoneBytes   int64  `json:"doneBytes"`
	Error       string `json:"error,omitempty"`
	CreatedAt   int64  `json:"createdAt"`
}

func (s *Store) AddJob(kind, fileName, srcAccount, srcRemoteID, dstAccount, dstProvider string, total int64) (string, error) {
	id := newIDForFile()
	_, err := s.db.Exec(`INSERT INTO jobs (id, kind, state, src_account_id, dst_account_id, src_path, dst_path, total_bytes, created_at, updated_at)
		VALUES (?, ?, 'queued', ?, ?, ?, ?, ?, ?, ?)`,
		id, kind, srcAccount, dstAccount, srcRemoteID, fileName, total, time.Now().Unix(), time.Now().Unix())
	return id, err
}

func (s *Store) UpdateJobProgress(id string, done, total int64) error {
	_, err := s.db.Exec(`UPDATE jobs SET done_bytes = ?, total_bytes = ?, updated_at = ? WHERE id = ?`, done, total, time.Now().Unix(), id)
	return err
}

func (s *Store) FinishJob(id, state, errMsg string) error {
	_, err := s.db.Exec(`UPDATE jobs SET state = ?, error = ?, updated_at = ? WHERE id = ?`, state, errMsg, time.Now().Unix(), id)
	return err
}

func (s *Store) ClaimNextQueuedJob() (*JobRow, error) {
	var j JobRow
	var dstProvider string
	err := s.db.QueryRow(`SELECT j.id, j.kind, j.state, j.src_path, j.dst_path, j.src_account_id, j.dst_account_id,
		COALESCE((SELECT a.provider_id FROM accounts a WHERE a.id = j.dst_account_id), ''), j.total_bytes, j.done_bytes, j.created_at
		FROM jobs j WHERE j.state = 'queued' ORDER BY j.created_at LIMIT 1`).
		Scan(&j.ID, &j.Kind, &j.State, &j.SrcRemote, &j.FileName, &j.SrcAccount, &j.DstAccount, &dstProvider, &j.TotalBytes, &j.DoneBytes, &j.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	j.DstProvider = dstProvider
	if _, err := s.db.Exec(`UPDATE jobs SET state = 'running', updated_at = ? WHERE id = ?`, time.Now().Unix(), j.ID); err != nil {
		return nil, err
	}
	j.State = "running"
	return &j, nil
}

func (s *Store) ListJobs(limit int) ([]JobRow, error) {
	rows, err := s.db.Query(`SELECT j.id, j.kind, j.state, j.src_path, j.dst_path, j.src_account_id, j.dst_account_id,
		COALESCE((SELECT a.provider_id FROM accounts a WHERE a.id = j.dst_account_id), ''), j.total_bytes, j.done_bytes, COALESCE(j.error, ''), j.created_at
		FROM jobs j ORDER BY j.created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []JobRow
	for rows.Next() {
		var j JobRow
		if err := rows.Scan(&j.ID, &j.Kind, &j.State, &j.SrcRemote, &j.FileName, &j.SrcAccount, &j.DstAccount, &j.DstProvider, &j.TotalBytes, &j.DoneBytes, &j.Error, &j.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// ---- rules ----

type RuleRow struct {
	ID       string `json:"id"`
	Priority int    `json:"priority"`
	Enabled  bool   `json:"enabled"`
	Field    string `json:"field"`
	Op       string `json:"op"`
	Value    string `json:"value"`
	Target   string `json:"target"`
}

func (s *Store) ListRules() ([]RuleRow, error) {
	rows, err := s.db.Query(`SELECT id, priority, enabled, match_field, match_op, match_value, target FROM rules ORDER BY priority`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RuleRow
	for rows.Next() {
		var r RuleRow
		var en int
		if err := rows.Scan(&r.ID, &r.Priority, &en, &r.Field, &r.Op, &r.Value, &r.Target); err != nil {
			return nil, err
		}
		r.Enabled = en == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) AddRule(priority int, enabled bool, field, op, value, target string) (string, error) {
	id := newIDForFile()
	en := 0
	if enabled {
		en = 1
	}
	_, err := s.db.Exec(`INSERT INTO rules (id, priority, enabled, match_field, match_op, match_value, target) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, priority, en, field, op, value, target)
	return id, err
}

func (s *Store) DeleteRule(id string) error {
	_, err := s.db.Exec(`DELETE FROM rules WHERE id = ?`, id)
	return err
}
