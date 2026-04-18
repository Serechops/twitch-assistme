package db

import (
	"database/sql"
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
