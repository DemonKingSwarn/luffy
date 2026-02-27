# User Input and fzf Integration

`core/input.go` wraps the `fzf.go` library and standard input to provide three selection primitives and one free-text prompt. Every interactive step in the CLI (search result selection, season/episode selection, quality selection, playback controls) goes through one of these functions.

## How It Works

### 1. fzf Configuration

All fzf calls share common options loaded from `LoadConfig()`:

- **`FzfPath`** — path to the fzf binary (default `"fzf"`). Override in config if fzf is not on `PATH`.
- **`Layout: LayoutReverse`** — fzf opens with the prompt at the bottom, results growing upward.
- **`Height: "40"`** — limits fzf to 40 rows so it does not consume the whole screen (applies to `Select` only).

The fzf prompt label is always `label + "> "`.

### 2. Index-Based Selection

`Select` and `SelectAction` pass integer indices as the fzf items and use a display function to render each index as its corresponding string. This avoids any ambiguity when two items have identical labels — the index is always the stable identity.

### 3. Select (returning index)

Used for search results, seasons, episodes, servers, and quality variants. Returns the **0-based index** of the chosen item. If the user cancels (presses Escape) or an error occurs, `os.Exit(1)` is called — there is no error return path, because cancellation at these steps is treated as a hard stop.

After selection the screen is cleared with `\033[H\033[2J`.

### 4. SelectAction (returning label string)

Used exclusively for the playback control menu in `PlayWithControls`. Returns the **label string** of the chosen action (e.g. `"Next"`). Returns `""` on cancellation rather than calling `os.Exit`, because the caller (`PlayWithControls`) treats an empty return as `Quit` and exits gracefully.

The screen is cleared after a successful selection.

### 5. SelectWithPreview (index with fzf preview window)

Used for the `--recommend` path when `--show-image` is active. Accepts an optional `previewCmd` string that is passed directly to fzf's `--preview` option. The preview command is responsible for rendering content (typically a `chafa` invocation via the shell) in the fzf preview pane.

Unlike `Select`, `SelectWithPreview` does not set a fixed `Height`, allowing fzf to use more of the terminal for the preview pane.

Returns the 0-based index of the chosen item. Exits on cancellation.

### 6. Prompt (free-text input)

`Prompt` prints `label + ": "` and reads one line from stdin using a buffered reader. Used by `cmd/root.go` when the `--search` flag is not provided on the command line, prompting the user to type a title.

## Public API

```go
// Prompt prints a label and reads one line of text from stdin.
// Returns the trimmed input string.
func Prompt(label string) string

// Select shows an fzf menu with the given items and returns the
// 0-based index of the chosen item. Calls os.Exit(1) on cancellation.
func Select(label string, items []string) int

// SelectAction shows an fzf menu with the given action labels and
// returns the label string of the chosen action.
// Returns "" if the selection is cancelled (no os.Exit).
func SelectAction(label string, actions []string) string

// SelectWithPreview shows an fzf menu with an optional preview pane.
// previewCmd is a shell command passed to fzf's --preview option;
// pass "" to disable the preview. Returns the 0-based index of the
// chosen item. Calls os.Exit(1) on cancellation.
func SelectWithPreview(label string, items []string, previewCmd string) int
```

## Behaviour Comparison

| Function | Returns | On cancel | Preview pane | Height limit |
|----------|---------|-----------|--------------|--------------|
| `Prompt` | string | — | No | N/A |
| `Select` | int (index) | `os.Exit(1)` | No | 40 rows |
| `SelectAction` | string (label) | `""` | No | 40 rows |
| `SelectWithPreview` | int (index) | `os.Exit(1)` | Optional | None (fzf default) |
