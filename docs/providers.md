# Providers

Providers are the scraping layer that converts a search query into a playable stream URL. Each provider lives in `core/providers/` and implements the `core.Provider` interface. The active provider is selected at startup via the `--provider` flag or the `provider` config key.

## Provider Interface

```go
type Provider interface {
    Search(query string) ([]SearchResult, error)
    GetMediaID(url string) (string, error)
    GetSeasons(mediaID string) ([]Season, error)
    GetEpisodes(id string, isSeason bool) ([]Episode, error)
    GetServers(episodeID string) ([]Server, error)
    GetLink(serverID string) (string, error)
}
```

The call chain for every play session is:

```
Search → GetMediaID → GetSeasons (series) → GetEpisodes → GetServers → GetLink → DecryptStream
```

For movies, `GetSeasons` is skipped and `GetEpisodes` is called with `isSeason = false`.

## Providers

### FlixHQ (`flixhq.go`) — default

**Base URL**: `https://flixhq.to`

| Method | Mechanism |
|--------|-----------|
| `Search` | GET `/search/{slug}` (spaces → hyphens); scrapes `div.flw-item` cards; up to 10 results |
| `GetMediaID` | Fetches media page; extracts numeric ID from `#watch-block[data-id]`, then `div.detail_page-watch[data-id]`, then `#movie_id[value]` |
| `GetSeasons` | GET `/ajax/season/list/{mediaID}`; scrapes `.dropdown-item[data-id]` |
| `GetEpisodes` | GET `/ajax/season/episodes/{seasonID}` (series) or `/ajax/movie/episodes/{movieID}` (movie); scrapes `.nav-item a[data-id]`, falls back to `a.eps-item[data-id]` |
| `GetServers` | GET `/ajax/episode/servers/{episodeID}`; scrapes `.nav-item a[data-id]` |
| `GetLink` | GET `/ajax/episode/sources/{serverID}`; JSON response `{"link": "..."}` |

All requests set `Referer: https://flixhq.to/`. The embed link returned by `GetLink` is then passed to `DecryptStream`.

---

### Sflix (`sflix.go`)

**Base URL**: `https://sflix.is`

Sflix uses an extended ID scheme where context (media ID, media type) is appended to IDs with `|` separators and threaded through every call:

```
seasonID → "seasonID|mediaID|type"
episodeID → "episodeID|mediaID|type"
serverID  → stripped back to bare ID in GetLink
```

| Method | Mechanism |
|--------|-----------|
| `Search` | GET `/search/{slug}`; scrapes `div.flw-item` cards; also checks URL path (`/tv/` vs `/movie/`) to determine media type |
| `GetMediaID` | Fetches media page; same three selectors as FlixHQ |
| `GetSeasons` | GET `/ajax/season/list/{mediaID}`; scrapes `.dropdown-item, .ss-item`; appends `|mediaID|type` to each season ID |
| `GetEpisodes` | Season path: GET `/ajax/season/episodes/{id}`; scrapes `.eps-item`. Movie path: GET `/ajax/episode/list/{id}`; scrapes `.link-item` |
| `GetServers` | Parses episodeID into `(actualID, mediaID, type)`; routes to `/ajax/episode/list/` (movie) or `/ajax/episode/servers/` (TV); scrapes `.link-item, .ulclear > li` |
| `GetLink` | Strips `|`-suffix from serverID; GET `/ajax/episode/sources/{id}`; JSON `{"link": "..."}` |

**Important**: because the referer for sflix streams is the embedder's origin (not `sflix.is`), `cmd/root.go` overrides `referer` after calling `GetLink`:

```go
if parsedURL, err := url.Parse(link); err == nil {
    referer = fmt.Sprintf("%s://%s/", parsedURL.Scheme, parsedURL.Host)
}
```

---

### Braflix (`braflix.go`)

**Base URL**: `https://braflix.nl`

Braflix has a similar structure to FlixHQ but with some differences:

| Method | Mechanism |
|--------|-----------|
| `Search` | GET `/search/{slug}` (URL-encoded); scrapes `div.flw-item`; media type inferred from `/tv/` in href; extracts year from `div.film-infor` text |
| `GetMediaID` | Regex-extracts trailing numeric ID from URL (e.g. `-19722` → `19722`) |
| `GetSeasons` | GET `/ajax/season/list/{mediaID}`; scrapes `a.ss-item[data-id]` |
| `GetEpisodes` | Season: GET `/ajax/season/episodes/{id}`; scrapes `a.eps-item`. Movie: GET `/ajax/episode/list/{id}`; scrapes `a.link-item` |
| `GetServers` | GET `/ajax/episode/servers/{episodeID}`; scrapes `.link-item` |
| `GetLink` | Strips `|`-suffix; GET `/ajax/episode/sources/{id}`; JSON `{"link": "..."}` |

Braflix does not use `core.NewRequest`; it builds requests directly with a Chrome User-Agent and `X-Requested-With: XMLHttpRequest`. The referer override described for sflix also applies.

---

### Movies4u (`movies4u.go`)

**Base URL**: `https://movies4u.am`

Movies4u is a Bollywood-only download provider. It does not use the standard server/embed flow.

| Method | Mechanism |
|--------|-----------|
| `Search` | GET `/?s={query}` (spaces → `+`); scrapes `article.entry-card` |
| `GetMediaID` | Returns the URL unchanged (used as the movie page URL) |
| `GetSeasons` | Returns `nil, nil` (no TV support) |
| `GetEpisodes` | Fetches the movie page; scans `h5` headings for quality strings (`1080p`, `720p`, `480p`); picks the best nexdrive.top link |
| `GetServers` | Returns `nil, nil` |
| `GetLink` | Fetches the nexdrive page; follows vcloud.zip or fastdl.zip links through HubCloud to the final download URL |

The download chain for vcloud links: nexdrive page → vcloud.zip page (Hunter JS extract) → HubCloud page → final direct link.

---

### HDRezka (`hdrezka.go`) — experimental

**Base URL**: `https://hdrezka.website`

HDRezka is a Russian-language streaming site. It uses a proprietary ID encoding for stream URLs.

| Method | Mechanism |
|--------|-----------|
| `Search` | GET `/search/?q={query}`; scrapes `div.b-content__inline_item`; detects series via `span.cat.series` |
| `GetMediaID` | Returns the URL (prepends base URL if path-relative) |
| `GetSeasons` | Fetches media page; scrapes `ul.b-simple_seasons__list li[data-tab_id]`; appends `|{tabID}` to create composite ID |
| `GetEpisodes` | Splits `id` on `|` to get `(url, seasonID)`; scrapes `ul#simple-episodes-list-{seasonID} li`; creates composite `url|season|episodeID` IDs |
| `GetServers` | Splits episodeID into `(url, season, episode)`; fetches page; scrapes `ul#translators-list li[data-translator_id]`; falls back to regex on inline JS if no translators found |
| `GetLink` | Splits serverID into `(url, season, episode, translatorID)`; extracts numeric show ID from URL; POSTs to `/ajax/get_cdn_series/` or `/ajax/get_cdn_movie/`; decodes the `url` field with `Decode` |

`Decode` strips the `#h` prefix and splits on `//_//`, base64-decoding each chunk and discarding chunks that contain junk characters (`#!@$^%`).

---

### YouTube (`youtube.go`)

**Base URL**: `https://www.youtube.com`

YouTube support is provided for watching YouTube content through luffy's player integration. Quality selection and download use yt-dlp via the player or the `--download` flag.

| Method | Mechanism |
|--------|-----------|
| `Search` | GET `/results?search_query={query}`; extracts `ytInitialData` JSON from the HTML; navigates the `twoColumnSearchResultsRenderer` tree to collect `videoRenderer` entries |
| `GetMediaID` | Parses `?v=` query param from the URL; also handles `youtu.be/{id}` short URLs |
| `GetSeasons` | Returns a single stub season `{ID: "1", Name: "Video"}` |
| `GetEpisodes` | Returns a single stub episode `{ID: mediaID, Name: "Watch Video"}` |
| `GetServers` | Returns a single stub server `{ID: episodeID, Name: "YouTube"}` |
| `GetLink` | Returns `https://www.youtube.com/watch?v={serverID}` directly |

The YouTube URL is passed to the player as-is. MPV resolves YouTube URLs natively via yt-dlp; VLC uses its YouTube plugin.

## Adding a New Provider

1. Create `core/providers/newprovider.go` implementing all six `Provider` methods.
2. Follow the `newRequest` helper pattern — set `Referer` and `User-Agent` consistently.
3. Add a branch in `cmd/root.go`:

```go
} else if strings.EqualFold(providerName, "newprovider") {
    provider = providers.NewNewProvider(client)
}
```

4. If the provider's stream links require a non-default Referer for m3u8 fetching, add the override block in `cmd/root.go` alongside the sflix/braflix block.
