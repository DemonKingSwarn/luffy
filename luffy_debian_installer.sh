#!/bin/sh

LUFFY_VERSION="1.0.13"
LUFFY_URL="https://github.com/demonkingswarn/luffy"
LUFFY_BINARY="${LUFFY_URL}/releases/download/v${LUFFY_VERSION}/luffy-linux-amd64"
DEPENDS="fzf chafa mpv libsixel-bin yt-dlp"

sudo apt install -y $DEPENDS
sudo curl -sLo /usr/sbin/luffy "$LUFFY_BINARY"
