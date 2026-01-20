#!/bin/sh

go build -ldflags="-s -w" -o luffy
upx --best --lzma luffy
