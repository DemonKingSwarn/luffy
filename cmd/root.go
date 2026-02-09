package cmd

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/demonkingswarn/luffy/core"
	"github.com/demonkingswarn/luffy/core/providers"
	"github.com/spf13/cobra"
)

var (
	seasonFlag    int
	episodeFlag   string
	actionFlag    string
	showImageFlag bool
	backendFlag   string
	cacheFlag     string
	providerFlag  string
	debugFlag     bool
	updateFlag    bool
)

const USER_AGENT = "luffy/1.0.14"

func init() {
	rootCmd.Flags().IntVarP(&seasonFlag, "season", "s", 0, "Specify season number")
	rootCmd.Flags().StringVarP(&episodeFlag, "episodes", "e", "", "Specify episode or range (e.g. 1, 1-5)")
	rootCmd.Flags().StringVarP(&actionFlag, "action", "a", "", "Action to perform (play, download)")
	rootCmd.Flags().BoolVar(&showImageFlag, "show-image", false, "Show poster preview using chafa")
	rootCmd.Flags().StringVarP(&providerFlag, "provider", "p", "", "Specify provider")
	rootCmd.Flags().BoolVarP(&debugFlag, "debug", "d", false, "Enable debug output")
	rootCmd.Flags().BoolVarP(&updateFlag, "update", "u", false, "Update Luffy")

	rootCmd.AddCommand(previewCmd)
	previewCmd.Flags().StringVar(&backendFlag, "backend", "sixel", "Image backend")
	previewCmd.Flags().StringVar(&cacheFlag, "cache", "", "Cache directory")
}

var rootCmd = &cobra.Command{
	Use:     "luffy [query]",
	Short:   "Watch movies and TV shows from the commandline",
	Version: core.Version,
	Args:    cobra.ArbitraryArgs,

	RunE: func(cmd *cobra.Command, args []string) error {
		if updateFlag {
			core.Update()
			return nil
		}

		client := core.NewClient()
		ctx := &core.Context{
			Client: client,
			Debug:  debugFlag,
		}

		cfg := core.LoadConfig()
		var providerName string
		if providerFlag != "" {
			providerName = providerFlag
		} else {
			providerName = cfg.Provider
		}

		var provider core.Provider
		if strings.EqualFold(providerName, "sflix") {
			provider = providers.NewSflix(client)
		} else if strings.EqualFold(providerName, "hdrezka") {
			provider = providers.NewHDRezka(client)
		} else if strings.EqualFold(providerName, "braflix") {
			provider = providers.NewBraflix(client)
		} else if strings.EqualFold(providerName, "brocoflix") {
			provider = providers.NewBrocoflix(client)
		} else if strings.EqualFold(providerName, "xprime") {
			provider = providers.NewXPrime(client)
		} else if strings.EqualFold(providerName, "movies4u") {
			provider = providers.NewMovies4u(client)
		} else if strings.EqualFold(providerName, "youtube") {
			provider = providers.NewYouTube(client)
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
			exe, _ := os.Executable()
			previewCmd := fmt.Sprintf("%s preview --backend %s --cache %s {}", exe, cfg.ImageBackend, cacheDir)
			idx = core.SelectWithPreview("Results:", titles, previewCmd)
		} else {
			idx = core.Select("Results:", titles)
		}
		selected := results[idx]

		ctx.Title = selected.Title
		ctx.URL = selected.URL
		ctx.ContentType = selected.Type

		if showImageFlag {
			go core.CleanCache()
		}

		fmt.Println("Selected:", ctx.Title)

		mediaID, err := provider.GetMediaID(ctx.URL)
		if err != nil {
			return err
		}

		// For sflix, append media type to mediaID to help with server detection
		// Format: "mediaID|type" (e.g., "39506|series" or "39506|movie")
		// Braflix doesn't need this as it uses the same endpoint for both
		if strings.EqualFold(providerName, "sflix") {
			mediaID = mediaID + "|" + string(ctx.ContentType)
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
			servers, err := provider.GetEpisodes(mediaID, false)
			if err != nil || len(servers) == 0 {
				return fmt.Errorf("could not find movie info")
			}
			episodesToProcess = servers
		}

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

			referer := link
			if strings.EqualFold(providerName, "hdrezka") {
				referer = ctx.URL
			}

			if strings.EqualFold(providerName, "hdrezka") {
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
						if streamURL == "" {
							streamURL = s
						}
					}
				}
				if streamURL == "" {
					streamURL = link
				}
				// Fix protocol if needed
				if !strings.HasPrefix(streamURL, "http") {
					// Sometimes it might be missing http
				}
			} else if strings.EqualFold(providerName, "movies4u") || strings.EqualFold(providerName, "youtube") {
				streamURL = link
			} else {
				if ctx.Debug {
					fmt.Println("Decrypting stream...")
				}
				var decryptedReferer string
				streamURL, subtitles, decryptedReferer, err = core.DecryptStream(link, ctx.Client)
				if err != nil {
					fmt.Printf("Decryption failed for %s: %v\n", name, err)
					return
				}
				if decryptedReferer != "" {
					referer = decryptedReferer
				}

				if strings.EqualFold(providerName, "sflix") || strings.EqualFold(providerName, "braflix") {
					// Use the main URL of the embed link as referrer
					if parsedURL, err := url.Parse(link); err == nil {
						referer = fmt.Sprintf("%s://%s/", parsedURL.Scheme, parsedURL.Host)
					} else {
						referer = link
					}
				}
			}

			if strings.Contains(streamURL, ".m3u8") {
				if ctx.Debug {
					fmt.Println("Checking for best quality stream...")
				}
				best, err := core.GetBestQualityM3U8(streamURL, ctx.Client)
				if err == nil {
					if ctx.Debug && best != streamURL {
						fmt.Printf("Upgraded quality: %s\n", best)
					}
					streamURL = best
				} else if ctx.Debug {
					fmt.Printf("Failed to parse m3u8: %v\n", err)
				}
			}

			switch currentAction {
			case "play":
				if ctx.Debug {
					fmt.Printf("Stream URL: %s\n", streamURL)
				}
				err = core.Play(streamURL, name, referer, USER_AGENT, subtitles, ctx.Debug)
				if err != nil {
					fmt.Println("Error playing:", err)
				}
			case "download":
				dlPath := cfg.DlPath
				homeDir, _ := os.UserHomeDir()
				if dlPath == "" {
					dlPath = homeDir
				}
				err = core.Download(homeDir, dlPath, name, streamURL, referer, USER_AGENT, subtitles, ctx.Debug)
				if err != nil {
					fmt.Println("Error downloading:", err)
				}
			default:
				fmt.Println("Unknown action:", currentAction)
			}
		}

		if ctx.ContentType == core.Movie {
			fmt.Printf("\nProcessing: %s\n", ctx.Title)

			var selectedServer core.Episode // abusing Episode struct for Server info
			if len(episodesToProcess) > 0 {
				selectedServer = episodesToProcess[0]
			}

			for _, s := range episodesToProcess {
				if strings.EqualFold(providerName, "hdrezka") {
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
				if !strings.EqualFold(providerName, "hdrezka") {
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

var previewCmd = &cobra.Command{
	Use:    "preview [title]",
	Short:  "Preview a poster for a title",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			return
		}
		title := strings.Join(args, " ")

		rePrefix := regexp.MustCompile(`^\[.*\] `)
		cleanTitle := rePrefix.ReplaceAllString(title, "")

		reSanitize := regexp.MustCompile(`[^a-zA-Z0-9]+`)
		safeTitle := reSanitize.ReplaceAllString(cleanTitle, "_")

		fullPath := filepath.Join(cacheFlag, safeTitle+".jpg")

		c := exec.Command("chafa", "-f", backendFlag, fullPath)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Run()
	},
}
