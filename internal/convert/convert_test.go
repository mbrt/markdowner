package convert

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromHTML_Basic(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<head><title>Test Article</title></head>
<body>
  <article>
    <h1>Test Article</h1>
    <p>This is a test paragraph with some content.</p>
    <p>Second paragraph here.</p>
  </article>
</body>
</html>`

	got, err := FromHTML(context.Background(), "https://example.com/article", html, false, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, got.Markdown)
	assert.Contains(t, got.Markdown, "test paragraph")
}

func TestFromHTML_Title(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<head><title>My Article Title</title></head>
<body><article><p>Content.</p></article></body>
</html>`

	got, err := FromHTML(context.Background(), "https://example.com/", html, false, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, got.Title)
}

func TestFromHTML_InvalidURL(t *testing.T) {
	_, err := FromHTML(context.Background(), "://bad-url", "<html></html>", false, 0)
	assert.Error(t, err)
}

func TestFromHTML_Headings(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<head><title>Heading Test</title></head>
<body>
  <article>
    <h1>Main Heading</h1>
    <p>Intro paragraph.</p>
    <h2>Sub Heading</h2>
    <p>More text.</p>
  </article>
</body>
</html>`

	got, err := FromHTML(context.Background(), "https://example.com/heading", html, false, 0)
	require.NoError(t, err)
	assert.Contains(t, got.Markdown, "#")
}

func TestFromHTML_SplitMainContent(t *testing.T) {
	// A page whose article is split into many image+caption blocks, with
	// class names that readability treats as boilerplate ("meta"), and
	// lazy-loaded images.
	var blocks strings.Builder
	for _, s := range []string{"alpha", "bravo", "charlie", "delta"} {
		blocks.WriteString(`<div class="row"><div class="gallery"><img data-src="https://example.com/` + s + `.jpg" alt="` + s + ` slide">` +
			`<div class="meta"><div class="meta-inside"><div class="meta-description"><p>Caption ` + s +
			` with enough words to look like narration.</p></div></div></div></div></div>`)
	}
	html := `<!DOCTYPE html><html><head><title>Talk</title></head><body>
<nav><a href="/x">Navigation link</a></nav>
<section role="main">
  <div class="row"><div class="intro"><p>` + "This is the long introduction paragraph of the talk, " +
		"which readability will happily pick as the article body on its own." + `</p>
  <p>Another introduction paragraph with plenty of words to dominate scoring.</p></div></div>` + blocks.String() + `
</section></body></html>`

	got, err := FromHTML(context.Background(), "https://example.com/talk", html, false, 0)
	require.NoError(t, err)
	for _, s := range []string{"alpha", "bravo", "charlie", "delta"} {
		assert.Contains(t, got.Markdown, "Caption "+s)
		assert.Contains(t, got.Markdown, "https://example.com/"+s+".jpg")
	}
	assert.NotContains(t, got.Markdown, "Navigation link")
}
