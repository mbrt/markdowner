// Package output handles writing converted content to files or stdout.
package output

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
)

// Frontmatter holds the metadata written at the top of each Markdown file.
type Frontmatter struct {
	Title string    `yaml:"title"`
	URL   string    `yaml:"url"`
	Date  time.Time `yaml:"date"`
	Tags  []string  `yaml:"tags,omitempty"`
}

// Doc holds the complete content of a fetched page, ready to write to disk.
type Doc struct {
	Frontmatter Frontmatter
	Markdown    string
}

// WriteFile writes a Markdown file with YAML frontmatter to outDir.
// It returns the path of the written file.
func WriteFile(outDir string, doc Doc) (string, error) {
	filename := slugify(targetName(doc))
	fm := doc.Frontmatter
	body := doc.Markdown
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("creating output directory: %w", err)
	}
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return "", fmt.Errorf("marshaling frontmatter: %w", err)
	}
	content := "---\n" + string(fmBytes) + "---\n\n" + body
	path := filepath.Join(outDir, filename+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("writing file %q: %w", path, err)
	}
	return path, nil
}

func targetName(doc Doc) string {
	if doc.Frontmatter.Title != "" {
		return doc.Frontmatter.Title
	}
	u, err := url.Parse(doc.Frontmatter.URL)
	if err != nil {
		return doc.Frontmatter.URL
	}
	return u.Host + u.Path
}

var reNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(s)
	// Replace non-ASCII with spaces.
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return '-'
	}, s)
	s = reNonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 80 {
		s = s[:80]
		s = strings.TrimRight(s, "-")
	}
	if s == "" {
		s = "untitled"
	}
	return s
}
