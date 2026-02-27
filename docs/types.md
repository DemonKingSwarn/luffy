# Core Types

`core/types.go` defines the shared data types, string-typed enumerations, and global constants used across providers and the CLI layer. Nothing in this file performs I/O.

## Enumerations

```go
// Action is what the user wants to do with a resolved stream URL.
type Action string

const (
    ActionPlay     Action = "play"
    ActionDownload Action = "download"
)

// MediaType distinguishes movies from TV series.
type MediaType string

const (
    Movie  MediaType = "movie"
    Series MediaType = "series"
)
```

`Action` is set from the `--download` flag in `cmd/root.go`. `MediaType` is carried on `SearchResult` and drives the branching between the movie flow and the series (season/episode) flow.

## Global Constants

```go
const (
    FLIXHQ_BASE_URL   = "https://flixhq.to"
    FLIXHQ_SEARCH_URL = FLIXHQ_BASE_URL + "/search"
    FLIXHQ_AJAX_URL   = FLIXHQ_BASE_URL + "/ajax"

    TMDB_API_KEY  = "653bb8af90162bd98fc7ee32bcbbfb3d"
    TMDB_BASE_URL = "https://api.themoviedb.org/3"
)
```

FlixHQ constants are duplicated here and in `core/providers/flixhq.go`; the provider file's copies take precedence at the package level. The TMDB constants are used by `core/recommend.go` and `core/types.go`'s TMDB response types.

## Provider-Facing Types

These types are used as the return values of every Provider method and are the currency of data exchange between the provider layer and `cmd/root.go`.

```go
// SearchResult is one item returned by Provider.Search.
type SearchResult struct {
    Title  string
    URL    string    // provider-specific media page URL
    Type   MediaType // Movie or Series
    Poster string    // absolute URL to a poster image (may be empty)
    Year   string    // release year string (may be empty)
}

// Season is one season returned by Provider.GetSeasons.
type Season struct {
    ID   string // opaque; passed back to GetEpisodes
    Name string // display name shown in fzf (e.g. "Season 1")
}

// Episode is one episode (or movie server) returned by Provider.GetEpisodes.
type Episode struct {
    ID   string // opaque; passed back to GetServers
    Name string // episode title shown in fzf
}

// Server is one streaming server returned by Provider.GetServers.
type Server struct {
    ID   string // opaque; passed to GetLink
    Name string // server name shown in fzf (e.g. "Vidcloud")
}
```

Note: `Episode` has no `Number` field. The 1-based episode number within a season must be tracked externally using the `episodeWithNum` struct defined in `cmd/root.go`.

## TMDB Response Types

Used internally by `core/recommend.go` to deserialise TMDB API responses. These are not part of the Provider interface.

```go
// TmdbSearchResult is the response from GET /search/multi.
type TmdbSearchResult struct {
    Results []struct {
        ID           int    `json:"id"`
        MediaType    string `json:"media_type"`
        Title        string `json:"title"`        // movies
        Name         string `json:"name"`         // TV shows
        PosterPath   string `json:"poster_path"`
        ReleaseDate  string `json:"release_date"`  // movies
        FirstAirDate string `json:"first_air_date"` // TV shows
    } `json:"results"`
}

// TmdbSeason is one season entry in a TV show's details response.
type TmdbSeason struct {
    ID           int    `json:"id"`
    Name         string `json:"name"`
    SeasonNumber int    `json:"season_number"`
}

// TmdbShowDetails is the response from GET /tv/{id}.
type TmdbShowDetails struct {
    Seasons []TmdbSeason `json:"seasons"`
}

// TmdbEpisode is one episode entry in a season details response.
type TmdbEpisode struct {
    ID            int    `json:"id"`
    EpisodeNumber int    `json:"episode_number"`
    Name          string `json:"name"`
}

// TmdbSeasonDetails is the response from GET /tv/{id}/season/{n}.
type TmdbSeasonDetails struct {
    Episodes []TmdbEpisode `json:"episodes"`
}
```
