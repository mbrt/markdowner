// Package convert provides HTML to Markdown conversion utilities.
package convert

import (
	"bytes"
	"context"
	"fmt"
	nurl "net/url"

	md "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/go-shiori/go-readability"
	"github.com/mbrt/markdowner/internal/images"
)

// Contents holds the extracted and converted content of a webpage.
type Contents struct {
	Title    string
	Excerpt  string
	Markdown string
	// Images maps relative local paths ("img/<sha>.<ext>") to raw image bytes.
	// Populated only when FromHTML is called with downloadImages = true.
	Images map[string][]byte
}

// FromHTML extracts the article content from HTML and converts it to Markdown.
// When downloadImages is true, external images are downloaded and their
// references in the Markdown are rewritten to local relative paths; the blobs
// are returned in Contents.Images.
func FromHTML(ctx context.Context, pageURL, html string, downloadImages bool) (Contents, error) {
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

	var imgs map[string][]byte
	if downloadImages {
		var rewritten string
		rewritten, imgs, err = images.Rewrite(ctx, mdc)
		if err != nil {
			return Contents{}, fmt.Errorf("rewriting images: %w", err)
		}
		mdc = rewritten
	}

	return Contents{
		Title:    article.Title,
		Excerpt:  article.Excerpt,
		Markdown: mdc,
		Images:   imgs,
	}, nil
}
