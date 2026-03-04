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

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	fm := Frontmatter{
		Title: "Test Article",
		URL:   "https://example.com/test",
		Date:  time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC),
		Tags:  []string{"go", "testing"},
	}
	body := "# Test\n\nHello world."

	doc := Doc{
		Frontmatter: fm,
		Markdown:    body,
	}
	_, err := WriteFile(dir, doc)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "test-article.md"))
	require.NoError(t, err)
	s := string(content)

	assert.True(t, strings.HasPrefix(s, "---\n"), "file should start with YAML frontmatter delimiter")
	assert.Contains(t, s, "title: Test Article")
	assert.Contains(t, s, "url: https://example.com/test")
	assert.Contains(t, s, "- go")
	assert.Contains(t, s, "Hello world.")
}

func TestWriteFile_NoTags(t *testing.T) {
	dir := t.TempDir()
	fm := Frontmatter{
		Title: "No Tags",
		URL:   "https://example.com/notags",
		Date:  time.Now(),
	}

	_, err := WriteFile(dir, Doc{Frontmatter: fm, Markdown: "body"})
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "no-tags.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(content), "tags:")
}

func TestWriteFile_CreatesDirectory(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "sub", "dir")
	fm := Frontmatter{Title: "X", URL: "https://x.com", Date: time.Now()}

	_, err := WriteFile(dir, Doc{Frontmatter: fm, Markdown: ""})
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "x.md"))
	assert.NoError(t, err)
}

func TestWriteFile_EmptyTitleFallsBackToURL(t *testing.T) {
	dir := t.TempDir()
	fm := Frontmatter{Title: "", URL: "https://example.com/some/page", Date: time.Now()}

	path, err := WriteFile(dir, Doc{Frontmatter: fm, Markdown: "body"})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "example-com-some-page.md"), path)
}
