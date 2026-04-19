package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps a sql.DB instance for the application.
type DB struct {
	conn *sql.DB
}

// Open opens (or creates) the SQLite database in the OS user config directory.
func Open() (*DB, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("db: cannot locate config dir: %w", err)
	}

	appDir := filepath.Join(configDir, "TwitchStreamerTools")
	if err := os.MkdirAll(appDir, 0700); err != nil {
		return nil, fmt.Errorf("db: cannot create app dir: %w", err)
	}

	dbPath := filepath.Join(appDir, "twitch_tools.db")
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}

	// Single writer — avoid "database is locked" on Windows.
	conn.SetMaxOpenConns(1)

	d := &DB{conn: conn}
	if err := d.migrate(); err != nil {
		conn.Close()
		return nil, err
	}
	return d, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.conn.Close()
}

func (d *DB) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS auth (
			id                INTEGER PRIMARY KEY CHECK(id = 1),
			access_token      TEXT    NOT NULL DEFAULT '',
			refresh_token     TEXT    NOT NULL DEFAULT '',
			expires_at        INTEGER NOT NULL DEFAULT 0,
			user_id           TEXT    NOT NULL DEFAULT '',
			user_login        TEXT    NOT NULL DEFAULT '',
			user_display_name TEXT    NOT NULL DEFAULT '',
			profile_image_url TEXT    NOT NULL DEFAULT '',
			offline_image_url TEXT    NOT NULL DEFAULT '',
			created_at        INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY NOT NULL,
			value TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS poll_archive (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			poll_id    TEXT    NOT NULL DEFAULT '',
			title      TEXT    NOT NULL DEFAULT '',
			status     TEXT    NOT NULL DEFAULT '',
			duration   INTEGER NOT NULL DEFAULT 0,
			choices    TEXT    NOT NULL DEFAULT '[]',
			started_at TEXT    NOT NULL DEFAULT '',
			ended_at   TEXT    NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS poll_templates (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			name       TEXT    NOT NULL DEFAULT '',
			title      TEXT    NOT NULL DEFAULT '',
			choices    TEXT    NOT NULL DEFAULT '[]',
			duration   INTEGER NOT NULL DEFAULT 120,
			created_at INTEGER NOT NULL DEFAULT 0
		)`,
	}

	defaults := map[string]string{
		"chat_sound_enabled":      "true",
		"chat_sound_path":         "",
		"chat_sound_volume":       "1.0",
		"chat_filter_ignore_own":  "true",
		"chat_filter_cooldown_ms": "0",
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("db: migrate begin: %w", err)
	}
	defer tx.Rollback()

	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return fmt.Errorf("db: migrate exec: %w", err)
		}
	}

	for k, v := range defaults {
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO settings (key, value) VALUES (?, ?)`, k, v,
		); err != nil {
			return fmt.Errorf("db: seed setting %q: %w", k, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Idempotent column additions for existing databases.
	for _, col := range []string{
		`ALTER TABLE auth ADD COLUMN profile_image_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE auth ADD COLUMN offline_image_url TEXT NOT NULL DEFAULT ''`,
	} {
		d.conn.Exec(col) // ignore error — column may already exist
	}
	return nil
}

// AuthRow holds the single auth record.
type AuthRow struct {
	AccessToken     string
	RefreshToken    string
	ExpiresAt       int64
	UserID          string
	UserLogin       string
	UserDisplayName string
	ProfileImageURL string
	OfflineImageURL string
	CreatedAt       int64
}

// GetAuth returns the stored auth row, or nil if none exists.
func (d *DB) GetAuth() (*AuthRow, error) {
	row := d.conn.QueryRow(
		`SELECT access_token, refresh_token, expires_at, user_id, user_login, user_display_name,
		        profile_image_url, offline_image_url, created_at
		 FROM auth WHERE id = 1`,
	)
	a := &AuthRow{}
	err := row.Scan(&a.AccessToken, &a.RefreshToken, &a.ExpiresAt,
		&a.UserID, &a.UserLogin, &a.UserDisplayName,
		&a.ProfileImageURL, &a.OfflineImageURL, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get auth: %w", err)
	}
	return a, nil
}

// SaveAuth upserts the auth row.
func (d *DB) SaveAuth(a AuthRow) error {
	_, err := d.conn.Exec(
		`INSERT INTO auth (id, access_token, refresh_token, expires_at, user_id, user_login, user_display_name,
		                   profile_image_url, offline_image_url, created_at)
		 VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   access_token=excluded.access_token,
		   refresh_token=excluded.refresh_token,
		   expires_at=excluded.expires_at,
		   user_id=excluded.user_id,
		   user_login=excluded.user_login,
		   user_display_name=excluded.user_display_name,
		   profile_image_url=excluded.profile_image_url,
		   offline_image_url=excluded.offline_image_url,
		   created_at=excluded.created_at`,
		a.AccessToken, a.RefreshToken, a.ExpiresAt,
		a.UserID, a.UserLogin, a.UserDisplayName,
		a.ProfileImageURL, a.OfflineImageURL, a.CreatedAt,
	)
	return err
}

// ClearAuth removes the auth row entirely.
func (d *DB) ClearAuth() error {
	_, err := d.conn.Exec(`DELETE FROM auth WHERE id = 1`)
	return err
}

// ClearAuthKeepRefresh clears user and token data but preserves the refresh
// token so the app can silently re-authenticate on the next login without
// requiring the user to go through Device Code Flow again.
func (d *DB) ClearAuthKeepRefresh() error {
	_, err := d.conn.Exec(
		`UPDATE auth SET
		   access_token='', expires_at=0,
		   user_id='', user_login='', user_display_name='',
		   profile_image_url='', offline_image_url=''
		 WHERE id = 1`,
	)
	return err
}

// GetSetting returns the value for a settings key, or "" if missing.
func (d *DB) GetSetting(key string) (string, error) {
	var val string
	err := d.conn.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

// SaveSetting upserts a key/value setting.
func (d *DB) SaveSetting(key, value string) error {
	_, err := d.conn.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		key, value,
	)
	return err
}

// ─── Poll Archive ────────────────────────────────────────────────────────────

// ArchivedPollChoice is a single choice stored inside a poll_archive record.
type ArchivedPollChoice struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Votes int    `json:"votes"`
}

// ArchivedPoll is a completed poll stored in the local database.
type ArchivedPoll struct {
	ID        int64
	PollID    string
	Title     string
	Status    string
	Duration  int
	Choices   []ArchivedPollChoice
	StartedAt string
	EndedAt   string
	CreatedAt int64
}

// SavePollArchive inserts a completed poll into the archive.
// Duplicate poll_ids are ignored (idempotent).
func (d *DB) SavePollArchive(p ArchivedPoll) error {
	choicesJSON, err := json.Marshal(p.Choices)
	if err != nil {
		return fmt.Errorf("db: marshal poll choices: %w", err)
	}
	_, err = d.conn.Exec(
		`INSERT OR IGNORE INTO poll_archive
		 (poll_id, title, status, duration, choices, started_at, ended_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.PollID, p.Title, p.Status, p.Duration,
		string(choicesJSON), p.StartedAt, p.EndedAt, p.CreatedAt,
	)
	return err
}

// GetPollArchive returns all archived polls, newest first.
func (d *DB) GetPollArchive() ([]ArchivedPoll, error) {
	rows, err := d.conn.Query(
		`SELECT id, poll_id, title, status, duration, choices, started_at, ended_at, created_at
		 FROM poll_archive ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("db: get poll archive: %w", err)
	}
	defer rows.Close()

	var out []ArchivedPoll
	for rows.Next() {
		var p ArchivedPoll
		var choicesJSON string
		if err := rows.Scan(&p.ID, &p.PollID, &p.Title, &p.Status, &p.Duration,
			&choicesJSON, &p.StartedAt, &p.EndedAt, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("db: scan poll archive row: %w", err)
		}
		if err := json.Unmarshal([]byte(choicesJSON), &p.Choices); err != nil {
			p.Choices = nil
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ─── Poll Templates ──────────────────────────────────────────────────────────

// PollTemplate is a saved reusable poll configuration.
type PollTemplate struct {
	ID        int64
	Name      string
	Title     string
	Choices   []string
	Duration  int
	CreatedAt int64
}

// SavePollTemplate inserts a new poll template.
func (d *DB) SavePollTemplate(t PollTemplate) error {
	choicesJSON, err := json.Marshal(t.Choices)
	if err != nil {
		return fmt.Errorf("db: marshal template choices: %w", err)
	}
	_, err = d.conn.Exec(
		`INSERT INTO poll_templates (name, title, choices, duration, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		t.Name, t.Title, string(choicesJSON), t.Duration, t.CreatedAt,
	)
	return err
}

// GetPollTemplates returns all saved templates, newest first.
func (d *DB) GetPollTemplates() ([]PollTemplate, error) {
	rows, err := d.conn.Query(
		`SELECT id, name, title, choices, duration, created_at
		 FROM poll_templates ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("db: get poll templates: %w", err)
	}
	defer rows.Close()

	var out []PollTemplate
	for rows.Next() {
		var t PollTemplate
		var choicesJSON string
		if err := rows.Scan(&t.ID, &t.Name, &t.Title, &choicesJSON, &t.Duration, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("db: scan poll template row: %w", err)
		}
		if err := json.Unmarshal([]byte(choicesJSON), &t.Choices); err != nil {
			t.Choices = nil
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeletePollTemplate removes a template by ID.
func (d *DB) DeletePollTemplate(id int64) error {
	_, err := d.conn.Exec(`DELETE FROM poll_templates WHERE id = ?`, id)
	return err
}
