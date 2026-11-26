package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", path)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	conn.SetMaxOpenConns(1)

	if err := migrate(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func migrate(conn *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
	token_hash TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	expires_at INTEGER NOT NULL,
	created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS share_links (
	token TEXT PRIMARY KEY,
	root_id TEXT NOT NULL,
	rel_path TEXT NOT NULL,
	is_dir INTEGER NOT NULL DEFAULT 0,
	password_hash TEXT NOT NULL DEFAULT '',
	allow_download INTEGER NOT NULL DEFAULT 1,
	expires_at INTEGER NOT NULL,
	created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_share_links_expires ON share_links(expires_at);

CREATE TABLE IF NOT EXISTS media_progress (
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	root_id TEXT NOT NULL,
	rel_path TEXT NOT NULL,
	position_sec REAL NOT NULL DEFAULT 0,
	duration_sec REAL NOT NULL DEFAULT 0,
	updated_at INTEGER NOT NULL,
	PRIMARY KEY (user_id, root_id, rel_path)
);

CREATE TABLE IF NOT EXISTS mount_roots (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	host_path TEXT NOT NULL,
	read_only INTEGER NOT NULL DEFAULT 0,
	private_only INTEGER NOT NULL DEFAULT 0,
	sort_order INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL
);
`
	if _, err := conn.Exec(schema); err != nil {
		return fmt.Errorf("migrate sqlite schema: %w", err)
	}
	return nil
}
