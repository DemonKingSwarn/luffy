<h1 align='center'>
    LUFFY
</h1>

<br>

<h3 align='center'>
    Spiritual successor of flix-cli and mov-cli.
</h3>


<div align='center'>
<br>


![Language](https://img.shields.io/badge/-go-00ADD8.svg?style=for-the-badge&logo=go&logoColor=white)

<a href="http://makeapullrequest.com"><img src="https://img.shields.io/badge/PRs-welcome-brightgreen.svg" alt="PRs Welcome"></a>

<img src="https://img.shields.io/badge/os-linux-brightgreen" alt="OS linux">
<img src="https://img.shields.io/badge/os-freebsd-brightscreen" alt="OS FreeBSD">
<img src="https://img.shields.io/badge/os-mac-brightgreen"alt="OS Mac">
<img src="https://img.shields.io/badge/os-windows-brightgreen" alt="OS Windows">
<img src="https://img.shields.io/badge/os-android-brightgreen" alt="OS Android">

<br>
</div>

<br>

---

![](./.assets/showcase.gif)

---

## Overview

- [Installation](#installation)
- [Dependencies](#dependencies)
- [Usage](#usage)
- [Support](#support)

## Installation

### 1. Go Install (Recommended)

If you have Go installed, you can easily install Luffy:

```bash
go install github.com/demonkingswarn/luffy@v1.0.5
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

# Dependencies

- [`mpv`](https://mpv.io) - Video Player
- [`iina`](https://iina.io) - Alternate video player for MacOS
- [`vlc`](https://play.google.com/store/apps/details?id=org.videolan.vlc) - Video Player for Android
- [`yt-dlp`](https://github.com/yt-dlp/yt-dlp) - Download manager
- [`fzf`](https://github.com/junegunn/fzf) - for selection menu

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
| `--show-image` | NA | Show posters preview. |


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

