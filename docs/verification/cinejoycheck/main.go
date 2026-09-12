// Command cinejoycheck exercises the Cinejoy provider end-to-end without the
// interactive fzf UI: TMDB search, ID resolution, season/episode lists, and
// GetLink server racing, with an optional bounded mpv decode proof.
//
// Usage (from the app checkout):
//
//	go run ./docs/verification/cinejoycheck -q "the goonies"
//	go run ./docs/verification/cinejoycheck -q "breaking bad" -episode 1 -play
//
// -play requires mpv and decodes three seconds with null audio/video outputs.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/demonkingswarn/luffy/core"
	"github.com/demonkingswarn/luffy/core/providers"
)

const userAgent = "Mozilla/5.0"

func main() {
	query := flag.String("q", "", "search query")
	tmdb := flag.String("tmdb", "", "resolve a movie by TMDB id only (skips search)")
	episode := flag.Int("episode", 1, "episode number to resolve for series")
	play := flag.Bool("play", false, "run 3s headless mpv decode proof")
	playCLI := flag.Bool("play-cli", false, "run mpv with the same invocation the CLI uses")
	flag.Parse()
	if *query == "" && *tmdb == "" {
		fmt.Fprintln(os.Stderr, "cinéjoycheck: -q or -tmdb is required")
		os.Exit(2)
	}

	client := core.NewClient()
	p := providers.NewCinejoy(client)

	requestID := ""
	if *tmdb != "" {
		requestID = "movie|" + *tmdb + "||"
		fmt.Printf("resolving movie by tmdb id only: %s\n", *tmdb)
	} else {
		results, err := p.Search(*query)
		if err != nil {
			fatal("search", err)
		}
		var picked *core.SearchResult
		for i := range results {
			if strings.EqualFold(results[i].Title, *query) && results[i].Type == core.Movie {
				picked = &results[i]
				break
			}
		}
		if picked == nil {
			picked = &results[0]
		}
		fmt.Printf("picked: %s (%s) type=%s\n", picked.Title, picked.Year, picked.Type)

		mediaID, err := p.GetMediaID(picked.URL)
		if err != nil {
			fatal("GetMediaID", err)
		}

		if picked.Type == core.Series {
			seasons, err := p.GetSeasons(mediaID)
			if err != nil {
				fatal("GetSeasons", err)
			}
			if len(seasons) == 0 {
				fatal("GetSeasons", fmt.Errorf("no seasons"))
			}
			fmt.Printf("seasons: %d (first: %s)\n", len(seasons), seasons[0].Name)
			episodes, err := p.GetEpisodes(seasons[0].ID, true)
			if err != nil {
				fatal("GetEpisodes", err)
			}
			if *episode < 1 || *episode > len(episodes) {
				fatal("episode", fmt.Errorf("%d out of range (max %d)", *episode, len(episodes)))
			}
			fmt.Printf("episodes: %d (first: %s)\n", len(episodes), episodes[0].Name)
			requestID = episodes[*episode-1].ID
		} else {
			servers, err := p.GetServers(mediaID)
			if err != nil || len(servers) == 0 {
				fatal("GetServers", fmt.Errorf("no servers: %w", err))
			}
			requestID = servers[0].ID
		}
	}

	start := time.Now()
	streamURL, err := p.GetLink(requestID)
	if err != nil {
		fatal("GetLink", err)
	}
	fmt.Printf("resolved in %s: %s\n", time.Since(start).Round(time.Millisecond), redact(streamURL))

	if *playCLI {
		if err := cliMpvProof(streamURL); err != nil {
			fatal("cli playback", err)
		}
		fmt.Println("OK (CLI-style mpv exited normally)")
		os.Exit(0)
	}
	if !*play {
		fmt.Println("OK (resolution only; pass -play for the decode proof)")
		os.Exit(0)
	}

	if err := decodeProof(streamURL); err != nil {
		fatal("playback", err)
	}
	fmt.Println("OK (mpv decoded 3s of video and audio)")
	os.Exit(0)
}

func cliMpvProof(streamURL string) error {
	// Mirror the fixed CLI behavior for cinejoy: the master playlist (with its
	// separate audio rendition group) goes straight to the player. No variant
	// extraction, which would strip the audio group and play silent video.
	fmt.Printf("quality stage: skipped for cinejoy (master passthrough)\n")

	// Mirror buildPlayerCmd's default (non-darwin, non-vlc) mpv invocation.
	args := []string{
		streamURL,
		"--referrer=" + providers.CinejoyBaseURL + "/",
		"--user-agent=" + userAgent,
		"--force-media-title=Playing CLI replication",
	}
	args = append(args, "--input-ipc-server=/tmp/cinejoycheck-mpv.sock")
	os.Remove("/tmp/cinejoycheck-mpv.sock")
	args = append(args, "--http-header-fields=Origin: "+providers.CinejoyBaseURL)

	cmd := exec.Command("mpv", args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	timer := time.AfterFunc(60*time.Second, func() {
		cmd.Process.Kill() //nolint:errcheck
	})
	err := cmd.Wait()
	timer.Stop()
	os.Remove("/tmp/cinejoycheck-mpv.sock")
	return err
}

func decodeProof(streamURL string) error {
	cmd := exec.Command("mpv", "--no-config", "--vo=null", "--ao=null", "--length=3",
		"--network-timeout=10", "--ytdl=no",
		"--user-agent="+userAgent,
		"--referrer=https://cinejoy.to/",
		"--http-header-fields=Origin: https://cinejoy.to",
		"--term-status-msg=PROBE time=${time-pos}",
		streamURL)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	// Hard bailout in case mpv hangs.
	timer := time.AfterFunc(45*time.Second, func() {
		cmd.Process.Kill() //nolint:errcheck
	})
	err := cmd.Wait()
	timer.Stop()
	return err
}

func redact(raw string) string {
	if len(raw) < 90 {
		return raw
	}
	return raw[:45] + "..." + raw[len(raw)-20:]
}

func fatal(stage string, err error) {
	fmt.Fprintf(os.Stderr, "%s failed: %v\n", stage, err)
	os.Exit(1)
}
