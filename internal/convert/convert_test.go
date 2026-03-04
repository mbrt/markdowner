package convert

import (
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

	got, err := FromHTML("https://example.com/article", html)
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

	got, err := FromHTML("https://example.com/", html)
	require.NoError(t, err)
	assert.NotEmpty(t, got.Title)
}

func TestFromHTML_InvalidURL(t *testing.T) {
	_, err := FromHTML("://bad-url", "<html></html>")
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

	got, err := FromHTML("https://example.com/heading", html)
	require.NoError(t, err)
	assert.Contains(t, got.Markdown, "#")
}
