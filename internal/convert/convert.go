// Package convert provides HTML to Markdown conversion utilities.
package convert

import (
	"bytes"
	"context"
	"fmt"
	nurl "net/url"

	md "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/go-shiori/go-readability"
	"github.com/mbrt/markdowner/internal/images"
)

// Contents holds the extracted and converted content of a webpage.
type Contents struct {
	Title    string
	Author   string
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

	var mdc string
	var imgs map[string][]byte
	if downloadImages {
		imgs = map[string][]byte{}
		imgPlugin := images.NewPlugin(ctx, imgs)
		conv := converter.NewConverter(
			converter.WithPlugins(
				base.NewBasePlugin(),
				commonmark.NewCommonmarkPlugin(),
				imgPlugin,
			),
		)
		mdc, err = conv.ConvertString(article.Content, converter.WithContext(ctx))
		if err != nil {
			return Contents{}, fmt.Errorf("converting to markdown: %w", err)
		}
	} else {
		mdc, err = md.ConvertString(article.Content)
		if err != nil {
			return Contents{}, fmt.Errorf("converting to markdown: %w", err)
		}
	}

	return Contents{
		Title:    article.Title,
		Author:   article.Byline,
		Excerpt:  article.Excerpt,
		Markdown: mdc,
		Images:   imgs,
	}, nil
}
