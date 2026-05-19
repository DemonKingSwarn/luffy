package core

import "testing"

func TestParseM3U8SubtitleAttributesWithQuotedComma(t *testing.T) {
	attrs := parseM3U8Attributes(`TYPE=SUBTITLES,GROUP-ID="subs",NAME="English, CC",LANGUAGE="en",URI="subtitles/eng/prog_index.m3u8"`)

	if attrs["TYPE"] != "SUBTITLES" {
		t.Fatalf("TYPE = %q, want SUBTITLES", attrs["TYPE"])
	}
	if attrs["NAME"] != "English, CC" {
		t.Fatalf("NAME = %q, want English, CC", attrs["NAME"])
	}
	if attrs["URI"] != "subtitles/eng/prog_index.m3u8" {
		t.Fatalf("URI = %q, want subtitles/eng/prog_index.m3u8", attrs["URI"])
	}
}

func TestIsEnglishSubtitle(t *testing.T) {
	if !isEnglishSubtitle("en", "English") {
		t.Fatal("expected en/English to be treated as English")
	}
	if isEnglishSubtitle("fr", "French") {
		t.Fatal("expected fr/French not to be treated as English")
	}
}
