package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/demonkingswarn/luffy/core"
	"github.com/demonkingswarn/luffy/core/providers"
	"github.com/spf13/cobra"
)

var (
	seasonFlag   int
	episodeFlag  string
	actionFlag   string
	showImageFlag bool
)

func init() {
	rootCmd.Flags().IntVarP(&seasonFlag, "season", "s", 0, "Specify season number")
	rootCmd.Flags().StringVarP(&episodeFlag, "episodes", "e", "", "Specify episode or range (e.g. 1, 1-5)")
	rootCmd.Flags().StringVarP(&actionFlag, "action", "a", "", "Action to perform (play, download)")
	rootCmd.Flags().BoolVar(&showImageFlag, "show-image", false, "Show poster preview using chafa")
}

var rootCmd = &cobra.Command{
	Use:   "luffy [query]",
	Short: "Watch movies and TV shows from the commandline",
	Version: core.Version,
	Args:  cobra.ArbitraryArgs,

	RunE: func(cmd *cobra.Command, args []string) error {
		client := core.NewClient()
		ctx := &core.Context{
			Client: client,
		}

		cfg := core.LoadConfig()
		var provider core.Provider
		if strings.EqualFold(cfg.Provider, "sflix") {
			provider = providers.NewSflix(client)
		} else if strings.EqualFold(cfg.Provider, "hdrezka") {
			provider = providers.NewHDRezka(client)
		} else if strings.EqualFold(cfg.Provider, "braflix") {
			provider = providers.NewBraflix(client)
		} else {
			provider = providers.NewFlixHQ(client)
		}

		if len(args) == 0 {
			ctx.Query = core.Prompt("Search")
		} else {
			ctx.Query = strings.Join(args, " ")
		}

		results, err := provider.Search(ctx.Query)
		if err != nil {
			return err
		}

		var titles []string
		for _, r := range results {
			titles = append(titles, fmt.Sprintf("[%s] %s", r.Type, r.Title))
		}

		var idx int
		if showImageFlag {
			fmt.Println("Downloading posters...")
			var wg sync.WaitGroup
			for _, r := range results {
				wg.Add(1)
				go func(r core.SearchResult) {
					defer wg.Done()
					core.DownloadPoster(r.Poster, r.Title)
				}(r)
			}
			wg.Wait()

			cfg := core.LoadConfig()
			cacheDir, _ := core.GetCacheDir()
			previewCmd := fmt.Sprintf("chafa -f %s \"%s/$(echo {} | sed -E 's/^\\[.*\\] //' | sed -E 's/[^a-zA-Z0-9]+/_/g').jpg\"", cfg.ImageBackend, cacheDir)
			idx = core.SelectWithPreview("Results:", titles, previewCmd)
		} else {
			idx = core.Select("Results:", titles)
		}
		selected := results[idx]

		ctx.Title = selected.Title
		ctx.URL = selected.URL
		ctx.ContentType = selected.Type

		if showImageFlag {
			// Clean cache after selection is made and we don't need previews anymore
			go core.CleanCache()
		}

		fmt.Println("Selected:", ctx.Title)

		mediaID, err := provider.GetMediaID(ctx.URL)
		if err != nil {
			return err
		}

		var episodesToProcess []core.Episode

		if ctx.ContentType == core.Series {
			seasons, err := provider.GetSeasons(mediaID)
			if err != nil {
				return err
			}
			if len(seasons) == 0 {
				return fmt.Errorf("no seasons found")
			}

			var selectedSeason core.Season
			if seasonFlag > 0 {
				if seasonFlag > len(seasons) {
					return fmt.Errorf("season %d not found (max %d)", seasonFlag, len(seasons))
				}
				selectedSeason = seasons[seasonFlag-1]
			} else {
				var sNames []string
				for _, s := range seasons {
					sNames = append(sNames, s.Name)
				}
				sIdx := core.Select("Seasons:", sNames)
				selectedSeason = seasons[sIdx]
			}

			allEpisodes, err := provider.GetEpisodes(selectedSeason.ID, true)
			if err != nil {
				return err
			}
			if len(allEpisodes) == 0 {
				return fmt.Errorf("no episodes found")
			}

			if episodeFlag != "" {
				indices, err := core.ParseEpisodeRange(episodeFlag)
				if err != nil {
					return err
				}
				for _, i := range indices {
					if i < 1 || i > len(allEpisodes) {
						fmt.Printf("Episode %d out of range (max %d), skipping\n", i, len(allEpisodes))
						continue
					}
					episodesToProcess = append(episodesToProcess, allEpisodes[i-1])
				}
			} else {
				var eNames []string
				for _, e := range allEpisodes {
					eNames = append(eNames, e.Name)
				}
				eIdx := core.Select("Episodes:", eNames)
				episodesToProcess = append(episodesToProcess, allEpisodes[eIdx])
			}

		} else {
			// Movie logic
			// For movies, GetEpisodes returns the list of servers directly
			servers, err := provider.GetEpisodes(mediaID, false)
			if err != nil || len(servers) == 0 {
				return fmt.Errorf("could not find movie info")
			}
			// Store the servers in episodesToProcess temporarily, but treat them as servers later
			episodesToProcess = servers
		}

		// Determine action
		currentAction := actionFlag
		if currentAction == "" {
			actions := []string{"Play", "Download"}
			actIdx := core.Select("Action:", actions)
			currentAction = actions[actIdx]
		}
		currentAction = strings.ToLower(currentAction)

		processStream := func(link, name string) {
			var streamURL string
			var subtitles []string
			var err error

			if strings.EqualFold(cfg.Provider, "hdrezka") {
				// Parse HDRezka multi-quality string: [360p]url,[480p]url...
				// We want the best quality
				streams := strings.Split(link, ",")
				bestQuality := 0
				for _, s := range streams {
					s = strings.TrimSpace(s)
					if strings.HasPrefix(s, "[") {
						end := strings.Index(s, "]")
						if end > 1 {
							qualityStr := s[1:end]
							qualityStr = strings.TrimSuffix(qualityStr, "p")
							q, _ := strconv.Atoi(qualityStr)
							if q > bestQuality {
								bestQuality = q
								streamURL = s[end+1:]
							}
						}
					} else {
						// No quality tag? assume it's the url
						if streamURL == "" {
							streamURL = s
						}
					}
				}
				if streamURL == "" {
					// Fallback to raw link
					streamURL = link
				}
				// Fix protocol if needed
				if !strings.HasPrefix(streamURL, "http") {
					// Sometimes it might be missing http
				}
			} else {
				fmt.Println("Decrypting stream...")
				streamURL, subtitles, err = core.DecryptStream(link, ctx.Client)
				if err != nil {
					fmt.Printf("Decryption failed for %s: %v\n", name, err)
					return
				}
			}

			switch currentAction {
			case "play":
				err = core.Play(streamURL, name, link, subtitles)
				if err != nil {
					fmt.Println("Error playing:", err)
				}
			case "download":
				homeDir, _ := os.UserHomeDir()
				err = core.Download(homeDir, name, streamURL, link, subtitles)
				if err != nil {
					fmt.Println("Error downloading:", err)
				}
			default:
				fmt.Println("Unknown action:", currentAction)
			}
		}

		if ctx.ContentType == core.Movie {
			// Movie Processing
			fmt.Printf("\nProcessing: %s\n", ctx.Title)

			// episodesToProcess contains servers for movies
			var selectedServer core.Episode // abusing Episode struct for Server info
			if len(episodesToProcess) > 0 {
				selectedServer = episodesToProcess[0]
			}
			
			// Find preferred server
			for _, s := range episodesToProcess {
				// Prefer VidCloud for non-hdrezka, or maybe Default for hdrezka
				if strings.EqualFold(cfg.Provider, "hdrezka") {
					// Just pick first or specific one? HDRezka usually has specific translator names.
					// Let's stick to default (0)
					selectedServer = s
					break
				}
				if strings.Contains(strings.ToLower(s.Name), "vidcloud") {
					selectedServer = s
					break
				}
			}

			link, err := provider.GetLink(selectedServer.ID)
			if err != nil {
				return fmt.Errorf("error getting link: %v", err)
			}

			processStream(link, ctx.Title)

		} else {
			// Series Processing
			for _, ep := range episodesToProcess {
				fmt.Printf("\nProcessing: %s\n", ep.Name)

				servers, err := provider.GetServers(ep.ID)
				if err != nil {
					fmt.Println("Error fetching servers:", err)
					continue
				}
				if len(servers) == 0 {
					fmt.Println("No servers found")
					continue
				}

				selectedServer := servers[0]
				if !strings.EqualFold(cfg.Provider, "hdrezka") {
					for _, s := range servers {
						if strings.Contains(strings.ToLower(s.Name), "vidcloud") {
							selectedServer = s
							break
						}
					}
				}

				link, err := provider.GetLink(selectedServer.ID)
				if err != nil {
					fmt.Println("Error getting link:", err)
					continue
				}

				processStream(link, ctx.Title+" - "+ep.Name)
			}
		}

		return nil
	},

}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
	}
}

