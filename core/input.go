package core

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/demonkingswarn/fzf.go"
)

func Prompt(label string) string {
	fmt.Print(label + ": ")
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}

func Select(label string, items []string) int {
	components := make([]interface{}, len(items))
	for i := range items {
		components[i] = i
	}

	cfg := LoadConfig()
	prompt := label + "> "
	height := "40"
	layout := fzf.LayoutReverse
	res, _, err := fzf.FzfPrompt(
		components,
		func(i interface{}) string {
			return items[i.(int)]
		},
		cfg.FzfPath,
		&fzf.Options{
			PromptString: &prompt,
			Layout:       &layout,
			Height:       &height,
		},
	)

	if err != nil {
		fmt.Println("Selection cancelled or failed:", err)
		os.Exit(1)
	}

	if res == nil {
		fmt.Println("No selection made")
		os.Exit(1)
	}

	fmt.Print("\033[H\033[2J") // Clear screen
	return res.(int)
}

func SelectWithPreview(label string, items []string, previewCmd string) int {
	components := make([]interface{}, len(items))
	for i := range items {
		components[i] = i
	}

	cfg := LoadConfig()
	prompt := label + "> "
	layout := fzf.LayoutReverse

	opts := &fzf.Options{
		PromptString: &prompt,
		Layout:       &layout,
	}

	if previewCmd != "" {
		opts.Preview = &previewCmd
	}

	res, _, err := fzf.FzfPrompt(
		components,
		func(i interface{}) string {
			return items[i.(int)]
		},
		cfg.FzfPath,
		opts,
	)

	if err != nil {
		fmt.Println("Selection cancelled or failed:", err)
		os.Exit(1)
	}

	if res == nil {
		fmt.Println("No selection made")
		os.Exit(1)
	}

	fmt.Print("\033[H\033[2J") // Clear screen
	return res.(int)
}
