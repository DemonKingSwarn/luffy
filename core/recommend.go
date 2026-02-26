package core

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Recommendation is a title suggested based on watch history.
type Recommendation struct {
	Title     string
	MediaType MediaType // Movie or Series
	Year      string
	Overview  string
	TmdbID    int
	Score     float64 // internal relevance score, higher is better
}

// tasteProfile holds weighted genre and keyword frequency maps built from
// the user's watch history. Weights are recency-decayed so recently watched
// titles influence the profile more than older ones.
type tasteProfile struct {
	genres   map[int]float64 // genre_id -> accumulated weight
	keywords map[int]float64 // keyword_id -> accumulated weight
}

// --- TMDB response types ---

type tmdbMultiResult struct {
	ID           int    `json:"id"`
	MediaType    string `json:"media_type"`
	Title        string `json:"title"` // movies
	Name         string `json:"name"`  // tv
	ReleaseDate  string `json:"release_date"`
	FirstAirDate string `json:"first_air_date"`
	Overview     string `json:"overview"`
}

type tmdbMultiResponse struct {
	Results []tmdbMultiResult `json:"results"`
}

type tmdbDetailsResponse struct {
	Genres []struct {
		ID int `json:"id"`
	} `json:"genres"`
}

type tmdbKeywordsResponse struct {
	// movies use "keywords", tv uses "results"
	Keywords []struct {
		ID int `json:"id"`
	} `json:"keywords"`
	Results []struct {
		ID int `json:"id"`
	} `json:"results"`
}

type tmdbDiscoverResponse struct {
	Results []struct {
		ID           int     `json:"id"`
		MediaType    string  `json:"media_type"` // only present in some endpoints
		Title        string  `json:"title"`
		Name         string  `json:"name"`
		ReleaseDate  string  `json:"release_date"`
		FirstAirDate string  `json:"first_air_date"`
		Overview     string  `json:"overview"`
		VoteAverage  float64 `json:"vote_average"`
		VoteCount    int     `json:"vote_count"`
		GenreIDs     []int   `json:"genre_ids"`
	} `json:"results"`
}

type tmdbKeywordItem struct {
	ID int `json:"id"`
}

// GetRecommendations builds a taste profile from the user's watch history and
// returns a ranked list of titles the user has not yet watched.
//
// Algorithm:
//  1. Load history; assign each show a recency weight (exponential decay).
//  2. For each show, fetch its TMDB genre and keyword IDs; accumulate into the
//     taste profile weighted by recency.
//  3. Derive the top genres from the profile and use /discover to pull a pool
//     of candidate movies and TV shows.
//  4. Score every candidate: sum of matched genre weight + keyword weight from
//     the profile, multiplied by a quality factor (vote_average * log(vote_count+1)).
//  5. Exclude already-watched titles, sort by score descending, return top 50.
func GetRecommendations(client *http.Client) ([]Recommendation, error) {
	db, err := OpenHistory()
	if err != nil {
		return nil, fmt.Errorf("recommend: could not open history: %w", err)
	}
	defer db.Close()

	shows, err := db.ListShows()
	if err != nil {
		return nil, fmt.Errorf("recommend: could not list history: %w", err)
	}
	if len(shows) == 0 {
		return nil, nil
	}

	// Build a set of watched titles (lowercase) for exclusion.
	watched := make(map[string]bool, len(shows))
	watchedIDs := make(map[int]bool)
	for _, s := range shows {
		watched[strings.ToLower(s.Title)] = true
	}

	// --- Step 1: resolve each history entry to a TMDB ID + media type ---
	type resolvedShow struct {
		tmdbID    int
		mediaType string // "movie" or "tv"
		weight    float64
	}

	now := time.Now()
	halfLife := 30 * 24 * time.Hour // 30-day half-life for recency decay

	var resolved []resolvedShow
	profile := tasteProfile{
		genres:   make(map[int]float64),
		keywords: make(map[int]float64),
	}

	for _, show := range shows {
		tmdbID, mediaType, err := tmdbLookup(client, show.Title)
		if err != nil || tmdbID == 0 {
			continue
		}

		// Recency weight: w = 2^(-age/halfLife). Ranges from 1.0 (just watched)
		// down toward 0 for very old watches.
		age := now.Sub(show.WatchedAt)
		weight := math.Pow(2, -age.Hours()/halfLife.Hours())

		watchedIDs[tmdbID] = true

		// Accumulate genres.
		genres, err := tmdbFetchGenres(client, tmdbID, mediaType)
		if err == nil {
			for _, g := range genres {
				profile.genres[g] += weight
			}
		}

		// Accumulate keywords.
		keywords, err := tmdbFetchKeywords(client, tmdbID, mediaType)
		if err == nil {
			for _, k := range keywords {
				profile.keywords[k] += weight
			}
		}

		resolved = append(resolved, resolvedShow{tmdbID, mediaType, weight})
	}

	if len(profile.genres) == 0 {
		// Fall back to the old simple approach if nothing could be resolved.
		return getRecommendationsSimple(client, shows, watched)
	}

	// --- Step 2: pick top genres to seed /discover ---
	topGenres := topNKeys(profile.genres, 3)

	// --- Step 3: pull candidate pools from /discover ---
	seen := make(map[int]bool)
	var candidates []Recommendation

	for _, mt := range []string{"movie", "tv"} {
		pool, err := tmdbDiscover(client, mt, topGenres)
		if err != nil {
			continue
		}
		for _, c := range pool {
			if seen[c.TmdbID] || watchedIDs[c.TmdbID] {
				continue
			}
			if watched[strings.ToLower(c.Title)] {
				continue
			}
			seen[c.TmdbID] = true
			candidates = append(candidates, c)
		}
	}

	// --- Step 4: score each candidate ---
	for i := range candidates {
		c := &candidates[i]
		genres, _ := tmdbFetchGenres(client, c.TmdbID, tmdbMediaTypeStr(c.MediaType))
		keywords, _ := tmdbFetchKeywords(client, c.TmdbID, tmdbMediaTypeStr(c.MediaType))

		var genreScore, keywordScore float64
		for _, g := range genres {
			genreScore += profile.genres[g]
		}
		for _, k := range keywords {
			keywordScore += profile.keywords[k]
		}
		c.Score = genreScore + keywordScore*0.5
	}

	// --- Step 5: sort and return top 50 ---
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
	if len(candidates) > 50 {
		candidates = candidates[:50]
	}

	// Filter out zero-score results (no overlap at all).
	var recs []Recommendation
	for _, c := range candidates {
		if c.Score > 0 {
			recs = append(recs, c)
		}
	}
	// If scoring wiped everything out, return unscored candidates so there's
	// always something to show.
	if len(recs) == 0 {
		recs = candidates
	}

	return recs, nil
}

// tmdbMediaTypeStr converts a MediaType to the TMDB media type string.
func tmdbMediaTypeStr(mt MediaType) string {
	if mt == Series {
		return "tv"
	}
	return "movie"
}

// topNKeys returns the top n keys from a float64 map by value, descending.
func topNKeys(m map[int]float64, n int) []int {
	type kv struct {
		k int
		v float64
	}
	var pairs []kv
	for k, v := range m {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].v > pairs[j].v })
	var out []int
	for i, p := range pairs {
		if i >= n {
			break
		}
		out = append(out, p.k)
	}
	return out
}

// tmdbLookup resolves a title to a TMDB ID and media type via /search/multi.
func tmdbLookup(client *http.Client, title string) (int, string, error) {
	endpoint := fmt.Sprintf("%s/search/multi?api_key=%s&query=%s&page=1",
		TMDB_BASE_URL, TMDB_API_KEY, url.QueryEscape(title))

	req, err := NewRequest("GET", endpoint)
	if err != nil {
		return 0, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	var res tmdbMultiResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return 0, "", err
	}

	// Prefer exact title match.
	for _, r := range res.Results {
		if r.MediaType != "movie" && r.MediaType != "tv" {
			continue
		}
		name := r.Title
		if name == "" {
			name = r.Name
		}
		if strings.EqualFold(name, title) {
			return r.ID, r.MediaType, nil
		}
	}
	// Fallback: first movie/tv hit.
	for _, r := range res.Results {
		if r.MediaType == "movie" || r.MediaType == "tv" {
			return r.ID, r.MediaType, nil
		}
	}
	return 0, "", nil
}

// tmdbFetchGenres returns genre IDs for a TMDB title.
func tmdbFetchGenres(client *http.Client, id int, mediaType string) ([]int, error) {
	endpoint := fmt.Sprintf("%s/%s/%d?api_key=%s", TMDB_BASE_URL, mediaType, id, TMDB_API_KEY)
	req, err := NewRequest("GET", endpoint)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var res tmdbDetailsResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(res.Genres))
	for _, g := range res.Genres {
		ids = append(ids, g.ID)
	}
	return ids, nil
}

// tmdbFetchKeywords returns keyword IDs for a TMDB title.
func tmdbFetchKeywords(client *http.Client, id int, mediaType string) ([]int, error) {
	endpoint := fmt.Sprintf("%s/%s/%d/keywords?api_key=%s", TMDB_BASE_URL, mediaType, id, TMDB_API_KEY)
	req, err := NewRequest("GET", endpoint)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var res tmdbKeywordsResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	// Movies use "keywords", TV uses "results".
	var ids []int
	for _, k := range res.Keywords {
		ids = append(ids, k.ID)
	}
	for _, k := range res.Results {
		ids = append(ids, k.ID)
	}
	return ids, nil
}

// tmdbDiscover fetches a pool of candidates from /discover filtered by genre.
func tmdbDiscover(client *http.Client, mediaType string, genreIDs []int) ([]Recommendation, error) {
	var genreParts []string
	for _, g := range genreIDs {
		genreParts = append(genreParts, fmt.Sprintf("%d", g))
	}
	genreParam := strings.Join(genreParts, ",")

	endpoint := fmt.Sprintf(
		"%s/discover/%s?api_key=%s&with_genres=%s&sort_by=vote_average.desc&vote_count.gte=100&page=1",
		TMDB_BASE_URL, mediaType, TMDB_API_KEY, genreParam,
	)

	req, err := NewRequest("GET", endpoint)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var res tmdbDiscoverResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	mt := Movie
	if mediaType == "tv" {
		mt = Series
	}

	var recs []Recommendation
	for _, r := range res.Results {
		title := r.Title
		if title == "" {
			title = r.Name
		}
		if title == "" {
			continue
		}
		year := r.ReleaseDate
		if year == "" {
			year = r.FirstAirDate
		}
		if len(year) > 4 {
			year = year[:4]
		}
		recs = append(recs, Recommendation{
			Title:     title,
			MediaType: mt,
			Year:      year,
			Overview:  r.Overview,
			TmdbID:    r.ID,
		})
	}
	return recs, nil
}

// getRecommendationsSimple is the fallback used when TMDB lookups all fail.
// It mirrors the original approach: fetch /recommendations for each history
// title and merge the results.
func getRecommendationsSimple(client *http.Client, shows []ShowSummary, watched map[string]bool) ([]Recommendation, error) {
	seen := make(map[int]bool)
	var recs []Recommendation

	for _, show := range shows {
		tmdbID, mediaType, err := tmdbLookup(client, show.Title)
		if err != nil || tmdbID == 0 {
			continue
		}
		candidates, err := tmdbSimpleRecommend(client, tmdbID, mediaType)
		if err != nil {
			continue
		}
		for _, c := range candidates {
			if seen[c.TmdbID] || watched[strings.ToLower(c.Title)] {
				continue
			}
			seen[c.TmdbID] = true
			recs = append(recs, c)
		}
	}
	return recs, nil
}

// tmdbSimpleRecommend fetches the TMDB /recommendations list for a single title.
func tmdbSimpleRecommend(client *http.Client, id int, mediaType string) ([]Recommendation, error) {
	endpoint := fmt.Sprintf("%s/%s/%d/recommendations?api_key=%s&page=1",
		TMDB_BASE_URL, mediaType, id, TMDB_API_KEY)

	req, err := NewRequest("GET", endpoint)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var res struct {
		Results []struct {
			ID           int     `json:"id"`
			MediaType    string  `json:"media_type"`
			Title        string  `json:"title"`
			Name         string  `json:"name"`
			ReleaseDate  string  `json:"release_date"`
			FirstAirDate string  `json:"first_air_date"`
			Overview     string  `json:"overview"`
			VoteAverage  float64 `json:"vote_average"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	mt := Movie
	if mediaType == "tv" {
		mt = Series
	}

	var recs []Recommendation
	for _, r := range res.Results {
		title := r.Title
		if title == "" {
			title = r.Name
		}
		if title == "" {
			continue
		}
		year := r.ReleaseDate
		if year == "" {
			year = r.FirstAirDate
		}
		if len(year) > 4 {
			year = year[:4]
		}
		recs = append(recs, Recommendation{
			Title:     title,
			MediaType: mt,
			Year:      year,
			Overview:  r.Overview,
			TmdbID:    r.ID,
		})
	}
	return recs, nil
}

// FormatRecommendLabel returns the fzf label for a recommendation.
func FormatRecommendLabel(r Recommendation) string {
	typeTag := "movie"
	if r.MediaType == Series {
		typeTag = "series"
	}
	if r.Year != "" {
		return fmt.Sprintf("[%s] %s (%s)", typeTag, r.Title, r.Year)
	}
	return fmt.Sprintf("[%s] %s", typeTag, r.Title)
}
