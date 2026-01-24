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
<img src="https://img.shields.io/badge/os-freebsd-brightgreen" alt="OS FreeBSD">
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
    - [Arch Linux](#1-arch-linux)
    - [Debian Linux](#2-debian-linux)
    - [MacOS](#3-macos)
    - [Windows](#4-windows)
    - [Go Install](#5-go-install)
    - [Build from Source](#6-build-from-source)
    - [Android Installation](#7-android-installation)
- [Dependencies](#dependencies)
- [Usage](#usage)
- [Support](#support)
- [Providers](#providers)

## Installation

### 1. Arch Linux

```sh
paru -S luffy-bin
```

### 2. Debian Linux

```sh
curl -sL "https://github.com/DemonKingSwarn/luffy/raw/refs/heads/master/luffy_debian_installer.sh" | bash
```

### 3. MacOS

```sh
brew tap gamedevCloudy/tools
brew install --cask iina
brew install luffy
```

### 4. Windows

Make sure you have [scoop.sh](https://scoop.sh) installed on your system.

```sh
scoop bucket add demon-apps https://github.com/DemonKingSwarn/flix-cli-bucket.git
scoop bucket add extras
scoop install luffy
```

> [!IMPORTANT]
> On windows if you want to use the `--show-image`, you need to use the `wezterm` terminal emulator. It is installed as a dependency on windows.

### 5. Go Install

If you have Go installed, you can easily install Luffy:

```bash
go install github.com/demonkingswarn/luffy@v1.0.10
```

### 6. Build from Source

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

### 7. Android Installation

Install termux [(Guide)](https://termux.com/)

```sh
pkg up -y
pkg in golang fzf python-yt-dlp
go install github.com/demonkingswarn/luffy@v1.0.10
```


# Dependencies

- [`mpv`](https://mpv.io) - Video Player for Linux and Windows
- [`vlc`](https://www.videolan.org/vlc/) - Alternate video player for Linux and Windows
- [`iina`](https://iina.io) - Alternate video player for MacOS
- [`vlc-android`](https://play.google.com/store/apps/details?id=org.videolan.vlc) - Video Player for Android
- [`yt-dlp`](https://github.com/yt-dlp/yt-dlp) - Download manager
- [`fzf`](https://github.com/junegunn/fzf) - For selection menu
- [`chafa`](https://github.com/hpjansson/chafa) & [`libsixel`](https://github.com/saitoha/libsixel) - For showing posters.

> [!IMPORTANT]
> To be able to see the images, you need terminal emulators which support the sixel image protocol.
>
> For ex: kitty, ghostty, wezterm, foot
>
> Also note that, if you are using `kitty` or `ghostty` then you need to add the following in the config file:
> ```yaml
> image_backend: kitty
> ```
> config file can be found at  `$HOME/.config/luffy/config.yaml`

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
| `--providers` | `-p` | Select provider. |


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


# Providers

Luffy uses four main providers, which you can easily change between by specifying them in the config file: `$HOME/.config/luffy/config.yaml`

- flixhq:
    ```yaml
    provider: flixhq
    ```
- brocoflix:
    ```yaml
    provider: brocoflix
    ```
- xprime:
    ```yaml
    provider: xprime
    ```
- movies4u (bollywood only):
    ```yaml
    provider: movies4u
    ```
    **NOTE**: `brocoflix` & `xprime` doesn't return any subtitle urls, but their quality is much better.

There are three experimental providers, using these may not always work. These are:

- sflix:
    ```yaml
    provider: sflix
    ```

- braflix:
    ```yaml
    provider: braflix
    ```

- hdrezka:
    ```yaml
    provider: hdrezka
    ```

Also note that `flixhq` is the default provider.
