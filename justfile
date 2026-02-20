# Project settings
binary_name := "luffy"
flags       := "-s -w"
build_dir   := "builds"

build: windows-amd64 windows-386 windows-arm linux-amd64 linux-386 linux-arm linux-risc android mac-arm mac-intel freebsd-amd64 freebsd-386

_build os arch ext="":
    @echo "Building {{os}}/{{arch}}..."
    mkdir -p {{build_dir}}
    GOOS={{os}} GOARCH={{arch}} CGO_ENABLED=0 go build -ldflags={{quote(flags)}} -o {{build_dir}}/{{binary_name}}-{{os}}-{{arch}}{{ext}}

_compress path:
    @upx --best --lzma {{path}} || echo "UPX skip: {{path}}"

# Platform Recipes
windows-amd64: (_build "windows" "amd64" ".exe")
    @just _compress {{build_dir}}/{{binary_name}}-windows-amd64.exe

windows-386:   (_build "windows" "386" ".exe")
    @just _compress {{build_dir}}/{{binary_name}}-windows-386.exe

windows-arm:   (_build "windows" "arm64" ".exe")

linux-amd64:   (_build "linux" "amd64")
    @just _compress {{build_dir}}/{{binary_name}}-linux-amd64

linux-386:     (_build "linux" "386")
    @just _compress {{build_dir}}/{{binary_name}}-linux-386

linux-arm:     (_build "linux" "arm64")
    @just _compress {{build_dir}}/{{binary_name}}-linux-arm64

android:     (_build "android" "arm64")
    @just _compress {{build_dir}}/{{binary_name}}-android-arm64

linux-risc:    (_build "linux" "riscv64")
    @just _compress {{build_dir}}/{{binary_name}}-linux-riscv64

mac-arm:       (_build "darwin" "arm64")
mac-intel:     (_build "darwin" "amd64")
freebsd-amd64: (_build "freebsd" "amd64")
freebsd-386:   (_build "freebsd" "386")

clean:
    rm -rf {{build_dir}}

test:
    go test ./... -v
