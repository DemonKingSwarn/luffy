#!/usr/bin/env bash

platform=$(uname -s)

if [[ "$platform" == "Linux" ]]; then
  go build -ldflags="-s -w" -o luffy.amd64
  upx --best --lzma luffy.amd64
else
  go build -o luffy-macos.aarch64
fi
