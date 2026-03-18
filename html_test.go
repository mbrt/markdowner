package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mbrt/markdowner/internal/fetch"
	"github.com/mbrt/markdowner/internal/output"
)

const simpleHTML = `<!DOCTYPE html>
<html>
<head><title>Test Article</title></head>
<body><article><p>Hello from HTML.</p></article></body>
</html>`

func collectHTMLResults(ch <-chan output.Result) []output.Result {
	var results []output.Result
	for r := range ch {
		results = append(results, r)
	}
	return results
}

func TestConvertHTMLBytes_Basic(t *testing.T) {
	doc := convertHTMLBytes(context.Background(), simpleHTML, "https://example.com/test", fetch.Overrides{})
	assert.NoError(t, doc.Err)
	assert.Contains(t, doc.Doc.Markdown, "Hello from HTML")
	assert.Equal(t, "https://example.com/test", doc.Doc.Frontmatter.URL)
}

func TestConvertHTMLBytes_EmptyBaseURL(t *testing.T) {
	doc := convertHTMLBytes(context.Background(), simpleHTML, "", fetch.Overrides{})
	assert.NoError(t, doc.Err)
	assert.Contains(t, doc.Doc.Markdown, "Hello from HTML")
	assert.Equal(t, "", doc.Doc.Frontmatter.URL)
}

func TestConvertHTMLBytes_AppliesOverrides(t *testing.T) {
	d := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	overrides := fetch.Overrides{
		Title:  "Custom Title",
		Author: "Custom Author",
		Source: "test-source",
		Date:   &d,
		Tags:   []string{"go", "test"},
	}
	doc := convertHTMLBytes(context.Background(), simpleHTML, "https://example.com/", overrides)
	require.NoError(t, doc.Err)
	assert.Equal(t, "Custom Title", doc.Doc.Frontmatter.Title)
	assert.Equal(t, "Custom Author", doc.Doc.Frontmatter.Author)
	assert.Equal(t, "test-source", doc.Doc.Frontmatter.Source)
	assert.Equal(t, &d, doc.Doc.Frontmatter.Date)
	assert.Equal(t, []string{"go", "test"}, doc.Doc.Frontmatter.Tags)
}

func TestConvertHTMLBytes_InvalidHTML_StillProducesDoc(t *testing.T) {
	// go-readability tolerates malformed HTML; only a bad base URL should fail.
	doc := convertHTMLBytes(context.Background(), "<not html at all>", "://bad-url", fetch.Overrides{})
	assert.Error(t, doc.Err)
	// The stub doc should carry the URL and a valid Saved timestamp.
	assert.Equal(t, "://bad-url", doc.Doc.Frontmatter.URL)
	assert.False(t, doc.Doc.Frontmatter.Saved.IsZero())
}

func TestConvertHTMLFiles_SingleFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "article.html")
	require.NoError(t, os.WriteFile(p, []byte(simpleHTML), 0o644))

	results := collectHTMLResults(convertHTMLFiles(context.Background(), []string{p}, "", fetch.Overrides{}))
	require.Len(t, results, 1)
	assert.NoError(t, results[0].Err)
	assert.Contains(t, results[0].Doc.Markdown, "Hello from HTML")
	// URL should be the file:// path when --url is not specified.
	assert.True(t, strings.HasPrefix(results[0].Doc.Frontmatter.URL, "file://"),
		"expected file:// URL, got %q", results[0].Doc.Frontmatter.URL)
	assert.Contains(t, results[0].Doc.Frontmatter.URL, "article.html")
}

func TestConvertHTMLFiles_URLOverride(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "page.html")
	require.NoError(t, os.WriteFile(p, []byte(simpleHTML), 0o644))

	results := collectHTMLResults(convertHTMLFiles(context.Background(), []string{p}, "https://example.com/page", fetch.Overrides{}))
	require.Len(t, results, 1)
	assert.NoError(t, results[0].Err)
	assert.Equal(t, "https://example.com/page", results[0].Doc.Frontmatter.URL)
}

func TestConvertHTMLFiles_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.html")
	p2 := filepath.Join(dir, "b.html")
	require.NoError(t, os.WriteFile(p1, []byte(simpleHTML), 0o644))
	require.NoError(t, os.WriteFile(p2, []byte(simpleHTML), 0o644))

	results := collectHTMLResults(convertHTMLFiles(context.Background(), []string{p1, p2}, "", fetch.Overrides{}))
	require.Len(t, results, 2)
	for _, r := range results {
		assert.NoError(t, r.Err)
		assert.Contains(t, r.Doc.Markdown, "Hello from HTML")
	}
}

func TestConvertHTMLFiles_MissingFile(t *testing.T) {
	results := collectHTMLResults(convertHTMLFiles(context.Background(), []string{"/nonexistent/file.html"}, "", fetch.Overrides{}))
	require.Len(t, results, 1)
	assert.ErrorContains(t, results[0].Err, "reading")
}

func TestConvertHTMLFiles_AppliesOverrides(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "doc.html")
	require.NoError(t, os.WriteFile(p, []byte(simpleHTML), 0o644))

	overrides := fetch.Overrides{
		Title:  "Override Title",
		Source: "my-source",
		Tags:   []string{"tag1"},
	}
	results := collectHTMLResults(convertHTMLFiles(context.Background(), []string{p}, "", overrides))
	require.Len(t, results, 1)
	assert.NoError(t, results[0].Err)
	assert.Equal(t, "Override Title", results[0].Doc.Frontmatter.Title)
	assert.Equal(t, "my-source", results[0].Doc.Frontmatter.Source)
	assert.Equal(t, []string{"tag1"}, results[0].Doc.Frontmatter.Tags)
}

func TestConvertHTMLStdin_Basic(t *testing.T) {
	// Replace stdin with a pipe.
	r, w, err := os.Pipe()
	require.NoError(t, err)
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old })

	go func() {
		defer w.Close()
		_, _ = w.WriteString(simpleHTML)
	}()

	results := collectHTMLResults(convertHTMLStdin(context.Background(), "https://example.com/stdin", fetch.Overrides{}))
	require.Len(t, results, 1)
	assert.NoError(t, results[0].Err)
	assert.Contains(t, results[0].Doc.Markdown, "Hello from HTML")
	assert.Equal(t, "https://example.com/stdin", results[0].Doc.Frontmatter.URL)
}

func TestConvertHTMLStdin_EmptyURL(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old })

	go func() {
		defer w.Close()
		_, _ = w.WriteString(simpleHTML)
	}()

	results := collectHTMLResults(convertHTMLStdin(context.Background(), "", fetch.Overrides{}))
	require.Len(t, results, 1)
	assert.NoError(t, results[0].Err)
	assert.Equal(t, "", results[0].Doc.Frontmatter.URL)
}
