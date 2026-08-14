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
