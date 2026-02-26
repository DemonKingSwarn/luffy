package cmd

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

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
	bestFlag      bool
	historyFlag   bool
	recommendFlag bool
)

// episodeWithNum pairs an episode with its 1-based number within the season.
// This is needed because core.Episode has no Number field, so the correct
// episode number must be tracked externally when building episodesToProcess.
type episodeWithNum struct {
	num int
	ep  core.Episode
}

const USER_AGENT = "luffy/1.0.14"

func init() {
	rootCmd.Flags().IntVarP(&seasonFlag, "season", "s", 0, "Specify season number")
	rootCmd.Flags().StringVarP(&episodeFlag, "episodes", "e", "", "Specify episode or range (e.g. 1, 1-5)")
	rootCmd.Flags().StringVarP(&actionFlag, "action", "a", "", "Action to perform (play, download)")
	rootCmd.Flags().BoolVar(&showImageFlag, "show-image", false, "Show poster preview using chafa")
	rootCmd.Flags().StringVarP(&providerFlag, "provider", "p", "", "Specify provider")
	rootCmd.Flags().BoolVarP(&debugFlag, "debug", "d", false, "Enable debug output")
	rootCmd.Flags().BoolVarP(&bestFlag, "best", "b", false, "Auto-select best quality")
	rootCmd.Flags().BoolVarP(&historyFlag, "history", "H", false, "Pick from watch history and resume")
	rootCmd.Flags().BoolVarP(&recommendFlag, "recommend", "r", false, "Show recommendations based on watch history")

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
		} else if strings.EqualFold(providerName, "movies4u") {
			provider = providers.NewMovies4u(client)
		} else if strings.EqualFold(providerName, "youtube") {
			provider = providers.NewYouTube(client)
		} else {
			provider = providers.NewFlixHQ(client)
		}

		// Open history DB once; non-fatal if it fails.
		histDB, histErr := core.OpenHistory()
		if histErr != nil && debugFlag {
			fmt.Printf("Warning: could not open history db: %v\n", histErr)
		}
		if histDB != nil {
			defer histDB.Close()
		}

		// --history: show unique shows from watch history, then resume.
		if historyFlag {
			if histDB == nil {
				return fmt.Errorf("history db unavailable: %v", histErr)
			}
			shows, err := histDB.ListShows()
			if err != nil {
				return fmt.Errorf("could not read history: %w", err)
			}
			if len(shows) == 0 {
				fmt.Println("No watch history found.")
				return nil
			}

			labels := make([]string, len(shows))
			for i, s := range shows {
				labels[i] = core.FormatShowLabel(s)
			}
			hIdx := core.Select("History:", labels)
			chosen := shows[hIdx]

			// Build the right provider from what was recorded in history.
			histProviderName := chosen.Provider
			if histProviderName == "" {
				histProviderName = providerName // fall back to current config
			}
			var histProvider core.Provider
			switch strings.ToLower(histProviderName) {
			case "sflix":
				histProvider = providers.NewSflix(client)
			case "hdrezka":
				histProvider = providers.NewHDRezka(client)
			case "braflix":
				histProvider = providers.NewBraflix(client)
			case "movies4u":
				histProvider = providers.NewMovies4u(client)
			case "youtube":
				histProvider = providers.NewYouTube(client)
			default:
				histProvider = providers.NewFlixHQ(client)
			}

			ctx.Title = chosen.Title
			ctx.URL = chosen.URL
			fmt.Println("Resuming:", ctx.Title)

			mediaID, err := histProvider.GetMediaID(ctx.URL)
			if err != nil {
				return err
			}
			if strings.EqualFold(histProviderName, "sflix") {
				mediaID = mediaID + "|series"
			}

			seasons, seasonsErr := histProvider.GetSeasons(mediaID)
			if seasonsErr != nil || len(seasons) == 0 {
				ctx.ContentType = core.Movie
			} else {
				ctx.ContentType = core.Series
			}

			var episodesToProcess []episodeWithNum
			selectedSeasonNum := 0

			if ctx.ContentType == core.Series {
				var selectedSeason core.Season
				if chosen.Season > 0 && chosen.Season <= len(seasons) {
					fmt.Printf("Last watched: Season %d, Episode %d\n", chosen.Season, chosen.Episode)
				}
				var sNames []string
				for _, s := range seasons {
					sNames = append(sNames, s.Name)
				}
				sIdx := core.Select("Seasons:", sNames)
				selectedSeason = seasons[sIdx]
				selectedSeasonNum = sIdx + 1

				allEpisodes, err := histProvider.GetEpisodes(selectedSeason.ID, true)
				if err != nil {
					return err
				}
				if len(allEpisodes) == 0 {
					return fmt.Errorf("no episodes found")
				}

				var eNames []string
				for _, e := range allEpisodes {
					eNames = append(eNames, e.Name)
				}
				eIdx := core.Select("Episodes:", eNames)
				episodesToProcess = append(episodesToProcess, episodeWithNum{num: eIdx + 1, ep: allEpisodes[eIdx]})

			} else {
				servers, err := histProvider.GetEpisodes(mediaID, false)
				if err != nil || len(servers) == 0 {
					return fmt.Errorf("could not find movie info")
				}
				for _, s := range servers {
					episodesToProcess = append(episodesToProcess, episodeWithNum{num: 0, ep: s})
				}
			}

			currentAction := actionFlag
			if currentAction == "" {
				actions := []string{"Play", "Download"}
				actIdx := core.Select("Action:", actions)
				currentAction = actions[actIdx]
			}
			currentAction = strings.ToLower(currentAction)

			processStream := buildProcessStream(ctx, cfg, histProviderName, currentAction, histDB, debugFlag, bestFlag)

			if ctx.ContentType == core.Movie {
				fmt.Printf("\nProcessing: %s\n", ctx.Title)
				var selectedServer core.Episode
				if len(episodesToProcess) > 0 {
					selectedServer = episodesToProcess[0].ep
				}
				for _, ewn := range episodesToProcess {
					if strings.EqualFold(histProviderName, "hdrezka") {
						selectedServer = ewn.ep
						break
					}
					if strings.Contains(strings.ToLower(ewn.ep.Name), "vidcloud") {
						selectedServer = ewn.ep
						break
					}
				}
				link, err := histProvider.GetLink(selectedServer.ID)
				if err != nil {
					return fmt.Errorf("error getting link: %v", err)
				}
				return processStream(link, ctx.Title, 0, 0, "")
			}

			for _, ewn := range episodesToProcess {
				ep := ewn.ep
				fmt.Printf("\nProcessing: %s\n", ep.Name)
				servers, err := histProvider.GetServers(ep.ID)
				if err != nil {
					fmt.Println("Error fetching servers:", err)
					continue
				}
				if len(servers) == 0 {
					fmt.Println("No servers found")
					continue
				}
				selectedServer := servers[0]
				if !strings.EqualFold(histProviderName, "hdrezka") {
					for _, s := range servers {
						if strings.Contains(strings.ToLower(s.Name), "vidcloud") {
							selectedServer = s
							break
						}
					}
				}
				link, err := histProvider.GetLink(selectedServer.ID)
				if err != nil {
					fmt.Println("Error getting link:", err)
					continue
				}
				processStream(link, ctx.Title+" - "+ep.Name, selectedSeasonNum, ewn.num, ep.Name) //nolint:errcheck
			}
			return nil
		}

		// --recommend: fetch TMDB recommendations from history, search the
		// configured provider for the picked title, then play/download normally.
		if recommendFlag {
			fmt.Println("Fetching recommendations from your watch history...")
			recs, err := core.GetRecommendations(client)
			if err != nil {
				return fmt.Errorf("could not fetch recommendations: %w", err)
			}
			if len(recs) == 0 {
				fmt.Println("No recommendations found. Watch more titles to get personalised suggestions.")
				return nil
			}

			labels := make([]string, len(recs))
			for i, r := range recs {
				labels[i] = core.FormatRecommendLabel(r)
			}
			rIdx := core.Select("Recommendations:", labels)
			chosen := recs[rIdx]

			fmt.Printf("Searching for \"%s\" on %s...\n", chosen.Title, providerName)
			results, err := provider.Search(chosen.Title)
			if err != nil || len(results) == 0 {
				return fmt.Errorf("could not find \"%s\" on provider %s", chosen.Title, providerName)
			}

			// Pick the best match: prefer exact title + matching media type.
			selected := results[0]
			for _, r := range results {
				if strings.EqualFold(r.Title, chosen.Title) && r.Type == chosen.MediaType {
					selected = r
					break
				}
				if strings.EqualFold(r.Title, chosen.Title) {
					selected = r
				}
			}

			ctx.Title = selected.Title
			ctx.URL = selected.URL
			ctx.ContentType = selected.Type
			fmt.Println("Selected:", ctx.Title)

			mediaID, err := provider.GetMediaID(ctx.URL)
			if err != nil {
				return err
			}
			if strings.EqualFold(providerName, "sflix") {
				mediaID = mediaID + "|" + string(ctx.ContentType)
			}

			var episodesToProcess []episodeWithNum
			selectedSeasonNum := 0

			if ctx.ContentType == core.Series {
				seasons, err := provider.GetSeasons(mediaID)
				if err != nil {
					return err
				}
				if len(seasons) == 0 {
					return fmt.Errorf("no seasons found")
				}
				var sNames []string
				for _, s := range seasons {
					sNames = append(sNames, s.Name)
				}
				sIdx := core.Select("Seasons:", sNames)
				selectedSeason := seasons[sIdx]
				selectedSeasonNum = sIdx + 1

				allEpisodes, err := provider.GetEpisodes(selectedSeason.ID, true)
				if err != nil {
					return err
				}
				if len(allEpisodes) == 0 {
					return fmt.Errorf("no episodes found")
				}
				var eNames []string
				for _, e := range allEpisodes {
					eNames = append(eNames, e.Name)
				}
				eIdx := core.Select("Episodes:", eNames)
				episodesToProcess = append(episodesToProcess, episodeWithNum{num: eIdx + 1, ep: allEpisodes[eIdx]})

			} else {
				servers, err := provider.GetEpisodes(mediaID, false)
				if err != nil || len(servers) == 0 {
					return fmt.Errorf("could not find movie info")
				}
				for _, s := range servers {
					episodesToProcess = append(episodesToProcess, episodeWithNum{num: 0, ep: s})
				}
			}

			currentAction := actionFlag
			if currentAction == "" {
				actions := []string{"Play", "Download"}
				actIdx := core.Select("Action:", actions)
				currentAction = actions[actIdx]
			}
			currentAction = strings.ToLower(currentAction)

			processStream := buildProcessStream(ctx, cfg, providerName, currentAction, histDB, debugFlag, bestFlag)

			if ctx.ContentType == core.Movie {
				fmt.Printf("\nProcessing: %s\n", ctx.Title)
				var selectedServer core.Episode
				if len(episodesToProcess) > 0 {
					selectedServer = episodesToProcess[0].ep
				}
				for _, ewn := range episodesToProcess {
					if strings.EqualFold(providerName, "hdrezka") {
						selectedServer = ewn.ep
						break
					}
					if strings.Contains(strings.ToLower(ewn.ep.Name), "vidcloud") {
						selectedServer = ewn.ep
						break
					}
				}
				link, err := provider.GetLink(selectedServer.ID)
				if err != nil {
					return fmt.Errorf("error getting link: %v", err)
				}
				return processStream(link, ctx.Title, 0, 0, "")
			}

			for _, ewn := range episodesToProcess {
				ep := ewn.ep
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
				processStream(link, ctx.Title+" - "+ep.Name, selectedSeasonNum, ewn.num, ep.Name) //nolint:errcheck
			}
			return nil
		}

		// --- Normal search flow ---

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
			// Use forward slashes in the preview command so it works correctly
			// on Windows where fzf runs the preview in a shell that may not
			// handle backslash-separated paths.
			exeFwd := strings.ReplaceAll(exe, `\`, `/`)
			cacheDirFwd := strings.ReplaceAll(cacheDir, `\`, `/`)
			previewCmd := fmt.Sprintf("%s preview --backend %s --cache %s {}", exeFwd, cfg.ImageBackend, cacheDirFwd)
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

		var episodesToProcess []episodeWithNum

		// Track season number for history.
		selectedSeasonNum := 0

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
				selectedSeasonNum = seasonFlag
			} else {
				var sNames []string
				for _, s := range seasons {
					sNames = append(sNames, s.Name)
				}
				sIdx := core.Select("Seasons:", sNames)
				selectedSeason = seasons[sIdx]
				selectedSeasonNum = sIdx + 1
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
					episodesToProcess = append(episodesToProcess, episodeWithNum{num: i, ep: allEpisodes[i-1]})
				}
			} else {
				var eNames []string
				for _, e := range allEpisodes {
					eNames = append(eNames, e.Name)
				}
				eIdx := core.Select("Episodes:", eNames)
				episodesToProcess = append(episodesToProcess, episodeWithNum{num: eIdx + 1, ep: allEpisodes[eIdx]})
			}

		} else {
			servers, err := provider.GetEpisodes(mediaID, false)
			if err != nil || len(servers) == 0 {
				return fmt.Errorf("could not find movie info")
			}
			for _, s := range servers {
				episodesToProcess = append(episodesToProcess, episodeWithNum{num: 0, ep: s})
			}
		}

		currentAction := actionFlag
		if currentAction == "" {
			actions := []string{"Play", "Download"}
			actIdx := core.Select("Action:", actions)
			currentAction = actions[actIdx]
		}
		currentAction = strings.ToLower(currentAction)

		processStream := buildProcessStream(ctx, cfg, providerName, currentAction, histDB, debugFlag, bestFlag)

		if ctx.ContentType == core.Movie {
			fmt.Printf("\nProcessing: %s\n", ctx.Title)

			var selectedServer core.Episode // abusing Episode struct for Server info
			if len(episodesToProcess) > 0 {
				selectedServer = episodesToProcess[0].ep
			}

			for _, ewn := range episodesToProcess {
				if strings.EqualFold(providerName, "hdrezka") {
					selectedServer = ewn.ep
					break
				}
				if strings.Contains(strings.ToLower(ewn.ep.Name), "vidcloud") {
					selectedServer = ewn.ep
					break
				}
			}

			link, err := provider.GetLink(selectedServer.ID)
			if err != nil {
				return fmt.Errorf("error getting link: %v", err)
			}

			if err := processStream(link, ctx.Title, 0, 0, ""); err != nil {
				return err
			}

		} else {
			// Series Processing
			for _, ewn := range episodesToProcess {
				ep := ewn.ep
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

				if err := processStream(link, ctx.Title+" - "+ep.Name, selectedSeasonNum, ewn.num, ep.Name); err != nil {
					continue
				}
			}
		}

		return nil
	},
}

// buildProcessStream returns a closure that decrypts, quality-selects, plays/downloads
// a stream and saves a history entry on success.
func buildProcessStream(
	ctx *core.Context,
	cfg *core.Config,
	providerName string,
	currentAction string,
	histDB *core.DB,
	debugMode bool,
	best bool,
) func(link, name string, season, episode int, epName string) error {
	return func(link, name string, season, episode int, epName string) error {
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
		} else if strings.EqualFold(providerName, "movies4u") || strings.EqualFold(providerName, "youtube") {
			streamURL = link
		} else {
			if debugMode {
				fmt.Println("Decrypting stream...")
			}
			var decryptedReferer string
			streamURL, subtitles, decryptedReferer, err = core.DecryptStream(link, ctx.Client)
			if err != nil {
				fmt.Printf("Decryption failed for %s: %v\n", name, err)
				return err
			}
			if decryptedReferer != "" {
				referer = decryptedReferer
			}

			if strings.EqualFold(providerName, "sflix") || strings.EqualFold(providerName, "braflix") {
				if parsedURL, err := url.Parse(link); err == nil {
					referer = fmt.Sprintf("%s://%s/", parsedURL.Scheme, parsedURL.Host)
				} else {
					referer = link
				}
			}
		}

		if strings.Contains(streamURL, ".m3u8") {
			if debugMode {
				fmt.Println("Fetching available qualities...")
				fmt.Printf("Master m3u8 URL: %s\n", streamURL)
				fmt.Printf("Referer: %s\n", referer)
			}
			qualities, directURL, err := core.GetQualities(streamURL, ctx.Client, referer)
			if err != nil {
				if debugMode {
					fmt.Printf("Failed to parse m3u8: %v\n", err)
				}
			} else if len(qualities) > 0 {
				if debugMode {
					fmt.Printf("Found %d quality variants\n", len(qualities))
				}
				selectBest := best || strings.EqualFold(cfg.Quality, "best")
				streamURL, err = core.SelectQuality(qualities, selectBest)
				if err != nil {
					fmt.Printf("Quality selection failed: %v\n", err)
					return err
				}
				if debugMode {
					fmt.Printf("Selected quality URL: %s\n", streamURL)
				}
			} else if directURL != "" {
				streamURL = directURL
			}
		}

		switch currentAction {
		case "play":
			if debugMode {
				fmt.Printf("Stream URL: %s\n", streamURL)
			}
			err = core.Play(streamURL, name, referer, USER_AGENT, subtitles, debugMode)
			if err != nil {
				fmt.Println("Error playing:", err)
				return err
			}
		case "download":
			dlPath := cfg.DlPath
			homeDir, _ := os.UserHomeDir()
			if dlPath == "" {
				dlPath = homeDir
			}
			if strings.EqualFold(providerName, "youtube") {
				err = core.DownloadYTDLP(homeDir, dlPath, name, streamURL, referer, USER_AGENT, debugMode)
			} else {
				err = core.Download(homeDir, dlPath, name, streamURL, referer, USER_AGENT, subtitles, debugMode)
			}
			if err != nil {
				fmt.Println("Error downloading:", err)
				return err
			}
		default:
			fmt.Println("Unknown action:", currentAction)
		}

		// Save to history on success.
		if histDB != nil {
			entry := core.HistoryEntry{
				Title:     ctx.Title,
				Season:    season,
				Episode:   episode,
				EpName:    epName,
				URL:       ctx.URL,
				Provider:  providerName,
				WatchedAt: time.Now(),
			}
			if herr := histDB.AddEntry(entry); herr != nil && debugMode {
				fmt.Printf("Warning: could not save history: %v\n", herr)
			}
		}

		return nil
	}
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

		core.PreviewWithBackend(fullPath, backendFlag)
	},
}
