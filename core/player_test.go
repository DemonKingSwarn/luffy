//go:build linux

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlaybackPreservesSuppliedSubtitles(t *testing.T) {
	// Keep config/hooks and all deletion targets inside disposable fixtures.
	root := t.TempDir()
	t.Setenv("HOME", root)
	tmp := filepath.Join(root, "tmp")
	bin := filepath.Join(root, "bin")
	for _, dir := range []string{tmp, bin, tmp + "-other"} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("TMPDIR", tmp)
	t.Setenv("PATH", bin)
	argsFile := filepath.Join(root, "args")
	t.Setenv("LUFFY_TEST_ARGS", argsFile)

	// Avoid an interactive fzf menu even when go test runs from a terminal.
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = stdin
	t.Cleanup(func() { os.Stdin = oldStdin; stdin.Close() })

	paths := []string{
		filepath.Join(tmp, "local.srt"),
		tmp + "/../outside.srt", // Preserve traversal; filepath.Join would clean it.
		filepath.Join(tmp+"-other", "prefix.srt"),
	}
	subtitles := append([]string{"", "https://example.invalid/subtitles.vtt"}, paths...)
	for _, entry := range []string{"Play", "PlayWithControls"} {
		for _, launch := range []string{"success", "failure"} {
			t.Run(entry+"/"+launch, func(t *testing.T) {
				player := filepath.Join(bin, "mpv")
				if launch == "success" {
					// Capture forwarding and leave an IPC fixture for the owner to clean.
					script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$LUFFY_TEST_ARGS\"\nfor arg do\n case $arg in --input-ipc-server=*) : > \"${arg#--input-ipc-server=}\";; esac\ndone\n"
					if err := os.WriteFile(player, []byte(script), 0700); err != nil {
						t.Fatal(err)
					}
				} else if err := os.Remove(player); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				for _, path := range paths {
					if err := os.WriteFile(path, []byte("caller-owned subtitle"), 0600); err != nil {
						t.Fatal(err)
					}
				}

				var playErr error
				if entry == "Play" {
					_, playErr = Play("https://example.invalid/video", "test", "", "", subtitles, false, 0, HookContext{})
				} else {
					_, playErr = PlayWithControls("https://example.invalid/video", "test", "", "", subtitles, false, 0, HookContext{})
				}
				if launch == "success" && playErr != nil {
					t.Fatal(playErr)
				}
				if launch == "failure" && (playErr == nil || !strings.Contains(playErr.Error(), "failed to start player")) {
					t.Fatalf("expected startup failure, got %v", playErr)
				}
				for _, path := range paths {
					data, err := os.ReadFile(path)
					if err != nil || string(data) != "caller-owned subtitle" {
						t.Errorf("supplied file %q was deleted or changed: %q, %v", path, data, err)
					}
				}
				if launch == "success" {
					data, err := os.ReadFile(argsFile)
					if err != nil {
						t.Fatal(err)
					}
					for _, sub := range subtitles[1:] {
						if !strings.Contains(string(data), "--sub-file="+sub+"\n") {
							t.Errorf("subtitle %q was not forwarded", sub)
						}
					}
					if strings.Contains(string(data), "--sub-file=\n") {
						t.Error("empty subtitle was forwarded")
					}
				}
				entries, err := os.ReadDir(filepath.Join(tmp, "mpvsockets"))
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) != 0 {
					t.Errorf("IPC fixtures were not cleaned: %v", entries)
				}
			})
		}
	}
}
