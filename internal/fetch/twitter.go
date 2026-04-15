package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
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

	return tweetToHTML(fxResp.Tweet), nil
}

// fxtwitter API response types.
type fxTweetResponse struct {
	Code    int     `json:"code"`
	Message string  `json:"message"`
	Tweet   fxTweet `json:"tweet"`
}

type fxTweet struct {
	Text             string   `json:"text"`
	CreatedTimestamp int64    `json:"created_timestamp"`
	Author           fxAuthor `json:"author"`
	Media            *fxMedia `json:"media"`
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

// htmlFromXArticle fetches the HTML of an X (Twitter) article URL by running a
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
