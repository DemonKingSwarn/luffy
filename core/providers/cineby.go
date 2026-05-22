package providers

import (
	"net/http"

	"github.com/demonkingswarn/luffy/core"
	cinebyproviders "github.com/demonkingswarn/luffy/core/cineby_providers"
)

type Cineby struct {
	Client *http.Client
}

func NewCineby(client *http.Client) *Cineby {
	return &Cineby{Client: client}
}

func (c *Cineby) Search(query string) ([]core.SearchResult, error) {
	vp := cinebyproviders.NewVideasy(c.Client)
	return vp.Search(query)
}

func (c *Cineby) GetMediaID(mediaURL string) (string, error) {
	vp := cinebyproviders.NewVideasy(c.Client)
	return vp.GetMediaID(mediaURL)
}

func (c *Cineby) GetSeasons(mediaID string) ([]core.Season, error) {
	vp := cinebyproviders.NewVideasy(c.Client)
	return vp.GetSeasons(mediaID)
}

func (c *Cineby) GetEpisodes(id string, isSeason bool) ([]core.Episode, error) {
	vp := cinebyproviders.NewVideasy(c.Client)
	return vp.GetEpisodes(id, isSeason)
}

func (c *Cineby) GetServers(episodeID string) ([]core.Server, error) {
	vp := cinebyproviders.NewVideasy(c.Client)
	return vp.GetServers(episodeID)
}

func (c *Cineby) GetLink(serverID string) (string, error) {
	vp := cinebyproviders.NewVideasy(c.Client)
	return vp.GetLink(serverID)
}
