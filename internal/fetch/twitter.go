package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// fxTwitterAPIBase is the base URL for the fxtwitter API, used to fetch tweet
// content. Tests override this to point at a local httptest server.
var fxTwitterAPIBase = "https://api.fxtwitter.com"

// isXArticleURL returns true if rawURL is an X (Twitter) article URL.
// X article URLs have the form: https://x.com/{username}/article/{id}
func isXArticleURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host != "x.com" && host != "www.x.com" {
		return false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	return len(parts) == 3 && parts[1] == "article"
}

// isXTweetURL returns true if rawURL is an X (Twitter) tweet/status URL.
// Tweet URLs have the form: https://x.com/{user}/status/{id} or
// https://x.com/i/status/{id}. Both x.com and twitter.com hosts are accepted.
func isXTweetURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	switch host {
	case "x.com", "www.x.com", "twitter.com", "www.twitter.com":
	default:
		return false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	// Matches /{user}/status/{id} and /i/status/{id}, possibly with
	// trailing segments like /photo/1.
	return len(parts) >= 3 && parts[1] == "status"
}

// tweetID extracts the numeric tweet ID from a parsed tweet URL path.
func tweetID(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 3 || parts[1] != "status" {
		return "", fmt.Errorf("not a tweet URL: %s", rawURL)
	}
	return parts[2], nil
}

// htmlFromXTweet fetches tweet content via the fxtwitter API and returns
// constructed HTML suitable for the readability + markdown conversion pipeline.
func htmlFromXTweet(ctx context.Context, pageURL string) (string, error) {
	id, err := tweetID(pageURL)
	if err != nil {
		return "", err
	}
	apiURL := fxTwitterAPIBase + "/i/status/" + id

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching from fxtwitter: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading fxtwitter response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fxtwitter API returned HTTP %d: %s", resp.StatusCode, body)
	}

	var fxResp fxTweetResponse
	if err := json.Unmarshal(body, &fxResp); err != nil {
		return "", fmt.Errorf("decoding fxtwitter response: %w", err)
	}
	if fxResp.Code != http.StatusOK {
		return "", fmt.Errorf("fxtwitter error %d: %s", fxResp.Code, fxResp.Message)
	}

	// When a tweet embeds an X Article, its text is empty and all content is
	// in the article field. Convert the article content directly to HTML from
	// the data already in the fxtwitter response.
	if fxResp.Tweet.Article != nil {
		return articleToHTML(fxResp.Tweet), nil
	}

	return tweetToHTML(fxResp.Tweet), nil
}

// fxtwitter API response types.
type fxTweetResponse struct {
	Code    int     `json:"code"`
	Message string  `json:"message"`
	Tweet   fxTweet `json:"tweet"`
}

type fxTweet struct {
	Text             string     `json:"text"`
	CreatedTimestamp int64      `json:"created_timestamp"`
	Author           fxAuthor   `json:"author"`
	Media            *fxMedia   `json:"media"`
	Article          *fxArticle `json:"article"`
}

// fxArticle holds the article data returned by the fxtwitter API when a tweet
// embeds an X Article.
type fxArticle struct {
	ID      string     `json:"id"`
	Title   string     `json:"title"`
	Content *fxContent `json:"content"`
}

// fxContent is the Draft.js-like content of an X Article.
type fxContent struct {
	Blocks    []fxBlock       `json:"blocks"`
	EntityMap []fxEntityEntry `json:"entityMap"`
}

// fxBlock is a single Draft.js content block.
type fxBlock struct {
	Type              string          `json:"type"`
	Text              string          `json:"text"`
	InlineStyleRanges []fxStyleRange  `json:"inlineStyleRanges"`
	EntityRanges      []fxEntityRange `json:"entityRanges"`
}

// fxStyleRange describes a range of inline styling within a block's text.
// Offsets are Unicode code-point indices, not byte offsets.
type fxStyleRange struct {
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	Style  string `json:"style"`
}

// fxEntityRange describes a range of text that references an entity by key.
// Offsets are Unicode code-point indices, not byte offsets.
type fxEntityRange struct {
	Offset int `json:"offset"`
	Length int `json:"length"`
	Key    int `json:"key"`
}

// fxEntityEntry is one element of the entityMap array.
type fxEntityEntry struct {
	Key   string        `json:"key"`
	Value fxEntityValue `json:"value"`
}

// fxEntityValue holds the entity type and its associated data.
type fxEntityValue struct {
	Type string       `json:"type"`
	Data fxEntityData `json:"data"`
}

// fxEntityData holds the URL of a LINK entity.
type fxEntityData struct {
	URL string `json:"url"`
}

type fxAuthor struct {
	Name       string `json:"name"`
	ScreenName string `json:"screen_name"`
}

type fxMedia struct {
	All []fxMediaItem `json:"all"`
}

type fxMediaItem struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// tweetToHTML constructs an HTML document from tweet data that go-readability
// can extract a title, author, and body from.
func tweetToHTML(t fxTweet) string {
	author := fmt.Sprintf("%s (@%s)", t.Author.Name, t.Author.ScreenName)
	title := "@" + t.Author.ScreenName + " - " + firstNWords(t.Text, 5)

	// Escape text for HTML, then convert newlines to <br>.
	body := html.EscapeString(t.Text)
	body = strings.ReplaceAll(body, "\n", "<br>\n")

	var media strings.Builder
	if t.Media != nil {
		for _, m := range t.Media.All {
			switch m.Type {
			case "photo":
				fmt.Fprintf(&media, "<img src=\"%s\" alt=\"\" />\n", html.EscapeString(m.URL))
			case "video", "gif":
				fmt.Fprintf(&media, "<p><a href=\"%s\">Video</a></p>\n", html.EscapeString(m.URL))
			}
		}
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta property="og:title" content="%s" />
<meta name="author" content="%s" />
<title>%s</title>
</head>
<body>
<article>
<p>%s</p>
%s</article>
</body>
</html>`,
		html.EscapeString(title),
		html.EscapeString(author),
		html.EscapeString(title),
		body,
		media.String(),
	)
}

// articleToHTML constructs an HTML document from a tweet that embeds an X
// Article. The article content is provided in Draft.js block format by the
// fxtwitter API and converted to HTML for the readability + markdown pipeline.
func articleToHTML(t fxTweet) string {
	article := t.Article
	author := fmt.Sprintf("%s (@%s)", t.Author.Name, t.Author.ScreenName)
	title := article.Title

	// Build a map from entity key to URL for quick lookup.
	entityURLs := map[int]string{}
	if article.Content != nil {
		for _, e := range article.Content.EntityMap {
			if e.Value.Type == "LINK" {
				k, err := strconv.Atoi(e.Key)
				if err == nil {
					entityURLs[k] = e.Value.Data.URL
				}
			}
		}
	}

	var body strings.Builder
	if article.Content != nil {
		renderDraftBlocks(&body, article.Content.Blocks, entityURLs)
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta property="og:title" content="%s" />
<meta name="author" content="%s" />
<title>%s</title>
</head>
<body>
<article>
<h1>%s</h1>
%s</article>
</body>
</html>`,
		html.EscapeString(title),
		html.EscapeString(author),
		html.EscapeString(title),
		html.EscapeString(title),
		body.String(),
	)
}

// renderDraftBlocks converts Draft.js content blocks to HTML, writing into w.
// Consecutive list items are wrapped in a single <ul> or <ol> element.
func renderDraftBlocks(w *strings.Builder, blocks []fxBlock, entityURLs map[int]string) {
	i := 0
	for i < len(blocks) {
		b := blocks[i]
		switch b.Type {
		case "unordered-list-item":
			fmt.Fprint(w, "<ul>\n")
			for i < len(blocks) && blocks[i].Type == "unordered-list-item" {
				fmt.Fprintf(w, "<li>%s</li>\n", draftBlockContent(blocks[i], entityURLs))
				i++
			}
			fmt.Fprint(w, "</ul>\n")
		case "ordered-list-item":
			fmt.Fprint(w, "<ol>\n")
			for i < len(blocks) && blocks[i].Type == "ordered-list-item" {
				fmt.Fprintf(w, "<li>%s</li>\n", draftBlockContent(blocks[i], entityURLs))
				i++
			}
			fmt.Fprint(w, "</ol>\n")
		default:
			tag := draftBlockTag(b.Type)
			content := draftBlockContent(b, entityURLs)
			if content == "" && tag == "p" {
				// Empty paragraph acts as a visual separator.
				fmt.Fprint(w, "<br>\n")
			} else {
				fmt.Fprintf(w, "<%s>%s</%s>\n", tag, content, tag)
			}
			i++
		}
	}
}

// draftBlockTag maps a Draft.js block type to its HTML tag name.
func draftBlockTag(blockType string) string {
	switch blockType {
	case "header-one":
		return "h1"
	case "header-two":
		return "h2"
	case "header-three":
		return "h3"
	case "header-four":
		return "h4"
	case "header-five":
		return "h5"
	case "header-six":
		return "h6"
	case "blockquote":
		return "blockquote"
	case "code-block":
		return "pre"
	default: // "unstyled", etc.
		return "p"
	}
}

// draftBlockContent converts the inline-styled and entity-annotated text of a
// single Draft.js block to an HTML fragment (no wrapping block-level tag).
// Offsets in style/entity ranges are Unicode code-point indices.
func draftBlockContent(b fxBlock, entityURLs map[int]string) string {
	runes := []rune(b.Text)
	n := len(runes)
	if n == 0 {
		return ""
	}

	// Collect all positions where style/entity coverage changes.
	bset := map[int]struct{}{0: {}, n: {}}
	for _, r := range b.InlineStyleRanges {
		s := clampInt(r.Offset, 0, n)
		e := clampInt(r.Offset+r.Length, 0, n)
		bset[s] = struct{}{}
		bset[e] = struct{}{}
	}
	for _, r := range b.EntityRanges {
		s := clampInt(r.Offset, 0, n)
		e := clampInt(r.Offset+r.Length, 0, n)
		bset[s] = struct{}{}
		bset[e] = struct{}{}
	}

	pts := make([]int, 0, len(bset))
	for p := range bset {
		pts = append(pts, p)
	}
	sort.Ints(pts)

	var sb strings.Builder
	for i := 0; i < len(pts)-1; i++ {
		start, end := pts[i], pts[i+1]
		if start >= end {
			continue
		}
		seg := html.EscapeString(string(runes[start:end]))

		// Collect and sort inline styles that fully cover this segment.
		var styles []string
		for _, r := range b.InlineStyleRanges {
			rs := clampInt(r.Offset, 0, n)
			re := clampInt(r.Offset+r.Length, 0, n)
			if rs <= start && end <= re {
				styles = append(styles, r.Style)
			}
		}
		sort.Strings(styles) // deterministic nesting order
		for _, style := range styles {
			seg = applyInlineStyle(style, seg)
		}

		// Apply the first entity link that fully covers this segment.
		for _, r := range b.EntityRanges {
			rs := clampInt(r.Offset, 0, n)
			re := clampInt(r.Offset+r.Length, 0, n)
			if rs <= start && end <= re {
				if url, ok := entityURLs[r.Key]; ok {
					seg = fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(url), seg)
				}
				break
			}
		}

		sb.WriteString(seg)
	}
	return sb.String()
}

// applyInlineStyle wraps inner with the HTML tag for the given Draft.js style name.
func applyInlineStyle(style, inner string) string {
	switch style {
	case "Bold", "BOLD":
		return "<strong>" + inner + "</strong>"
	case "Italic", "ITALIC":
		return "<em>" + inner + "</em>"
	case "Code", "CODE":
		return "<code>" + inner + "</code>"
	case "UNDERLINE":
		return "<u>" + inner + "</u>"
	case "STRIKETHROUGH":
		return "<s>" + inner + "</s>"
	default:
		return inner
	}
}

// clampInt returns v clamped to the range [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
// headless Chrome instance. It waits for the React application to render the
// article content before returning the page source.
func htmlFromXArticle(ctx context.Context, pageURL string) (string, error) {
	// Poll until the article view is present in the DOM, indicating the
	// React application has finished rendering the article content.
	// X uses data-testid attributes (React Native Web), not semantic HTML.
	xWait := chromedp.ActionFunc(func(ctx context.Context) error {
		for {
			var found bool
			if err := chromedp.Evaluate(
				`!!document.querySelector('[data-testid="twitterArticleReadView"]')`,
				&found,
			).Do(ctx); err != nil {
				return err
			}
			if found {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
	})
	// Overwrite the page title with the actual article title so that
	// go-readability extracts it correctly. Both the <title> element and
	// the og:title / twitter:title meta tags are patched because readability
	// prefers Open Graph metadata over the <title> tag.
	xPatchTitle := chromedp.Evaluate(
		`(function(){
			var t = document.querySelector('[data-testid="twitter-article-title"]');
			if (!t) return;
			var title = t.textContent;
			document.title = title;
			var og = document.querySelector('meta[property="og:title"]');
			if (og) og.setAttribute('content', title);
			var tw = document.querySelector('meta[name="twitter:title"]');
			if (tw) tw.setAttribute('content', title);
		})()`,
		nil,
	)
	return runBrowser(ctx, pageURL, xWait, xPatchTitle)
}

// firstNWords returns the first n whitespace-separated words of s, joined by
// spaces. If s has fewer than n words, all words are returned.
func firstNWords(s string, n int) string {
	words := strings.Fields(s)
	if len(words) > n {
		words = words[:n]
	}
	return strings.Join(words, " ")
}
