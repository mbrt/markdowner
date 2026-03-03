package output

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
			got := Slugify(tt.input)
			if got != tt.want {
				t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSlugify_NoTrailingDash(t *testing.T) {
	// A string that truncates at a dash boundary.
	s := strings.Repeat("a", 79) + "-extra"
	got := Slugify(s)
	if strings.HasSuffix(got, "-") {
		t.Errorf("Slugify result has trailing dash: %q", got)
	}
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

	if err := WriteFile(dir, "test-article", fm, body); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "test-article.md"))
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}
	s := string(content)

	if !strings.HasPrefix(s, "---\n") {
		t.Errorf("file should start with YAML frontmatter delimiter; got: %q", s[:20])
	}
	if !strings.Contains(s, "title: Test Article") {
		t.Errorf("frontmatter missing title; got:\n%s", s)
	}
	if !strings.Contains(s, "url: https://example.com/test") {
		t.Errorf("frontmatter missing url; got:\n%s", s)
	}
	if !strings.Contains(s, "- go") {
		t.Errorf("frontmatter missing tags; got:\n%s", s)
	}
	if !strings.Contains(s, "Hello world.") {
		t.Errorf("file missing body content; got:\n%s", s)
	}
}

func TestWriteFile_NoTags(t *testing.T) {
	dir := t.TempDir()
	fm := Frontmatter{
		Title: "No Tags",
		URL:   "https://example.com/notags",
		Date:  time.Now(),
	}

	if err := WriteFile(dir, "no-tags", fm, "body"); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "no-tags.md"))
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}
	// tags field should be omitted when empty
	if strings.Contains(string(content), "tags:") {
		t.Errorf("expected no tags field in frontmatter; got:\n%s", string(content))
	}
}

func TestWriteFile_CreatesDirectory(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "sub", "dir")
	fm := Frontmatter{Title: "X", URL: "https://x.com", Date: time.Now()}

	if err := WriteFile(dir, "x", fm, ""); err != nil {
		t.Fatalf("WriteFile() should create directory, got error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "x.md")); err != nil {
		t.Errorf("expected file to exist: %v", err)
	}
}
