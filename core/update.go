package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
)

type Release struct {
	TagName string `json:"tag_name"`
}

var RemoteVer string

func check_update() {
	url := "https://api.github.com/repos/demonkingswarn/luffy/releases/latest"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic(err)
	}

	req.Header.Set("User-Agent", "luffy/1.0.14")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}

	defer resp.Body.Close()

	var r Release

	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		panic(err)
	}

	RemoteVer = r.TagName
}

func Update() {
	operating_system := runtime.GOOS

	check_update()
	if Version == RemoteVer {
		fmt.Println("Already Up to Date!")
		os.Exit(0)
	} else {
		fmt.Println("Updating: %s -> %s", Version, RemoteVer)
		update_binary(operating_system)
	}
}

func update_binary(distro string) {
	var bin_url string
	switch(distro) {
		case "linux":
			bin_url = fmt.Sprintf("https://github.com/DemonKingSwarn/luffy/releases/download/%s/luffy-linux-%s", RemoteVer, runtime.GOARCH)
			break
		case "windows":
			bin_url = fmt.Sprintf("https://github.com/DemonKingSwarn/luffy/releases/download/%s/luffy-windows-%s.exe", RemoteVer, runtime.GOARCH)
			break
		case "darwin":
			bin_url = fmt.Sprintf("https://github.com/DemonKingSwarn/luffy/releases/download/%s/luffy-macos-%s", RemoteVer, runtime.GOARCH)
			break
		case "freebsd":
			bin_url = fmt.Sprintf("https://github.com/DemonKingSwarn/luffy/releases/download/%s/luffy-freebsd-%s", RemoteVer, runtime.GOARCH)
			break
		case "android":
			bin_url = fmt.Sprintf("https://github.com/DemonKingSwarn/luffy/releases/download/%s/luffy-android-%s", RemoteVer, runtime.GOARCH)
			break
		default:
			fmt.Println("Unsupported OS!")
			break
	}
	
	var bin string
	if runtime.GOOS == "windows" {
		bin, _ = exec.LookPath("luffy.exe")
	} else {
		bin, _ = exec.LookPath("luffy")
	}

	resp, err := http.Get(bin_url)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	file, _ := os.Create(bin)

	defer file.Close()

	_, err = io.Copy(file, resp.Body)

	if err != nil {
		panic(err)
	}

}
