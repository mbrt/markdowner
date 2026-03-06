package output

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"  leading and trailing  ", "leading-and-trailing"},
		{"multiple   spaces", "multiple-spaces"},
		{"Special! @#$ Characters", "special-characters"},
		{"café au lait", "caf-au-lait"},
		{"", "untitled"},
		{"   ", "untitled"},
		{"already-slug", "already-slug"},
		{"UPPER CASE", "upper-case"},
		{strings.Repeat("a", 100), strings.Repeat("a", 80)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, slugify(tt.input))
		})
	}
}

func TestSlugify_NoTrailingDash(t *testing.T) {
	s := strings.Repeat("a", 79) + "-extra"
	got := slugify(s)
	assert.False(t, strings.HasSuffix(got, "-"), "result should not have trailing dash: %q", got)
}

func TestWriteDoc(t *testing.T) {
	dir := t.TempDir()
	d := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	fm := Frontmatter{
		Title: "Test Article",
		URL:   "https://example.com/test",
		Date:  &d,
		Saved: time.Date(2024, 4, 1, 8, 0, 0, 0, time.UTC),
		Tags:  []string{"go", "testing"},
	}
	body := "# Test\n\nHello world."

	doc := Doc{
		Frontmatter: fm,
		Markdown:    body,
	}
	_, err := NewWriter(dir, ModeFlat).WriteDoc(doc)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "test-article.md"))
	require.NoError(t, err)
	s := string(content)

	assert.True(t, strings.HasPrefix(s, "---\n"), "file should start with YAML frontmatter delimiter")
	assert.Contains(t, s, "date: 2024-03-01T12:00:00Z")
	assert.Contains(t, s, "saved: 2024-04-01T08:00:00Z")
	assert.Contains(t, s, "title: Test Article")
	assert.Contains(t, s, "url: https://example.com/test")
	assert.Contains(t, s, "- go")
	assert.Contains(t, s, "Hello world.")
}

func TestWriteDoc_NoTags(t *testing.T) {
	dir := t.TempDir()
	fm := Frontmatter{
		Title: "No Tags",
		URL:   "https://example.com/notags",
		Saved: time.Now(),
	}

	_, err := NewWriter(dir, ModeFlat).WriteDoc(Doc{Frontmatter: fm, Markdown: "body"})
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "no-tags.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(content), "tags:")
	assert.NotContains(t, string(content), "date:")
}

func TestWriteDoc_CreatesDirectory(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "sub", "dir")
	fm := Frontmatter{Title: "X", URL: "https://x.com", Saved: time.Now()}

	_, err := NewWriter(dir, ModeFlat).WriteDoc(Doc{Frontmatter: fm, Markdown: ""})
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "x.md"))
	assert.NoError(t, err)
}

func TestWriteDoc_WritesImages(t *testing.T) {
	dir := t.TempDir()
	fm := Frontmatter{Title: "Img Test", URL: "https://example.com/img", Saved: time.Now()}
	doc := Doc{
		Frontmatter: fm,
		Markdown:    "![pic](img/abc123.png)",
		Images: map[string][]byte{
			"img/abc123.png": []byte("fakepng"),
		},
	}

	_, err := NewWriter(dir, ModeFlat).WriteDoc(doc)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "img", "abc123.png"))
	require.NoError(t, err)
	assert.Equal(t, []byte("fakepng"), data)
}

func TestWriteDoc_EmptyTitleFallsBackToURL(t *testing.T) {
	dir := t.TempDir()
	fm := Frontmatter{Title: "", URL: "https://example.com/some/page", Saved: time.Now()}

	path, err := NewWriter(dir, ModeFlat).WriteDoc(Doc{Frontmatter: fm, Markdown: "body"})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "example-com-some-page.md"), path)
}

func TestWeekSubDir(t *testing.T) {
	tests := []struct {
		name    string
		saved   time.Time
		wantSub string
	}{
		{
			name:    "mid year week",
			saved:   time.Date(2024, 3, 18, 0, 0, 0, 0, time.UTC), // ISO week 12
			wantSub: filepath.Join("2024", "w12"),
		},
		{
			name:    "first week zero-padded",
			saved:   time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC), // ISO week 2
			wantSub: filepath.Join("2024", "w02"),
		},
		{
			name:    "week 1 of year",
			saved:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), // ISO week 1 of 2024
			wantSub: filepath.Join("2024", "w01"),
		},
		{
			name:    "late December in next ISO year",
			saved:   time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC), // ISO week 53 of 2020
			wantSub: filepath.Join("2020", "w53"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := weekSubDir("base", tt.saved)
			assert.Equal(t, filepath.Join("base", tt.wantSub), got)
		})
	}
}

func TestWriteDoc_ModeWeek(t *testing.T) {
	dir := t.TempDir()
	saved := time.Date(2024, 3, 18, 0, 0, 0, 0, time.UTC) // ISO week 12
	fm := Frontmatter{Title: "Week Test", URL: "https://example.com/w", Saved: saved}

	path, err := NewWriter(dir, ModeWeek).WriteDoc(Doc{Frontmatter: fm, Markdown: "body"})
	require.NoError(t, err)

	expected := filepath.Join(dir, "2024", "w12", "week-test.md")
	assert.Equal(t, expected, path)
	_, err = os.Stat(expected)
	assert.NoError(t, err)
}
