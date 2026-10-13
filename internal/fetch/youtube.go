package fetch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mbrt/markdowner/internal/output"
)

// ytDLPPath is the yt-dlp executable used to extract YouTube metadata. Tests
// override this to point at a fake script.
var ytDLPPath = "yt-dlp"

const (
	// paraTargetRunes is the length after which a transcript paragraph is
	// closed at the next sentence boundary.
	paraTargetRunes = 500
	// paraSoftRunes is the length after which a paragraph is also closed at
	// the next pause in speech, for captions without punctuation.
	paraSoftRunes = 800
	// paraMaxRunes is the length after which a paragraph is closed at the
	// longest pause seen so far.
	paraMaxRunes = 1500
	// paraPause is the minimum gap between the start of two words that
	// counts as a pause. Typical gaps are around 250ms.
	paraPause = time.Second
)

// reSoundTag matches non-speech annotations such as "[Music]" or "[Applause]".
var reSoundTag = regexp.MustCompile(`\[[^\]]*\]`)

// isYouTubeURL returns true if rawURL points to a single YouTube video.
func isYouTubeURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	switch u.Hostname() {
	case "youtu.be":
		return len(parts) == 1 && parts[0] != ""
	case "youtube.com", "www.youtube.com", "m.youtube.com":
	default:
		return false
	}
	switch parts[0] {
	case "watch":
		return u.Query().Get("v") != ""
	case "shorts", "live", "embed":
		return len(parts) == 2 && parts[1] != ""
	}
	return false
}

type ytInfo struct {
	ID                string               `json:"id"`
	Title             string               `json:"title"`
	Channel           string               `json:"channel"`
	Uploader          string               `json:"uploader"`
	UploadDate        string               `json:"upload_date"`
	Language          string               `json:"language"`
	Chapters          []ytChapter          `json:"chapters"`
	Subtitles         map[string][]ytTrack `json:"subtitles"`
	AutomaticCaptions map[string][]ytTrack `json:"automatic_captions"`
}

type ytChapter struct {
	StartTime float64 `json:"start_time"`
	Title     string  `json:"title"`
}

type ytTrack struct {
	Ext string `json:"ext"`
	URL string `json:"url"`
}

type word struct {
	start time.Duration
	text  string
}

// youTube fetches the metadata and captions of a YouTube video and returns
// its transcript as a Doc.
func (c Client) youTube(ctx context.Context, pageURL string) (output.Doc, error) {
	info, err := ytDLPInfo(ctx, pageURL)
	if err != nil {
		return output.Doc{}, err
	}
	trackURL, err := pickCaptionTrack(info)
	if err != nil {
		return output.Doc{}, err
	}
	data, err := c.get(ctx, trackURL)
	if err != nil {
		return output.Doc{}, fmt.Errorf("fetching captions: %w", err)
	}
	words, err := parseJSON3(data)
	if err != nil {
		return output.Doc{}, fmt.Errorf("parsing captions: %w", err)
	}

	author := info.Channel
	if author == "" {
		author = info.Uploader
	}
	var date *time.Time
	if t, err := time.Parse("20060102", info.UploadDate); err == nil {
		date = &t
	}
	return output.Doc{
		Frontmatter: output.Frontmatter{
			Title:  info.Title,
			Author: author,
			URL:    pageURL,
			Date:   date,
			Saved:  time.Now().UTC(),
		},
		Markdown: transcriptMarkdown(words, info.Chapters),
	}, nil
}

func ytDLPInfo(ctx context.Context, pageURL string) (ytInfo, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, ytDLPPath,
		"--dump-json", "--skip-download", "--no-playlist", "--no-update", "--", pageURL)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return ytInfo{}, fmt.Errorf("yt-dlp is required for YouTube URLs: %w", err)
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return ytInfo{}, fmt.Errorf("running yt-dlp: %w: %s", err, msg)
		}
		return ytInfo{}, fmt.Errorf("running yt-dlp: %w", err)
	}
	var info ytInfo
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		return ytInfo{}, fmt.Errorf("parsing yt-dlp output: %w", err)
	}
	return info, nil
}

// pickCaptionTrack returns the URL of the json3 caption track in the video's
// original language, preferring human-written subtitles over automatic ones.
func pickCaptionTrack(info ytInfo) (string, error) {
	lang := originalLanguage(info)
	for _, tracks := range captionCandidates(info, lang) {
		for _, t := range tracks {
			if t.Ext == "json3" {
				return t.URL, nil
			}
		}
	}
	if lang == "" {
		return "", errors.New("no captions available (unknown original language)")
	}
	return "", fmt.Errorf("no captions available in original language %q", lang)
}

// captionCandidates returns the caption tracks matching lang, in order of
// preference.
func captionCandidates(info ytInfo, lang string) [][]ytTrack {
	manual := slices.Sorted(func(yield func(string) bool) {
		for k := range info.Subtitles {
			if k != "live_chat" && !yield(k) {
				return
			}
		}
	})
	if lang == "" {
		// Without knowing the original language, only an unambiguous
		// manual track can be picked.
		if len(manual) == 1 {
			return [][]ytTrack{info.Subtitles[manual[0]]}
		}
		return nil
	}

	// Exact matches first (e.g. "en-US"), then same base language ("en").
	var candidates [][]ytTrack
	if t, ok := info.Subtitles[lang]; ok {
		candidates = append(candidates, t)
	}
	for _, k := range manual {
		if k != lang && baseLang(k) == baseLang(lang) {
			candidates = append(candidates, info.Subtitles[k])
		}
	}
	return append(candidates, autoCaptionCandidates(info, lang)...)
}

// autoCaptionCandidates returns the automatic caption tracks matching lang,
// in order of preference. Tracks with an "-orig" suffix are transcribed from
// the audio, while other languages are machine translations.
func autoCaptionCandidates(info ytInfo, lang string) [][]ytTrack {
	auto := slices.Sorted(maps.Keys(info.AutomaticCaptions))
	candidates := [][]ytTrack{info.AutomaticCaptions[lang+"-orig"]}
	for _, k := range auto {
		if l, ok := strings.CutSuffix(k, "-orig"); ok && l != lang && baseLang(l) == baseLang(lang) {
			candidates = append(candidates, info.AutomaticCaptions[k])
		}
	}
	return append(candidates, info.AutomaticCaptions[lang], info.AutomaticCaptions[baseLang(lang)])
}

// originalLanguage returns the spoken language of the video. When the
// metadata doesn't say, it falls back to the language of the only "-orig"
// automatic caption track. Videos with auto-dubbed audio have one "-orig"
// track per dub, so the choice is ambiguous when there are several.
func originalLanguage(info ytInfo) string {
	if info.Language != "" {
		return info.Language
	}
	var orig []string
	for k := range info.AutomaticCaptions {
		if lang, ok := strings.CutSuffix(k, "-orig"); ok {
			orig = append(orig, lang)
		}
	}
	if len(orig) == 1 {
		return orig[0]
	}
	return ""
}

func baseLang(lang string) string {
	base, _, _ := strings.Cut(lang, "-")
	return base
}

func (c Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// parseJSON3 parses YouTube's json3 caption format into timed words,
// dropping non-speech annotations.
func parseJSON3(data []byte) ([]word, error) {
	var doc struct {
		Events []struct {
			TStartMs int64 `json:"tStartMs"`
			Segs     []struct {
				UTF8      string `json:"utf8"`
				TOffsetMs int64  `json:"tOffsetMs"`
			} `json:"segs"`
		} `json:"events"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	var words []word
	for _, ev := range doc.Events {
		for _, s := range ev.Segs {
			start := time.Duration(ev.TStartMs+s.TOffsetMs) * time.Millisecond
			for w := range strings.FieldsSeq(reSoundTag.ReplaceAllString(s.UTF8, " ")) {
				words = append(words, word{start: start, text: w})
			}
		}
	}
	return words, nil
}

// transcriptMarkdown renders words as prose paragraphs, with one section per
// chapter when chapters are available.
func transcriptMarkdown(words []word, chapters []ytChapter) string {
	type section struct {
		title string
		words []word
	}
	// Words before the first chapter (if any) go in an untitled section.
	sections := []section{{}}
	next := 0
	for _, w := range words {
		for next < len(chapters) &&
			w.start >= time.Duration(chapters[next].StartTime*float64(time.Second)) {
			sections = append(sections, section{title: chapters[next].Title})
			next++
		}
		last := &sections[len(sections)-1]
		last.words = append(last.words, w)
	}

	var blocks []string
	for _, s := range sections {
		if len(s.words) == 0 {
			continue
		}
		if s.title != "" {
			blocks = append(blocks, "## "+s.title)
		}
		blocks = append(blocks, paragraphs(s.words)...)
	}
	if len(blocks) == 0 {
		return ""
	}
	return strings.Join(blocks, "\n\n") + "\n"
}

// paragraphs groups words into paragraphs of at least paraTargetRunes. A
// paragraph ends at the first sentence boundary past the target. Captions
// without punctuation end at the first pause in speech past paraSoftRunes
// instead, or at the longest pause once paraMaxRunes is reached.
func paragraphs(words []word) []string {
	var (
		paras []string
		cur   []string
		n     int
		// Index into cur of the word after the longest pause so far.
		best    int
		bestGap time.Duration
	)
	flush := func(at int) {
		paras = append(paras, strings.Join(cur[:at], " "))
		cur = slices.Clone(cur[at:])
		n, best, bestGap = 0, 0, 0
		for _, w := range cur {
			n += utf8.RuneCountInString(w) + 1
		}
	}
	for i, w := range words {
		if i > 0 && n >= paraTargetRunes {
			gap := w.start - words[i-1].start
			switch {
			case endsSentence(words[i-1].text),
				n >= paraSoftRunes && gap >= paraPause:
				flush(len(cur))
			case gap > bestGap:
				best, bestGap = len(cur), gap
			}
		}
		cur = append(cur, w.text)
		n += utf8.RuneCountInString(w.text) + 1
		if n >= paraMaxRunes {
			if best == 0 {
				best = len(cur)
			}
			flush(best)
		}
	}
	if len(cur) > 0 {
		paras = append(paras, strings.Join(cur, " "))
	}
	return paras
}

func endsSentence(word string) bool {
	word = strings.TrimRight(word, `"')]»”’`)
	r, _ := utf8.DecodeLastRuneInString(word)
	return strings.ContainsRune(".!?…。！？", r)
}
