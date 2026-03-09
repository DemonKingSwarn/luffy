module github.com/demonkingswarn/luffy

go 1.25

require (
	github.com/PuerkitoBio/goquery v1.11.0
	github.com/demonkingswarn/fzf.go v0.0.5
	github.com/diniamo/gopv v0.0.0-20251028165920-b71b8f821a6c
	github.com/spf13/cobra v1.10.2
	golang.org/x/term v0.40.0
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.46.1
)

require (
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/andybalholm/cascadia v1.3.3 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	go.pennock.tech/swallowjson v1.0.2 // indirect
	golang.org/x/exp v0.0.0-20251023183803-a4bb9ffd2546 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	modernc.org/libc v1.70.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace modernc.org/libc => ./vendor-patches/modernc.org/libc

replace modernc.org/sqlite => ./vendor-patches/modernc.org/sqlite
