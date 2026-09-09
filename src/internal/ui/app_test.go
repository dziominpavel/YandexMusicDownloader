package ui

import (
	"strings"
	"testing"

	"yamdl/internal/downloader"
)

func TestFormatEvent(t *testing.T) {
	cases := []struct {
		in   downloader.Event
		text string
		kind string
	}{
		{downloader.Event{Kind: downloader.KindDownloading, Label: "A — T"}, "[downloading] A — T", "info"},
		{downloader.Event{Kind: downloader.KindDone, Label: "A — T", Format: "FLAC"}, "[done] A — T (FLAC)", "done"},
		{downloader.Event{Kind: downloader.KindDone, Label: "A — T", Format: "MP3", Fallback: true}, "[done] A — T (MP3) [fallback MP3]", "done"},
		{downloader.Event{Kind: downloader.KindDone, Label: "A — T", Format: "FLAC", Converted: true}, "[done] A — T (FLAC) [ALAC→FLAC]", "done"},
		{downloader.Event{Kind: downloader.KindSkipped, Label: "A — T"}, "[skip] A — T (already exists)", "info"},
		{downloader.Event{Kind: downloader.KindSkipped, Label: "A — T", Detail: downloader.SkipDuplicate}, "[skip] A — T (duplicate)", "info"},
		{downloader.Event{Kind: downloader.KindSkipped, Label: "A — T", Detail: downloader.SkipUnavailable}, "[skip] A — T (unavailable)", "info"},
		{downloader.Event{Kind: downloader.KindFailed, Label: "A — T", Detail: "boom"}, "[error] A — T: boom", "err"},
	}
	for _, c := range cases {
		text, kind := formatEvent(c.in)
		if text != c.text || kind != c.kind {
			t.Errorf("got (%q,%q) want (%q,%q)", text, kind, c.text, c.kind)
		}
	}
}

func TestFormatTrackStatus(t *testing.T) {
	cases := []struct {
		in   downloader.Event
		text string
		kind string
	}{
		{downloader.Event{Kind: downloader.KindDownloading}, "[downloading]", "info"},
		{downloader.Event{Kind: downloader.KindDone, Format: "FLAC"}, "[done] (FLAC)", "done"},
		{downloader.Event{Kind: downloader.KindDone, Format: "MP3", Fallback: true}, "[done] (MP3) [fallback MP3]", "done"},
		{downloader.Event{Kind: downloader.KindDone, Format: "FLAC", Converted: true}, "[done] (FLAC) [ALAC→FLAC]", "done"},
		{downloader.Event{Kind: downloader.KindSkipped}, "[skip] (already exists)", "info"},
		{downloader.Event{Kind: downloader.KindSkipped, Detail: downloader.SkipDuplicate}, "[skip] (duplicate)", "info"},
		{downloader.Event{Kind: downloader.KindSkipped, Detail: downloader.SkipUnavailable}, "[skip] (unavailable)", "info"},
		{downloader.Event{Kind: downloader.KindFailed, Detail: "boom"}, "[error] boom", "err"},
	}
	for _, c := range cases {
		text, kind := formatTrackStatus(c.in)
		if text != c.text || kind != c.kind {
			t.Errorf("got (%q,%q) want (%q,%q)", text, kind, c.text, c.kind)
		}
	}
}

func TestDownloadWithoutToken(t *testing.T) {
	a := &App{}
	got := a.Download("https://music.yandex.ru/album/1/track/2")
	if !strings.HasPrefix(got, "[error]") {
		t.Fatalf("want [error], got %q", got)
	}
}

func TestDownloadBadLink(t *testing.T) {
	a := &App{client: downloader.NewClient("")}
	got := a.Download("not a link")
	if !strings.HasPrefix(got, "[error]") {
		t.Fatalf("want [error], got %q", got)
	}
}

