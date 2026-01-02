# Luffy

**Luffy** is a powerful command-line interface (CLI) tool that allows you to search for, stream, and download movies and TV shows directly from your terminal. It scrapes content from FlixHQ and leverages external tools for high-quality playback and downloading.

## Features

- **Search & Discover**: Quickly search for movies and TV series.
- **Interactive Selection**: Uses fuzzy finding to select titles, seasons, and episodes.
- **Stream**: Watch content instantly using `mpv` (Linux/Windows) or `iina` (macOS).
- **Download**: Download episodes or movies for offline viewing using `yt-dlp`.
- **Batch Operations**: Support for selecting specific seasons and episode ranges (e.g., `1-5`).
- **Cross-Platform**: Works on Linux, macOS, and Windows (assuming dependencies are met).

## Prerequisites

Before using Luffy, ensure you have the following installed on your system:

1.  **Go** (v1.21+ recommended) - To build the application.
2.  **Media Player**:
    -   **Linux/Windows**: [mpv](https://mpv.io/)
    -   **macOS**: [iina](https://iina.io/) (or `mpv`)
3.  **Downloader**:
    -   [yt-dlp](https://github.com/yt-dlp/yt-dlp) - Required for downloading content.

## Installation

1.  Clone the repository:
    ```bash
    git clone https://github.com/yourusername/luffy.git
    cd luffy
    ```

2.  Build and install the binary:
    ```bash
    go install
    ```
    *Ensure your `$GOPATH/bin` is in your system's `PATH`.*

## Usage

The basic syntax is:
```bash
luffy [query] [flags]
```

### Examples

**1. Interactive Search & Play**
Search for a show, select the result, choose a season/episode, and pick an action (Play/Download) interactively:
```bash
luffy "one piece"
```

**2. Play Specific Episode**
Skip the interactive selection for season and episode by providing flags:
```bash
luffy "breaking bad" -s 1 -e 1 -a play
```
*This plays Season 1, Episode 1 of the first search result for "breaking bad".*

**3. Download a Range of Episodes**
Download episodes 1 through 5 of Season 2:
```bash
luffy "stranger things" -s 2 -e 1-5 -a download
```

### Flags

| Flag | Short | Description | Example |
|------|-------|-------------|---------|
| `--season` | `-s` | Specify the season number | `-s 1` |
| `--episodes` | `-e` | Specify episode number or range | `-e 3` or `-e 1-5` |
| `--action` | `-a` | Action to perform: `play` or `download` | `-a download` |

## Disclaimer

This tool is for educational purposes only. The developers of this tool do not host any content and are not affiliated with the streaming services scraped. Please respect copyright laws in your jurisdiction.
