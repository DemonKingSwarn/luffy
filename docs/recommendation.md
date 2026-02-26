# Recommendation Engine

Luffy's recommendation engine (`core/recommend.go`) builds a personalised list of unwatched movies and TV shows from your watch history using TMDB as the data source.

## How It Works

### 1. Load Watch History

The engine reads every unique title from the history database (`~/.config/luffy/history.sqlite`). Each entry carries a `WatchedAt` timestamp used in the next step.

### 2. Recency Weighting

Not all history entries are treated equally. Titles you watched recently have more influence on your recommendations than titles you watched months ago. The weight for each entry is computed with exponential decay:

```
weight = 2 ^ (-age / half_life)
```

Where `age` is the time since you watched the title and `half_life` is **30 days**. This means:

| Time since watched | Weight |
|--------------------|--------|
| Today              | ~1.0   |
| 30 days ago        | ~0.5   |
| 60 days ago        | ~0.25  |
| 90 days ago        | ~0.125 |

### 3. Build a Taste Profile

For each history entry the engine calls TMDB to resolve the title to a TMDB ID and media type, then fetches:

- **Genre IDs** via `GET /movie/{id}` or `GET /tv/{id}`
- **Keyword IDs** via `GET /movie/{id}/keywords` or `GET /tv/{id}/keywords`

Each genre and keyword ID is accumulated into two weighted frequency maps, with the recency weight from step 2:

```
profile.genres[genreID]   += weight
profile.keywords[keywordID] += weight
```

After processing all history entries, genres and keywords that appear repeatedly across many recently-watched titles will have the highest accumulated weights. This is the taste profile.

### 4. Discover Candidate Titles

The top 3 genres by accumulated weight are extracted from the profile. These are used to query TMDB's `/discover` endpoint for both movies and TV shows:

```
GET /discover/movie?with_genres=<top3>&sort_by=vote_average.desc&vote_count.gte=100
GET /discover/tv?with_genres=<top3>&sort_by=vote_average.desc&vote_count.gte=100
```

The `vote_count.gte=100` filter ensures the results are not obscure low-sample-size titles. Results are pre-sorted by TMDB vote average so the candidate pool starts with well-regarded titles.

Titles already in your watch history are excluded from the candidate pool immediately.

### 5. Score Each Candidate

Every candidate is scored against the taste profile:

```
score = Σ(genre_weight for matching genres)
      + 0.5 × Σ(keyword_weight for matching keywords)
```

Keywords are given half the weight of genres because they are more specific and niche — a genre match is a stronger signal of general taste alignment than a keyword match.

### 6. Rank and Return

Candidates are sorted by score descending. The top 50 are returned. Any candidate with a score of zero (no genre or keyword overlap with the profile at all) is filtered out. If every candidate scores zero, all are returned unfiltered so there is always something to show.

## Fallback Behaviour

If TMDB lookups fail for all history entries (no network, API key issue, empty history, etc.), the engine falls back to the original simpler approach:

For each watched title → call `GET /movie/{id}/recommendations` or `GET /tv/{id}/recommendations` → merge and deduplicate results.

This fallback produces reasonable results but is not personalised — it is TMDB's generic "viewers of X also watched" list with no scoring.

## TMDB API Calls Per Run

For a history of N unique shows, the engine makes at most:

| Call | Count |
|------|-------|
| `/search/multi` (title → TMDB ID) | N |
| `/movie/{id}` or `/tv/{id}` (genres) | N |
| `/movie/{id}/keywords` or `/tv/{id}/keywords` | N |
| `/discover/movie` | 1 |
| `/discover/tv` | 1 |
| `/movie/{id}` or `/tv/{id}` (genres for candidates) | up to 40 |
| `/movie/{id}/keywords` or `/tv/{id}/keywords` (for candidates) | up to 40 |

Total: **3N + 82** calls in the worst case. For a typical history of 10–20 titles this is 112–142 calls. All calls are sequential; there is currently no parallelism or caching.

## Key Types

```go
// Recommendation is a title suggested based on watch history.
type Recommendation struct {
    Title     string
    MediaType MediaType // Movie or Series
    Year      string
    Overview  string
    TmdbID    int
    Score     float64   // relevance score, higher is better
}

// tasteProfile holds weighted genre and keyword frequency maps.
type tasteProfile struct {
    genres   map[int]float64 // genre_id -> accumulated weight
    keywords map[int]float64 // keyword_id -> accumulated weight
}
```

## Public API

```go
// GetRecommendations returns up to 50 scored recommendations based on
// the user's watch history. Returns nil, nil if history is empty.
func GetRecommendations(client *http.Client) ([]Recommendation, error)

// FormatRecommendLabel returns the fzf display label for a recommendation.
// Format: "[movie] Title (Year)" or "[series] Title (Year)"
func FormatRecommendLabel(r Recommendation) string
```

## Internal Helpers

| Function | Purpose |
|----------|---------|
| `tmdbLookup` | Resolves a title string to a TMDB ID and media type via `/search/multi` |
| `tmdbFetchGenres` | Fetches genre IDs for a TMDB ID |
| `tmdbFetchKeywords` | Fetches keyword IDs for a TMDB ID |
| `tmdbDiscover` | Fetches a pool of candidates from `/discover` filtered by genre |
| `topNKeys` | Returns the top N keys from a `map[int]float64` by value |
| `getRecommendationsSimple` | Fallback: merges TMDB `/recommendations` lists across history |
| `tmdbSimpleRecommend` | Fetches the TMDB `/recommendations` list for a single title |
