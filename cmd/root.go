package cmd

import (
	"fmt"
	"os"
	"strings"

	"luffy/core"
	"github.com/spf13/cobra"
)

var (
	seasonFlag   int
	episodeFlag  string
	actionFlag   string
)

func init() {
	rootCmd.Flags().IntVarP(&seasonFlag, "season", "s", 0, "Specify season number")
	rootCmd.Flags().StringVarP(&episodeFlag, "episodes", "e", "", "Specify episode or range (e.g. 1, 1-5)")
	rootCmd.Flags().StringVarP(&actionFlag, "action", "a", "", "Action to perform (play, download)")
}

var rootCmd = &cobra.Command{
	Use:   "luffy [query]",
	Short: "Watch movies and TV shows from the commandline",
	Args:  cobra.ArbitraryArgs,

	RunE: func(cmd *cobra.Command, args []string) error {
		client := core.NewClient()
		ctx := &core.Context{
			Client: client,
		}

		if len(args) == 0 {
			ctx.Query = core.Prompt("Search")
		} else {
			ctx.Query = strings.Join(args, " ")
		}

		results, err := core.SearchContent(ctx.Query, ctx.Client)
		if err != nil {
			return err
		}

		var titles []string
		for _, r := range results {
			titles = append(titles, fmt.Sprintf("[%s] %s", r.Type, r.Title))
		}

		idx := core.Select("Results:", titles)
		selected := results[idx]

		ctx.Title = selected.Title
		ctx.URL = selected.URL
		ctx.ContentType = selected.Type

		fmt.Println("Selected:", ctx.Title)

		mediaID, err := core.GetMediaID(ctx.URL, ctx.Client)
		if err != nil {
			return err
		}

		var episodesToProcess []core.Episode

		if ctx.ContentType == core.Series {
			seasons, err := core.GetSeasons(mediaID, ctx.Client)
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

			allEpisodes, err := core.GetEpisodes(selectedSeason.ID, true, ctx.Client)
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
			allEpisodes, err := core.GetEpisodes(mediaID, false, ctx.Client)
			if err != nil || len(allEpisodes) == 0 {
				return fmt.Errorf("could not find movie info")
			}
			episodesToProcess = append(episodesToProcess, allEpisodes[0])
		}

		// Determine action
		currentAction := actionFlag
		if currentAction == "" {
			actions := []string{"Play", "Download"}
			actIdx := core.Select("Action:", actions)
			currentAction = actions[actIdx]
		}
		currentAction = strings.ToLower(currentAction)

		for _, ep := range episodesToProcess {
			fmt.Printf("\nProcessing: %s\n", ep.Name)
			
			servers, err := core.GetServers(ep.ID, ctx.Client)
			if err != nil {
				fmt.Println("Error fetching servers:", err)
				continue
			}
			if len(servers) == 0 {
				fmt.Println("No servers found")
				continue
			}

			selectedServer := servers[0]
			for _, s := range servers {
				if strings.Contains(strings.ToLower(s.Name), "vidcloud") {
					selectedServer = s
					break
				}
			}

			link, err := core.GetLink(selectedServer.ID, ctx.Client)
			if err != nil {
				fmt.Println("Error getting link:", err)
				continue
			}

			fmt.Println("Decrypting stream...")
			streamURL, subtitles, err := core.DecryptStream(link, ctx.Client)
			if err != nil {
				fmt.Printf("Decryption failed for %s: %v\n", ep.Name, err)
				continue
			}

			switch currentAction {
			case "play":
				err = core.Play(streamURL, ctx.Title + " - " + ep.Name, core.FLIXHQ_BASE_URL, subtitles)
				if err != nil {
					fmt.Println("Error playing:", err)
				}
			case "download":
				homeDir, _ := os.UserHomeDir()
				err = core.Download(homeDir, ctx.Title + " - " + ep.Name, streamURL, core.FLIXHQ_BASE_URL, subtitles)
				if err != nil {
					fmt.Println("Error downloading:", err)
				}
			default:
				fmt.Println("Unknown action:", currentAction)
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

