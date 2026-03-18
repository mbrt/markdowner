package fetch

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

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
