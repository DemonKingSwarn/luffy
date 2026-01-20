# Luffy

![Language](https://img.shields.io/badge/language-Go-blue.svg)
![OS](https://img.shields.io/badge/OS-Linux%20%7C%20freeBSD%20%7C%20macOS%20%7C%20Windows%20%7C%20Android-lightgrey)

**Luffy** is a high-efficient, powerful, and fast movie scraper and streamer for the terminal. It allows you to search for, stream, and download movies and TV shows directly from your command line.

## Overview

Luffy scrapes content from high-quality sources and leverages external tools for playback and downloading, providing a seamless terminal-based entertainment experience.

## Installation

### 1. Go Install (Recommended)

If you have Go installed, you can easily install Luffy:

```bash
go install github.com/demonkingswarn/luffy@v1.0.3
```

### 2. Build from Source

1.  Clone the repository:
    ```bash
    git clone https://github.com/demonkingswarn/luffy.git
    cd luffy
    ```

2.  Build and install:
    ```bash
    go install .
    ```
    *Ensure your `$GOPATH/bin` is in your system's `PATH`.*

## Dependencies

To use Luffy to its full potential, ensure you have the following installed:

*   **[mpv](https://mpv.io/)**: Video Player (Linux/FreeBSD/Windows)
*   **[iina](https://iina.io/)**: Video Player (macOS)
*   **[vlc](https://www.videolan.org/vlc/download-android.html)**: Video Player (Android)
*   **[yt-dlp](https://github.com/yt-dlp/yt-dlp)**: Required for downloading content.

## Usage

```bash
luffy [query] [flags]
```

`[query]` is the title you want to search for (e.g., "breaking bad", "dune", "one piece").

### Options

| Flag | Alias | Description |
|------|-------|-------------|
| `--action` | `-a` | Action to perform: `play` (default) or `download`. |
| `--season` | `-s` | (Series only) Specify the season number. |
| `--episodes` | `-e` | (Series only) Specify a single episode (`5`) or a range (`1-5`). |
| `--help` | `-h` | Show help message and exit. |

### 🎬 Examples

**Search & Play a Movie**
Search for a title and select interactively:
```bash
luffy "dune"
```

**Download a Movie**
```bash
luffy "dune" --action download
```

**Play a TV Episode**
Directly play Season 1, Episode 1:
```bash
luffy "breaking bad" -s 1 -e 1
```

**Download a Range of Episodes**
Download episodes 1 through 5 of Season 2:
```bash
luffy "stranger things" -s 2 -e 1-5 -a download
```


# Support
You can contact the developer directly via this <a href="mailto:swarn@demonkingswarn.live">email</a>. However, the most recommended way is to head to the discord server.

<a href="https://discord.gg/JF85vTkDyC"><img src="https://invidget.switchblade.xyz/JF85vTkDyC"></a>

If you run into issues or want to request a new feature, you are encouraged to make a GitHub issue, won't bite you, trust me.


## Provider

| Website | Available Qualities | Content |
|---------|---------------------|---------|
| FlixHQ  | 720p, 1080p         | Movies, TV Series |

## Disclaimer

This tool is for educational purposes only. The developers of this tool do not host any content and are not affiliated with the streaming services scraped. Please respect copyright laws in your jurisdiction.

## Contributing

Pull requests are welcome and appreciated. For major changes, please open an issue first to discuss what you would like to change.

