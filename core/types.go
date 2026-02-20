package core

type Action string
type MediaType string

const (
	ActionPlay     Action = "play"
	ActionDownload Action = "download"

	Movie  MediaType = "movie"
	Series MediaType = "series"
)

const (
	FLIXHQ_BASE_URL   = "https://flixhq.to"
	FLIXHQ_SEARCH_URL = FLIXHQ_BASE_URL + "/search"
	FLIXHQ_AJAX_URL   = FLIXHQ_BASE_URL + "/ajax"
	TMDB_API_KEY      = "653bb8af90162bd98fc7ee32bcbbfb3d"
	TMDB_BASE_URL     = "https://api.themoviedb.org/3"
)

type TmdbSearchResult struct {
	Results []struct {
		ID           int    `json:"id"`
		MediaType    string `json:"media_type"`
		Title        string `json:"title"`
		Name         string `json:"name"`
		PosterPath   string `json:"poster_path"`
		ReleaseDate  string `json:"release_date"`
		FirstAirDate string `json:"first_air_date"`
	} `json:"results"`
}

type TmdbSeason struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	SeasonNumber int    `json:"season_number"`
}

type TmdbShowDetails struct {
	Seasons []TmdbSeason `json:"seasons"`
}

type TmdbEpisode struct {
	ID            int    `json:"id"`
	EpisodeNumber int    `json:"episode_number"`
	Name          string `json:"name"`
}

type TmdbSeasonDetails struct {
	Episodes []TmdbEpisode `json:"episodes"`
}

type SearchResult struct {
	Title  string
	URL    string
	Type   MediaType
	Poster string
	Year   string
}

type Season struct {
	ID   string
	Name string
}

type Episode struct {
	ID   string
	Name string
}

type Server struct {
	ID   string
	Name string
}
