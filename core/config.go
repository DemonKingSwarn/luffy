package core

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	FzfPath string
}

func LoadConfig() *Config {
	config := &Config{
		FzfPath: "fzf", // Default
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return config
	}

	configPath := filepath.Join(home, ".config", "luffy", "conf")
	file, err := os.Open(configPath)
	if err != nil {
		// Config file doesn't exist or can't be opened, use defaults
		return config
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.Trim(strings.TrimSpace(parts[1]), "\"'")

			if key == "fzf_path" {
				config.FzfPath = value
			}
		}
	}

	return config
}
