package core

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	FzfPath      string `yaml:"fzf_path"`
	Player       string `yaml:"player"`
	ImageBackend string `yaml:"image_backend"`
	Provider     string `yaml:"provider"`
	DlPath       string `yaml:"dl_path"`
	Quality      string `yaml:"quality"`
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
