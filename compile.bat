@echo off

go build -o luffy.exe
upx --best --lzma luffy.exe
