package core

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// HistoryEntry represents a single watch history record.
type HistoryEntry struct {
	ID       int64
	Title    string
	Season   int    // 0 for movies
	Episode  int    // 0 for movies
	EpName   string // episode display name, empty for movies
	URL      string // provider media URL (used to resume)
	Provider string // provider name (e.g. "flixhq", "sflix")

	WatchedAt time.Time
}

// DB wraps an open SQLite database for history operations.
type DB struct {
	conn *sql.DB
}

// historyDBPath returns ~/.config/luffy/history.sqlite.
func historyDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("history: could not determine home dir: %w", err)
	}
	dir := filepath.Join(home, ".config", "luffy")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("history: could not create config dir: %w", err)
	}
	return filepath.Join(dir, "history.sqlite"), nil
}

// OpenHistory opens (or creates) the history database.
func OpenHistory() (*DB, error) {
	path, err := historyDBPath()
	if err != nil {
		return nil, err
	}
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("history: open db: %w", err)
	}
	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, err
	}
	return db, nil
}

// Close releases the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	// Initial table creation.
	_, err := db.conn.Exec(`CREATE TABLE IF NOT EXISTS history (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		title       TEXT    NOT NULL,
		season      INTEGER NOT NULL DEFAULT 0,
		episode     INTEGER NOT NULL DEFAULT 0,
		ep_name     TEXT    NOT NULL DEFAULT '',
		url         TEXT    NOT NULL,
		provider    TEXT    NOT NULL DEFAULT '',
		watched_at  DATETIME NOT NULL
	)`)
	if err != nil {
		return err
	}

	// Add provider column to existing databases that predate it.
	_, _ = db.conn.Exec(`ALTER TABLE history ADD COLUMN provider TEXT NOT NULL DEFAULT ''`)

	return nil
}

// AddEntry inserts a new history record.
func (db *DB) AddEntry(e HistoryEntry) error {
	_, err := db.conn.Exec(
		`INSERT INTO history (title, season, episode, ep_name, url, provider, watched_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.Title, e.Season, e.Episode, e.EpName, e.URL, e.Provider, e.WatchedAt.UTC(),
	)
	return err
}

// ShowSummary is one entry per unique show (title+url), carrying the most
// recent watch details so the user can resume from where they left off.
type ShowSummary struct {
	Title    string
	URL      string
	Provider string
	Season   int
	Episode  int
	EpName   string

	WatchedAt time.Time
}

// ListShows returns one row per unique (title, url) pair, ordered by the
// most recent watch time descending. This is what --history displays.
func (db *DB) ListShows() ([]ShowSummary, error) {
	rows, err := db.conn.Query(`
		SELECT title, url, provider, season, episode, ep_name, MAX(watched_at) AS last_watched
		FROM history
		GROUP BY title, url
		ORDER BY last_watched DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shows []ShowSummary
	for rows.Next() {
		var s ShowSummary
		var watchedAt string
		if err := rows.Scan(&s.Title, &s.URL, &s.Provider, &s.Season, &s.Episode, &s.EpName, &watchedAt); err != nil {
			return nil, err
		}
		s.WatchedAt = parseTime(watchedAt)
		shows = append(shows, s)
	}
	return shows, rows.Err()
}

// FormatShowLabel returns a human-readable fzf label for a show summary.
func FormatShowLabel(s ShowSummary) string {
	if s.Season > 0 {
		return fmt.Sprintf("%s  (last: S%02dE%02d)", s.Title, s.Season, s.Episode)
	}
	return s.Title
}

func parseTime(s string) time.Time {
	for _, layout := range []string{
		"2006-01-02 15:04:05+00:00",
		"2006-01-02T15:04:05Z",
		time.RFC3339,
		"2006-01-02 15:04:05",
	} {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t
		}
	}
	return time.Time{}
}
