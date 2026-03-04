package images

import (
	"bytes"
	"context"
	"log/slog"
	"strings"

	"github.com/JohannesKaufmann/dom"
	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"golang.org/x/net/html"
)

// Plugin is a html-to-markdown converter plugin that intercepts <img> nodes
// with absolute http/https URLs, downloads the images, and rewrites the
// markdown reference to a local relative path of the form
// "img/<sha3-128hex>.<ext>".
//
// Non-http/https sources (relative paths, data URIs, …) are left for the
// default commonmark renderer to handle.
type Plugin struct {
	ctx     context.Context
	results map[string][]byte
	cache   map[string]string // imgURL → local path, "" means download failed
}

// NewPlugin returns a Plugin that downloads images using ctx and accumulates
// the raw image bytes in results (keyed by local relative path).
func NewPlugin(ctx context.Context, results map[string][]byte) *Plugin {
	return &Plugin{
		ctx:     ctx,
		results: results,
		cache:   map[string]string{},
	}
}

// Name returns the plugin name, satisfying the converter.Plugin interface.
func (p *Plugin) Name() string { return "image-downloader" }

// Init registers the image-download renderer for <img> nodes at PriorityEarly,
// satisfying the converter.Plugin interface.
func (p *Plugin) Init(conv *converter.Converter) error {
	conv.Register.RendererFor("img", converter.TagTypeInline, p.render, converter.PriorityEarly)
	return nil
}

func (p *Plugin) render(ctx converter.Context, w converter.Writer, n *html.Node) converter.RenderStatus {
	src := dom.GetAttributeOr(n, "src", "")
	src = strings.TrimSpace(src)
	if src == "" {
		return converter.RenderTryNext
	}

	// Resolve relative URLs to absolute before checking scheme.
	src = ctx.AssembleAbsoluteURL(ctx, "img", src)

	if !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
		return converter.RenderTryNext
	}

	localPath := p.localPathFor(src)
	if localPath == "" {
		// Download failed previously or now; fall back to normal rendering.
		return converter.RenderTryNext
	}

	alt := dom.GetAttributeOr(n, "alt", "")
	alt = strings.ReplaceAll(alt, "\n", " ")
	alt = escapeAlt(alt)

	title := dom.GetAttributeOr(n, "title", "")
	title = strings.ReplaceAll(title, "\n", " ")

	w.WriteRune('!')
	w.WriteRune('[')
	w.WriteString(alt)
	w.WriteRune(']')
	w.WriteRune('(')
	w.WriteString(localPath)
	if title != "" {
		w.WriteRune(' ')
		w.WriteRune('"')
		w.WriteString(title)
		w.WriteRune('"')
	}
	w.WriteRune(')')

	return converter.RenderSuccess
}

// localPathFor returns the local path for imgURL, downloading if necessary.
// Returns "" if the download has failed (logs a warning once).
func (p *Plugin) localPathFor(imgURL string) string {
	if lp, seen := p.cache[imgURL]; seen {
		return lp
	}

	lp, data, err := downloadImage(p.ctx, imgURL)
	if err != nil {
		slog.Warn("downloading image", "url", imgURL, "err", err)
		p.cache[imgURL] = ""
		return ""
	}

	p.cache[imgURL] = lp
	p.results[lp] = data
	return lp
}

// escapeAlt escapes '[' and ']' in alt text so they don't break markdown link
// syntax, matching the behaviour of the commonmark plugin.
func escapeAlt(alt string) string {
	b := []byte(alt)
	var buf bytes.Buffer
	for i := range b {
		if b[i] == '[' || b[i] == ']' {
			if i == 0 || b[i-1] != '\\' {
				buf.WriteRune('\\')
			}
		}
		buf.WriteByte(b[i])
	}
	return buf.String()
}
