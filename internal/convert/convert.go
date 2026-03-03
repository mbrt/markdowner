package convert

import (
	"bytes"
	"fmt"
	nurl "net/url"

	md "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/go-shiori/go-readability"
)

// Contents holds the extracted and converted content of a webpage.
type Contents struct {
	Title    string
	Excerpt  string
	Markdown string
}

// FromHTML extracts the article content from HTML and converts it to Markdown.
func FromHTML(pageURL, html string) (Contents, error) {
	purl, err := nurl.Parse(pageURL)
	if err != nil {
		return Contents{}, fmt.Errorf("parsing URL %q: %w", pageURL, err)
	}
	article, err := readability.FromReader(bytes.NewBufferString(html), purl)
	if err != nil {
		return Contents{}, fmt.Errorf("extracting article: %w", err)
	}
	mdc, err := md.ConvertString(article.Content)
	if err != nil {
		return Contents{}, fmt.Errorf("converting to markdown: %w", err)
	}
	return Contents{
		Title:    article.Title,
		Excerpt:  article.Excerpt,
		Markdown: mdc,
	}, nil
}
