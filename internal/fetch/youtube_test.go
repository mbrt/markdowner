package fetch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsYouTubeURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://www.youtube.com/watch?v=aircAruvnKk", true},
		{"https://youtube.com/watch?v=aircAruvnKk&t=42s", true},
		{"https://m.youtube.com/watch?v=aircAruvnKk", true},
		{"https://youtu.be/aircAruvnKk", true},
		{"https://youtu.be/aircAruvnKk?si=abc", true},
		{"https://www.youtube.com/shorts/abc123", true},
		{"https://www.youtube.com/live/abc123", true},
		{"https://www.youtube.com/embed/abc123", true},
		// Not single videos.
		{"https://www.youtube.com/watch", false},
		{"https://www.youtube.com/playlist?list=PL123", false},
		{"https://www.youtube.com/@3blue1brown", false},
		{"https://www.youtube.com/", false},
		{"https://youtu.be/", false},
		// Wrong host.
		{"https://example.com/watch?v=abc", false},
		{"not a url", false},
	}
	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			assert.Equal(t, tc.want, isYouTubeURL(tc.url))
		})
	}
}

// fakeYTDLP installs a fake yt-dlp executable that prints info as JSON.
func fakeYTDLP(t *testing.T, info any) {
	t.Helper()
	dir := t.TempDir()
	b, err := json.Marshal(info)
	require.NoError(t, err)
	jsonPath := filepath.Join(dir, "info.json")
	require.NoError(t, os.WriteFile(jsonPath, b, 0o600))
	script := filepath.Join(dir, "yt-dlp")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\ncat "+jsonPath+"\n"), 0o700)) //nolint:gosec

	orig := ytDLPPath
	ytDLPPath = script
	t.Cleanup(func() { ytDLPPath = orig })
}

// captionServer serves json3 caption files keyed by URL path.
func captionServer(t *testing.T, files map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func tracks(srv *httptest.Server, path string) []map[string]string {
	return []map[string]string{
		{"ext": "vtt", "url": srv.URL + path + ".vtt"},
		{"ext": "json3", "url": srv.URL + path},
	}
}

const autoJSON3 = `{"events":[
{"tStartMs":0,"dDurationMs":100000,"id":1},
{"tStartMs":0,"segs":[{"utf8":"[Music]"}]},
{"tStartMs":4390,"aAppend":1,"segs":[{"utf8":"\n"}]},
{"tStartMs":4400,"segs":[{"utf8":"hello"},{"utf8":" there"}]},
{"tStartMs":6000,"aAppend":1,"segs":[{"utf8":"\n"}]},
{"tStartMs":6010,"segs":[{"utf8":"this is auto"}]},
{"tStartMs":65000,"segs":[{"utf8":"second chapter"},{"utf8":" [Applause]"}]}
]}`

func TestYouTubeURL(t *testing.T) {
	srv := captionServer(t, map[string]string{
		"/auto-orig": autoJSON3,
		"/manual-en": `{"events":[{"tStartMs":1000,"segs":[{"utf8":"Hello there."}]},` +
			`{"tStartMs":2000,"segs":[{"utf8":"This is manual."}]}]}`,
		"/manual-de": `{"events":[{"tStartMs":1000,"segs":[{"utf8":"Hallo."}]}]}`,
	})

	t.Run("prefers manual subtitles in original language", func(t *testing.T) {
		fakeYTDLP(t, map[string]any{
			"id":          "abc",
			"title":       "A Video",
			"channel":     "Some Channel",
			"upload_date": "20171005",
			"language":    "en-US",
			"subtitles": map[string]any{
				"de": tracks(srv, "/manual-de"),
				"en": tracks(srv, "/manual-en"),
			},
			"automatic_captions": map[string]any{
				"en-orig": tracks(srv, "/auto-orig"),
				"de":      tracks(srv, "/auto-orig"),
			},
		})
		doc, err := Client{}.URL(context.Background(), "https://youtu.be/abc", false, 0)
		require.NoError(t, err)
		assert.Equal(t, "A Video", doc.Frontmatter.Title)
		assert.Equal(t, "Some Channel", doc.Frontmatter.Author)
		assert.Equal(t, "https://youtu.be/abc", doc.Frontmatter.URL)
		require.NotNil(t, doc.Frontmatter.Date)
		assert.Equal(t, time.Date(2017, 10, 5, 0, 0, 0, 0, time.UTC), *doc.Frontmatter.Date)
		assert.Equal(t, "Hello there. This is manual.\n", doc.Markdown)
	})

	t.Run("falls back to automatic captions with chapters", func(t *testing.T) {
		fakeYTDLP(t, map[string]any{
			"title":    "Auto Video",
			"uploader": "Uploader",
			"chapters": []map[string]any{
				{"start_time": 0, "title": "Intro"},
				{"start_time": 60, "title": "Main"},
			},
			"subtitles": map[string]any{
				// A translation, not the original language.
				"de": tracks(srv, "/manual-de"),
			},
			"automatic_captions": map[string]any{
				"en-orig": tracks(srv, "/auto-orig"),
			},
		})
		doc, err := Client{}.URL(context.Background(), "https://youtu.be/abc", false, 0)
		require.NoError(t, err)
		assert.Equal(t, "Uploader", doc.Frontmatter.Author)
		assert.Nil(t, doc.Frontmatter.Date)
		assert.Equal(t,
			"## Intro\n\nhello there this is auto\n\n## Main\n\nsecond chapter\n",
			doc.Markdown)
	})

	t.Run("auto-dubbed video uses the original audio language", func(t *testing.T) {
		// Auto-dubbed videos have one "-orig" track per audio dub.
		fakeYTDLP(t, map[string]any{
			"title":    "Dubbed",
			"language": "en-US",
			"automatic_captions": map[string]any{
				"de-DE-orig": tracks(srv, "/manual-de"),
				"en-orig":    tracks(srv, "/auto-orig"),
				"uk-orig":    tracks(srv, "/manual-de"),
			},
		})
		doc, err := Client{}.URL(context.Background(), "https://youtu.be/abc", false, 0)
		require.NoError(t, err)
		assert.Equal(t, "hello there this is auto second chapter\n", doc.Markdown)
	})

	t.Run("no captions in original language", func(t *testing.T) {
		fakeYTDLP(t, map[string]any{
			"title":    "No Captions",
			"language": "it",
			"subtitles": map[string]any{
				"en": tracks(srv, "/manual-en"),
			},
		})
		_, err := Client{}.URL(context.Background(), "https://youtu.be/abc", false, 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `no captions available in original language "it"`)
	})

	t.Run("unknown language with single manual track", func(t *testing.T) {
		fakeYTDLP(t, map[string]any{
			"title":     "Single Track",
			"subtitles": map[string]any{"en": tracks(srv, "/manual-en")},
		})
		doc, err := Client{}.URL(context.Background(), "https://youtu.be/abc", false, 0)
		require.NoError(t, err)
		assert.Equal(t, "Hello there. This is manual.\n", doc.Markdown)
	})

	t.Run("unknown language with ambiguous tracks", func(t *testing.T) {
		fakeYTDLP(t, map[string]any{
			"title": "Ambiguous",
			"subtitles": map[string]any{
				"de": tracks(srv, "/manual-de"),
				"en": tracks(srv, "/manual-en"),
			},
		})
		_, err := Client{}.URL(context.Background(), "https://youtu.be/abc", false, 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown original language")
	})
}

func TestYouTubeMissingYTDLP(t *testing.T) {
	orig := ytDLPPath
	ytDLPPath = filepath.Join(t.TempDir(), "does-not-exist")
	t.Cleanup(func() { ytDLPPath = orig })

	_, err := Client{}.URL(context.Background(), "https://youtu.be/abc", false, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "yt-dlp")
}

// timedWords splits text into words spaced by gap, with an extra pause before
// each word in pauses (indexed by word position).
func timedWords(text string, gap time.Duration, pauses map[int]time.Duration) []word {
	var (
		words []word
		t     time.Duration
	)
	for i, w := range strings.Fields(text) {
		t += gap + pauses[i]
		words = append(words, word{start: t, text: w})
	}
	return words
}

func TestParagraphs(t *testing.T) {
	t.Run("sentence boundaries", func(t *testing.T) {
		sentence := strings.Repeat("word ", 60) + "end."
		text := strings.Repeat(sentence+" ", 4)
		paras := paragraphs(timedWords(text, 250*time.Millisecond, nil))
		assert.Len(t, paras, 2)
		for _, p := range paras {
			assert.True(t, strings.HasSuffix(p, "end."), p)
		}
	})

	t.Run("pauses without punctuation", func(t *testing.T) {
		// 5 runes per word: a pause after 200 words (~1000 runes) is past
		// paraSoftRunes, so the paragraph ends there.
		text := strings.Repeat("word ", 300)
		paras := paragraphs(timedWords(text, 250*time.Millisecond, map[int]time.Duration{
			100: 2 * time.Second, // too early: below paraSoftRunes
			200: 2 * time.Second,
		}))
		require.Len(t, paras, 2)
		assert.Len(t, strings.Fields(paras[0]), 200)
		assert.Len(t, strings.Fields(paras[1]), 100)
	})

	t.Run("longest pause at hard limit", func(t *testing.T) {
		// No pause reaches paraPause, so the split happens at the longest
		// gap once paraMaxRunes is reached.
		text := strings.Repeat("word ", 400)
		paras := paragraphs(timedWords(text, 250*time.Millisecond, map[int]time.Duration{
			150: 300 * time.Millisecond,
			180: 500 * time.Millisecond,
		}))
		require.Len(t, paras, 2)
		assert.Len(t, strings.Fields(paras[0]), 180)
		assert.Len(t, strings.Fields(paras[1]), 220)
	})

	t.Run("hard limit without timing", func(t *testing.T) {
		text := strings.Repeat("word ", 1000)
		paras := paragraphs(timedWords(text, 0, nil))
		assert.Greater(t, len(paras), 1)
		for _, p := range paras {
			assert.LessOrEqual(t, len(p), paraMaxRunes)
		}
	})
}
