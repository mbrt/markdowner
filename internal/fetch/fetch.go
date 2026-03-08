// Package fetch provides utilities for fetching web page content.
package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/mbrt/markdowner/internal/convert"
	"github.com/mbrt/markdowner/internal/output"
)

const (
	// Maximize compatibility with various sites by using a common desktop browser user agent.
	userAgent       = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36"
	defaultTimeout  = 10 * time.Second
	retryBackoff    = time.Second
	maxRetryBackoff = 20 * time.Second
)

// Client configures HTTP fetching behavior. The zero value is ready to use
// with sensible defaults.
type Client struct {
	HTTPClient      *http.Client
	RetryBackoff    time.Duration
	MaxRetryBackoff time.Duration
}

func (c Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func (c Client) initialBackoff() time.Duration {
	if c.RetryBackoff > 0 {
		return c.RetryBackoff
	}
	return retryBackoff
}

func (c Client) maxBackoff() time.Duration {
	if c.MaxRetryBackoff > 0 {
		return c.MaxRetryBackoff
	}
	return maxRetryBackoff
}

// HTML fetches the raw HTML content of the page at pageURL.
// It retries with exponential backoff until the context is done.
func (c Client) HTML(ctx context.Context, pageURL string) (string, error) {
	backoff := c.initialBackoff()
	maxBO := c.maxBackoff()

	var lastErr error
	for {
		html, err := c.htmlOnce(ctx, pageURL)
		if err == nil {
			return html, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return "", lastErr
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBO {
			backoff = maxBO
		}
	}
}

func (c Client) htmlOnce(ctx context.Context, pageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// URL fetches the page at pageURL, converts it to Markdown, and returns a Doc.
// When downloadImages is true, external images are downloaded and stored in
// Doc.Images.
func (c Client) URL(ctx context.Context, pageURL string, downloadImages bool, maxImageSize int64) (output.Doc, error) {
	html, err := c.HTML(ctx, pageURL)
	if err != nil {
		return output.Doc{}, err
	}

	contents, err := convert.FromHTML(ctx, pageURL, html, downloadImages, maxImageSize)
	if err != nil {
		return output.Doc{}, fmt.Errorf("converting %q: %w", pageURL, err)
	}

	return output.Doc{
		Frontmatter: output.Frontmatter{
			Title:  contents.Title,
			Author: contents.Author,
			URL:    pageURL,
			Date:   contents.Date,
			Saved:  time.Now().UTC(),
		},
		Markdown: contents.Markdown,
		Images:   contents.Images,
	}, nil
}

// HTML fetches the raw HTML content of the page at pageURL.
// It retries with exponential backoff until the context is done.
func HTML(ctx context.Context, pageURL string) (string, error) {
	return Client{}.HTML(ctx, pageURL)
}

// URL fetches the page at pageURL, converts it to Markdown, and returns a Doc.
// When downloadImages is true, external images are downloaded and stored in
// Doc.Images.
func URL(ctx context.Context, pageURL string, downloadImages bool, maxImageSize int64) (output.Doc, error) {
	return Client{}.URL(ctx, pageURL, downloadImages, maxImageSize)
}

// Overrides holds optional frontmatter overrides applied to every fetched doc.
type Overrides struct {
	Title  string
	Author string
	Source string
	Date   *time.Time
	Saved  *time.Time
	Tags   []string
}

// Fetcher fetches URLs in parallel and produces results on a channel.
type Fetcher struct {
	Client         Client
	Parallel       int
	Timeout        time.Duration
	DownloadImages bool
	MaxImageSize   int64
	Overrides      Overrides
}

// FetchURLs fetches the given URLs in parallel and returns a channel of
// results. The channel is closed when all URLs have been processed.
func (f Fetcher) FetchURLs(ctx context.Context, urls []string) <-chan output.Result {
	f.setDefaults()

	ch := make(chan output.Result)
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(f.Parallel)

	go func() {
		defer close(ch)
		for _, pageURL := range urls {
			g.Go(func() error {
				fetchCtx, cancel := context.WithTimeout(ctx, f.Timeout)
				defer cancel()

				doc, err := f.Client.URL(fetchCtx, pageURL, f.DownloadImages, f.MaxImageSize)
				if err != nil {
					ch <- output.Result{Err: fmt.Errorf("fetching %q: %w", pageURL, err)}
					return nil
				}
				f.Overrides.apply(&doc)
				ch <- output.Result{Doc: doc}
				return nil
			})
		}
		_ = g.Wait()
	}()

	return ch
}

func (f *Fetcher) setDefaults() {
	if f.Parallel <= 0 {
		f.Parallel = 1
	}
	if f.Timeout <= 0 {
		f.Timeout = defaultTimeout
	}
}

func (o Overrides) apply(doc *output.Doc) {
	if o.Title != "" {
		doc.Frontmatter.Title = o.Title
	}
	if o.Author != "" {
		doc.Frontmatter.Author = o.Author
	}
	if o.Source != "" {
		doc.Frontmatter.Source = o.Source
	}
	if o.Date != nil {
		doc.Frontmatter.Date = o.Date
	}
	if o.Saved != nil {
		doc.Frontmatter.Saved = *o.Saved
	}
	if len(o.Tags) > 0 {
		doc.Frontmatter.Tags = o.Tags
	}
}
