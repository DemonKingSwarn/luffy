build: windows-amd64 windows-386 windows-arm linux-amd64 linux-386 linux-arm linux-risc mac-arm mac-intel freebsd-amd64 freebsd-386

windows-amd64:
  GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o builds/luffy-windows-amd64.exe
  upx --best --lzma builds/luffy-windows-amd64.exe

windows-386:
  GOOS=windows GOARCH=386 CGO_ENABLED=0 go build -ldflags="-s -w" -o builds/luffy-windows-386.exe
  upx --best --lzma builds/luffy-windows-386.exe

windows-arm:
  GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o builds/luffy-windows-arm64.exe
  
linux-amd64:
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o builds/luffy-linux-amd64
  upx --best --lzma builds/luffy-linux-amd64

linux-386:
  GOOS=linux GOARCH=386 CGO_ENABLED=0 go build -ldflags="-s -w" -o builds/luffy-linux-386
  upx --best --lzma builds/luffy-linux-386

linux-arm:
  GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o builds/luffy-linux-arm64
  upx --best --lzma builds/luffy-linux-arm64

linux-risc:
  GOOS=linux GOARCH=riscv64 CGO_ENABLED=0 go build -ldflags="-s -w" -o builds/luffy-linux-riscv64
  upx --best --lzma builds/luffy-linux-riscv64

mac-arm:
  GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o builds/luffy-mac-arm64

mac-intel:
  GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o builds/luffy-mac-amd64

freebsd-amd64:
  GOOS=freebsd GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o builds/luffy-freebsd-amd64

freebsd-386:
  GOOS=freebsd GOARCH=386 CGO_ENABLED=0 go build -ldflags="-s -w" -o builds/luffy-freebsd-386

