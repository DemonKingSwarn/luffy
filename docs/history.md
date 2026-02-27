# Watch History

Luffy's watch history system (`core/history.go`) records every title you play in a local SQLite database and exposes a summary view used by the `--history` flag to let you resume from where you left off.

## How It Works

### 1. Database Location and Opening

The database lives at `~/.config/luffy/history.sqlite`. When `OpenHistory` is called it:

1. Resolves `~/.config/luffy/` and creates the directory if it does not exist.
2. Opens (or creates) the SQLite file using the CGO-free `modernc.org/sqlite` driver — no C toolchain is required for any build target.
3. Runs `migrate()` to ensure the schema is up-to-date.

### 2. Schema and Migration

The schema is created idempotently with `CREATE TABLE IF NOT EXISTS`:

```sql
CREATE TABLE history (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    title      TEXT    NOT NULL,
    season     INTEGER NOT NULL DEFAULT 0,  -- 0 for movies
    episode    INTEGER NOT NULL DEFAULT 0,  -- 0 for movies
    ep_name    TEXT    NOT NULL DEFAULT '',
    url        TEXT    NOT NULL,            -- provider media URL (used to resume)
    provider   TEXT    NOT NULL DEFAULT '',
    watched_at DATETIME NOT NULL
)
```

The `provider` column was added after the initial release. The migration also runs `ALTER TABLE history ADD COLUMN provider` so that pre-existing databases from older versions are upgraded automatically without destroying any data. SQLite silently ignores the `ALTER TABLE` if the column already exists.

Conventions:
- `season = 0` and `episode = 0` mean the entry is a movie.
- `url` stores the provider-specific media page URL (e.g. `https://flixhq.to/tv/watch-...`). This URL is passed directly back to the provider when resuming, so the full provider flow is re-run from that point.

### 3. Recording a Watch

`AddEntry` inserts a single row into `history`. The `WatchedAt` timestamp is stored as UTC. Multiple rows for the same title are allowed — every episode watched generates its own row — which is how the engine tracks the most recently watched season and episode per show.

### 4. Listing Shows for --history

`ListShows` returns one row per unique `(title, url)` pair, ordered by most-recent watch descending. It uses a `GROUP BY` with `MAX(watched_at)` to collapse all individual episode rows into a single summary per show:

```sql
SELECT title, url, provider, season, episode, ep_name, MAX(watched_at) AS last_watched
FROM history
GROUP BY title, url
ORDER BY last_watched DESC
```

The `season` and `episode` values that come back are those of the most recently watched episode, which is what the `--history` path uses to auto-jump to the right season and prompt the user from there.

### 5. Display Labels

`FormatShowLabel` converts a `ShowSummary` into the fzf-visible string:

- **Series**: `"Breaking Bad  (last: S03E07)"`
- **Movie**: `"Inception"`

## Key Types

```go
// HistoryEntry is one watch event to be persisted.
type HistoryEntry struct {
    ID        int64
    Title     string
    Season    int       // 0 for movies
    Episode   int       // 0 for movies
    EpName    string    // episode display name, empty for movies
    URL       string    // provider media URL (used to resume)
    Provider  string    // e.g. "flixhq", "sflix"
    WatchedAt time.Time
}

// ShowSummary is one entry per unique (title, url) pair, carrying
// the most-recent watch details for the --history resume flow.
type ShowSummary struct {
    Title     string
    URL       string
    Provider  string
    Season    int
    Episode   int
    EpName    string
    WatchedAt time.Time
}

// DB wraps an open SQLite connection.
type DB struct {
    conn *sql.DB
}
```

## Public API

```go
// OpenHistory opens (or creates) the history database at
// ~/.config/luffy/history.sqlite. The caller must call db.Close()
// when finished.
func OpenHistory() (*DB, error)

// Close releases the database connection.
func (db *DB) Close() error

// AddEntry inserts one watch event into history.
func (db *DB) AddEntry(e HistoryEntry) error

// ListShows returns one ShowSummary per unique (title, url),
// ordered by most-recent watch time descending.
func (db *DB) ListShows() ([]ShowSummary, error)

// FormatShowLabel returns the fzf display label for a show.
// Movies: "Title". Series: "Title  (last: S01E03)".
func FormatShowLabel(s ShowSummary) string
```

## Internal Helpers

| Function | Purpose |
|----------|---------|
| `historyDBPath` | Resolves `~/.config/luffy/history.sqlite`, creating the config dir if needed |
| `migrate` | Creates the `history` table and applies column additions for schema upgrades |
| `parseTime` | Parses SQLite `DATETIME` strings in multiple possible formats into `time.Time` |
