package core

import "net/http"

type EpisodeData struct {
	File    string
	Label   string
	Type    string
	Season  int
	Episode int
}

type Context struct {
	Client *http.Client

	Query       string
	URL         string
	Title       string
	ContentType MediaType

	Season   int
	Episodes []int

	SelectedMedia []EpisodeData
	PlayType      Action
	Debug         bool
}
