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
<img src="https://img.shields.io/badge/os-mac-brightgreen"alt="OS Mac">
<img src="https://img.shields.io/badge/os-windows-brightgreen" alt="OS Windows">
<img src="https://img.shields.io/badge/os-android-brightgreen" alt="OS Android">

<br>


<a href="https://discord.gg/JF85vTkDyC"><img src="https://invidget.switchblade.xyz/JF85vTkDyC"></a><a href="https://matrix.to/#/#swarn-discord:chat.demonkingswarn.live"><img src="./.assets/matrix-logo.svg"></a>

</div>
<br>

---

![](./.assets/showcase.gif)

---

## Overview

- [Installation](#installation)
    - [Arch Linux](#1-arch-linux)
    - [MacOS](#2-macos)
    - [Windows](#3-windows)
    - [Go Install](#4-go-install)
    - [Build from Source](#5-build-from-source)
    - [Android Installation](#6-android-installation)
- [Dependencies](#dependencies)
- [Usage](#usage)
- [Configuration](#configuration)
- [Providers](#providers)

 > [!NOTE] 
 > Before creating an issue, make sure to update luffy

## Installation

### 1. Arch Linux

```sh
paru -S luffy-bin
```


### 2. MacOS

```sh
brew tap gamedevCloudy/tools
brew install --cask iina
brew install luffy
```

### 3. Windows

Make sure you have [scoop.sh](https://scoop.sh) installed on your system.

```sh
scoop bucket add demon-apps https://github.com/DemonKingSwarn/flix-cli-bucket.git
scoop bucket add extras
scoop install luffy
```

> [!IMPORTANT]
> On windows if you want to use the `--show-image`, you need to use the `wezterm` terminal emulator. It is installed as a dependency on windows.

### 4. Go Install

If you have Go installed, you can easily install Luffy:

```bash
go install github.com/demonkingswarn/luffy@latest
```

### 5. Build from Source

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

### 6. Android Installation

Install termux [(Guide)](https://termux.com/)

```sh
pkg up -y
pkg in fzf python-yt-dlp
curl -sL "https://github.com/DemonKingSwarn/luffy/releases/download/v1.1.0/luffy-android-arm64" -o $PREFIX/bin/luffy
chmod +x $PREFIX/bin/luffy
```


# Dependencies

- [`mpv`](https://mpv.io) - Video Player for Linux and Windows
- [`vlc`](https://www.videolan.org/vlc/) - Alternate video player for Linux and Windows
- [`iina`](https://iina.io) - Video Player for MacOS
- [`vlc-android`](https://play.google.com/store/apps/details?id=org.videolan.vlc) - Video Player for Android
- [`yt-dlp`](https://github.com/yt-dlp/yt-dlp) - Download manager
- [`fzf`](https://github.com/junegunn/fzf) - For selection menus
- [`chafa`](https://github.com/hpjansson/chafa) & [`libsixel`](https://github.com/saitoha/libsixel) - For poster previews (`--show-image`)

## Usage

```bash
luffy [query] [flags]
```

`[query]` is the title you want to search for (e.g., "breaking bad", "dune", "one piece"). The query is optional when using `--history` or `--recommend`.

### Flags

| Flag | Alias | Description |
|------|-------|-------------|
| `--action` | `-a` | Action to perform: `play` (default) or `download`. |
| `--season` | `-s` | (Series only) Specify the season number. |
| `--episodes` | `-e` | (Series only) Specify a single episode (`5`) or a range (`1-5`). |
| `--best` | `-b` | Auto-select the best available quality (skips fzf quality prompt). |
| `--provider` | `-p` | Select provider (e.g. `sflix`, `braflix`). Overrides config. |
| `--history` | `-H` | Browse watch history and resume a previous title. |
| `--recommend` | `-r` | Get personalised recommendations based on watch history. |
| `--show-image` | NA | Show poster previews in fzf (requires chafa and a supported terminal). |
| `--debug` | `-d` | Print debug information (URLs, decryption steps, etc.). |
| `--help` | `-h` | Show help message and exit. |

### Playback Controls

When watching a TV series, an fzf menu appears alongside the player with four options:

| Action | Description |
|--------|-------------|
| **Next** | Kill the player and move to the next episode. |
| **Previous** | Kill the player and move to the previous episode. |
| **Replay** | Restart the current episode from the beginning. |
| **Quit** | Kill the player and exit. |

### Examples

**Search & Play a Movie**
```bash
luffy "dune"
```

**Download a Movie**
```bash
luffy "dune" --action download
```

**Play a TV Episode**
```bash
luffy "breaking bad" -s 1 -e 1
```

**Download a Range of Episodes**
```bash
luffy "stranger things" -s 2 -e 1-5 -a download
```

**Auto-select Best Quality**
```bash
luffy "dune" --best
```

**Use a Different Provider**
```bash
luffy "breaking bad" --provider sflix
```

**Resume from Watch History**
```bash
luffy --history
```

**Get Personalised Recommendations**
```bash
luffy --recommend
```

**Recommendations with Poster Previews**
```bash
luffy --recommend --show-image
```

## Configuration

The config file lives at `~/.config/luffy/config.yaml`. All fields are optional; defaults are shown below.

```yaml
# Path to the fzf binary. Set to an absolute path if fzf is not on PATH.
fzf_path: fzf

# Video player: "mpv" (default) or "vlc". IINA is used automatically on macOS.
player: mpv

# chafa rendering backend for poster previews.
# Options: sixel (default), kitty, iterm, symbols
image_backend: sixel

# Default search provider.
provider: flixhq

# Directory where downloaded files are saved. Defaults to home directory.
dl_path: ""

# Quality selection: leave empty to show an fzf prompt, or set to "best"
# to always auto-select the highest available quality.
quality: ""
```

> [!IMPORTANT]
> To see poster images, your terminal emulator must support a graphics protocol.
> Supported terminals include kitty, ghostty, WezTerm, and foot (sixel).
>
> If you use kitty or ghostty, set `image_backend: kitty` in your config.


# Providers

You can set the default provider in the config file (`~/.config/luffy/config.yaml`) or override it per-run with `--provider`.

| Provider | `provider:` value | Notes |
|----------|-------------------|-------|
| FlixHQ | `flixhq` | Default |
| Sflix | `sflix` | |
| Braflix | `braflix` | |
| Movies4u | `movies4u` | Bollywood only |
| YouTube | `youtube` | Streams/downloads via yt-dlp |
| HDRezka | `hdrezka` | Experimental — may not always work |

Example config:
```yaml
provider: sflix
```
