@echo off

go build -o luffy-amd64.exe
upx --best --lzma luffy-amd64.exe
