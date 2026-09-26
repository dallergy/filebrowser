package fbhttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/filebrowser/filebrowser/v2/files"
)

func TestNormalizeSRTLineBreaks(t *testing.T) {
	input := []byte("first<br>second<BR/>third<br />fourth<br class=\"x\">fifth")
	got := string(normalizeSRTLineBreaks(input))
	want := "first\nsecond\nthird\nfourth\nfifth"
	if got != want {
		t.Fatalf("normalizeSRTLineBreaks() = %q, want %q", got, want)
	}
}

func TestSubtitleFileHandlerConvertsSRTBreakTags(t *testing.T) {
	fs := afero.NewMemMapFs()
	const path = "/sample.srt"
	const content = "1\n" +
		"00:00:01,000 --> 00:00:02,000\n" +
		"First<br>Second<BR/>Third<br />Fourth\n\n"

	if err := afero.WriteFile(fs, path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write subtitle: %v", err)
	}
	info, err := fs.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat subtitle: %v", err)
	}

	file := &files.FileInfo{
		Fs:      fs,
		Path:    path,
		Name:    "sample.srt",
		ModTime: info.ModTime(),
	}
	req := httptest.NewRequest(http.MethodGet, "/api/subtitle/sample.srt?inline=true", http.NoBody)
	rec := httptest.NewRecorder()

	status, err := subtitleFileHandler(rec, req, file)
	if err != nil {
		t.Fatalf("subtitleFileHandler returned error: %v", err)
	}
	if status != 0 {
		t.Fatalf("subtitleFileHandler status = %d, want 0", status)
	}

	body := rec.Body.String()
	if strings.Contains(body, "FirstSecond") {
		t.Fatalf("WebVTT output collapsed SRT <br> tags: %q", body)
	}
	if !strings.Contains(body, "First\nSecond\nThird\nFourth") {
		t.Fatalf("WebVTT output = %q, want converted SRT <br> tags as line breaks", body)
	}
}

// Regression for GHSA-448h-jr2h-3vhp: conversion read the whole subtitle file
// into memory, several times over, with no size limit, so a large .srt/.ass
// file could exhaust server memory.
func TestSubtitleFileHandlerRejectsOversizedFiles(t *testing.T) {
	cue := "1\n00:00:01,000 --> 00:00:02,000\nline\n\n"
	oversized := strings.Repeat(cue, maxSubtitleSize/len(cue)+1)

	for _, name := range []string{"big.srt", "big.ass"} {
		t.Run(name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			if err := afero.WriteFile(fs, "/"+name, []byte(oversized), 0o644); err != nil {
				t.Fatal(err)
			}

			for _, size := range []int64{int64(len(oversized)), 0} {
				// Size 0 stands in for a file that grew after it was stat'ed.
				file := &files.FileInfo{Fs: fs, Path: "/" + name, Name: name, Size: size}
				req := httptest.NewRequest(http.MethodGet, "/api/subtitle/"+name, http.NoBody)
				rec := httptest.NewRecorder()

				status, _ := subtitleFileHandler(rec, req, file)
				if status != http.StatusBadRequest {
					t.Errorf("stat size %d: status = %d; want 400", size, status)
				}
				if rec.Body.Len() != 0 {
					t.Errorf("VULNERABLE: stat size %d: oversized subtitle was converted (%d bytes)", size, rec.Body.Len())
				}
			}
		})
	}
}
