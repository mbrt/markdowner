// Package convert provides HTML to Markdown conversion utilities.
package convert

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/go-shiori/go-readability"
	"golang.org/x/net/html"

	"github.com/mbrt/markdowner/internal/images"
)

// Contents holds the extracted and converted content of a webpage.
type Contents struct {
	Title    string
	Author   string
	Date     *time.Time
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
func FromHTML(ctx context.Context, pageURL, html string, downloadImages bool, maxImageSize int64) (Contents, error) {
	purl, err := url.Parse(pageURL)
	if err != nil {
		return Contents{}, fmt.Errorf("parsing URL %q: %w", pageURL, err)
	}
	article, err := readability.FromReader(bytes.NewBufferString(html), purl)
	if err != nil {
		return Contents{}, fmt.Errorf("extracting article: %w", err)
	}

	var (
		mdc  string
		imgs map[string][]byte
	)
	plugins := []converter.Plugin{
		base.NewBasePlugin(),
		commonmark.NewCommonmarkPlugin(),
	}
	if downloadImages {
		imgs = map[string][]byte{}
		plugins = append(plugins, images.NewPlugin(ctx, imgs, maxImageSize))
	}

	content := article.Content
	if main := mainContent(html, article.TextContent); main != "" {
		content = main
	}

	conv := converter.NewConverter(converter.WithPlugins(plugins...))
	mdc, err = conv.ConvertString(content, converter.WithContext(ctx), converter.WithDomain(pageURL))
	if err != nil {
		return Contents{}, fmt.Errorf("converting to markdown: %w", err)
	}

	date := article.ModifiedTime
	if date == nil || date.IsZero() {
		date = article.PublishedTime
	}

	return Contents{
		Title:    article.Title,
		Author:   article.Byline,
		Date:     date,
		Excerpt:  article.Excerpt,
		Markdown: mdc,
		Images:   imgs,
	}, nil
}

// minMainRatio is the fraction of the main container's text that readability
// must have extracted to be trusted.
const minMainRatio = 0.5

// mainContent returns the HTML of the page's declared main-content container
// (<main>, [role=main] or <article>) when readability's extraction (given as
// plain text) covers only a small part of it, or "" when readability's result
// should be used.
//
// Readability scores blocks by class names and text density, and can discard
// large parts of pages that split an article into many blocks, such as
// slide galleries with a caption each. The page's own declaration of its main
// content is a better guide in that case.
func mainContent(pageHTML, extractedText string) string {
	doc, err := html.Parse(strings.NewReader(pageHTML))
	if err != nil {
		return ""
	}
	main := findMain(doc)
	if main == nil {
		return ""
	}
	pruneMain(main)
	if float64(textLen(extractedText)) >= minMainRatio*float64(nodeTextLen(main)) {
		return ""
	}

	var sb strings.Builder
	if err := html.Render(&sb, main); err != nil {
		return ""
	}
	return sb.String()
}

func findMain(n *html.Node) *html.Node {
	if n.Type == html.ElementNode {
		if n.Data == "main" || n.Data == "article" || attr(n, "role") == "main" {
			return n
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if m := findMain(c); m != nil {
			return m
		}
	}
	return nil
}

// pruneMain removes non-content elements and makes lazy-loaded images
// directly loadable.
func pruneMain(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == html.ElementNode {
			switch c.Data {
			case "script", "style", "noscript", "nav", "iframe", "form":
				n.RemoveChild(c)
				c = next
				continue
			case "img":
				if attr(c, "src") == "" {
					if ds := attr(c, "data-src"); ds != "" {
						c.Attr = append(c.Attr, html.Attribute{Key: "src", Val: ds})
					}
				}
			}
		}
		pruneMain(c)
		c = next
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func nodeTextLen(n *html.Node) int {
	if n.Type == html.TextNode {
		return textLen(n.Data)
	}
	total := 0
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		total += nodeTextLen(c)
	}
	return total
}

func textLen(s string) int {
	return len(strings.Join(strings.Fields(s), " "))
}
