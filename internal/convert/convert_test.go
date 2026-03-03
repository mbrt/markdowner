package convert

import (
	"strings"
	"testing"
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
	if err != nil {
		t.Fatalf("FromHTML() error = %v", err)
	}
	if got.Markdown == "" {
		t.Error("expected non-empty Markdown")
	}
	if !strings.Contains(got.Markdown, "test paragraph") {
		t.Errorf("Markdown does not contain expected content; got:\n%s", got.Markdown)
	}
}

func TestFromHTML_Title(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<head><title>My Article Title</title></head>
<body><article><p>Content.</p></article></body>
</html>`

	got, err := FromHTML("https://example.com/", html)
	if err != nil {
		t.Fatalf("FromHTML() error = %v", err)
	}
	if got.Title == "" {
		t.Error("expected non-empty Title")
	}
}

func TestFromHTML_InvalidURL(t *testing.T) {
	_, err := FromHTML("://bad-url", "<html></html>")
	if err == nil {
		t.Error("expected error for invalid URL, got nil")
	}
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
	if err != nil {
		t.Fatalf("FromHTML() error = %v", err)
	}
	// html-to-markdown should produce ## or # markers
	if !strings.Contains(got.Markdown, "#") {
		t.Errorf("expected heading markers in Markdown; got:\n%s", got.Markdown)
	}
}
