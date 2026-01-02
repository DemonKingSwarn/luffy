package core

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/ktr0731/go-fuzzyfinder"
)

func Prompt(label string) string {
	fmt.Print(label + ": ")
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}

func Select(label string, items []string) int {
	idx, err := fuzzyfinder.Find(
		items,
		func(i int) string {
			return items[i]
		},
	)
	if err != nil {
		fmt.Println("Selection cancelled or failed:", err)
		os.Exit(1)
	}
	return idx
}
