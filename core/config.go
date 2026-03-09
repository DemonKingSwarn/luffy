package core

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// HooksConfig holds shell commands to run at specific playback lifecycle points.
// Each hook is a shell command string (executed via sh -c on Unix, cmd /c on Windows).
// The following environment variables are set for every hook:
//
//	LUFFY_TITLE      – media title
//	LUFFY_URL        – provider media URL
//	LUFFY_SEASON     – season number (0 for movies)
//	LUFFY_EPISODE    – episode number (0 for movies)
//	LUFFY_EP_NAME    – episode name (empty for movies)
//	LUFFY_PROVIDER   – provider name
//	LUFFY_ACTION     – "play" or "download"
//	LUFFY_STREAM_URL – resolved stream URL (set for on_play / on_download)
//	LUFFY_POSITION   – playback position in seconds (set for on_exit only)
type HooksConfig struct {
	// OnPlay is run just before the player is launched.
	OnPlay string `yaml:"on_play"`
	// OnExit is run after the player exits (mpv/vlc closed).
	OnExit string `yaml:"on_exit"`
	// OnDownload is run just before yt-dlp / ffmpeg is launched.
	OnDownload string `yaml:"on_download"`
}

type Config struct {
	FzfPath      string `yaml:"fzf_path"`
	Player       string `yaml:"player"`
	ImageBackend string `yaml:"image_backend"`
	Provider     string `yaml:"provider"`
	DlPath       string `yaml:"dl_path"`
	Quality      string `yaml:"quality"`
	// MpvArgs holds extra command-line arguments appended to every mpv invocation.
	// Example: ["--hwdec=auto", "--volume=80"]
	MpvArgs []string    `yaml:"mpv_args"`
	Hooks   HooksConfig `yaml:"hooks"`
}

func LoadConfig() *Config {
	config := &Config{
		FzfPath:      "fzf",    // Default
		Player:       "mpv",    // Default player
		ImageBackend: "sixel",  // Default image backend
		Provider:     "flixhq", // Default provider
		DlPath:       "",       // Default: use home directory
		Quality:      "",       // Default: prompt user to select quality
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return config
	}

	configPath := filepath.Join(home, ".config", "luffy", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		// Config file doesn't exist or can't be read, use defaults
		return config
	}

	// Parse YAML into config struct
	err = yaml.Unmarshal(data, config)
	if err != nil {
		return &Config{
			FzfPath:      "fzf",
			Player:       "mpv",
			ImageBackend: "sixel",
			Provider:     "flixhq",
			DlPath:       "",
			Quality:      "",
		}
	}

	return config
}
