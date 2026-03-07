package images

import (
	"bytes"
	"context"
	"encoding/hex"
	"log/slog"
	"strings"

	"github.com/JohannesKaufmann/dom"
	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"golang.org/x/crypto/sha3"
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
	ctx          context.Context
	results      map[string][]byte
	maxImageSize int64
	cache        map[string]string // imgURL → local path, "" means download failed
}

// NewPlugin returns a Plugin that downloads images using ctx and accumulates
// the raw image bytes in results (keyed by local relative path).
// When maxImageSize > 0, images exceeding that size are re-encoded as JPEG
// at a quality level that fits within the limit.
func NewPlugin(ctx context.Context, results map[string][]byte, maxImageSize int64) *Plugin {
	return &Plugin{
		ctx:          ctx,
		results:      results,
		maxImageSize: maxImageSize,
		cache:        map[string]string{},
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

	var localPath string
	switch {
	case strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://"):
		localPath = p.localPathFor(src)
	case strings.HasPrefix(src, "data:"):
		localPath = p.localPathForEmbedded(src)
	default:
		return converter.RenderTryNext
	}
	if localPath == "" {
		// Download/decode failed; fall back to normal rendering.
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

	if p.maxImageSize > 0 {
		compressed, newExt, err := compressToJPEG(data, p.maxImageSize)
		if err != nil {
			slog.Warn("compressing image", "url", imgURL, "err", err)
		} else {
			data = compressed
			if newExt != "" {
				lp = replaceExt(lp, newExt)
			}
		}
	}

	p.cache[imgURL] = lp
	p.results[lp] = data
	return lp
}

// localPathForEmbedded decodes a data URI and saves the image locally.
// Returns "" if the URI is malformed or unsupported (logs a warning once).
func (p *Plugin) localPathForEmbedded(src string) string {
	if lp, seen := p.cache[src]; seen {
		return lp
	}

	ext, data, err := decodeDataURI(src)
	if err != nil {
		slog.Warn("decoding embedded image", "err", err)
		p.cache[src] = ""
		return ""
	}

	if p.maxImageSize > 0 {
		compressed, newExt, cerr := compressToJPEG(data, p.maxImageSize)
		if cerr != nil {
			slog.Warn("compressing embedded image", "err", cerr)
		} else {
			data = compressed
			if newExt != "" {
				ext = newExt
			}
		}
	}

	h := sha3.NewShake128()
	h.Write(data)
	hashBytes := make([]byte, 16)
	h.Read(hashBytes)
	lp := "img/" + hex.EncodeToString(hashBytes) + ext

	p.cache[src] = lp
	p.results[lp] = data
	return lp
}

// replaceExt replaces the file extension in path with newExt.
func replaceExt(path, newExt string) string {
	ext := strings.LastIndex(path, ".")
	if ext < 0 {
		return path + newExt
	}
	return path[:ext] + newExt
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
