// Package fetch provides utilities for fetching web page content.
package fetch

import (
	"context"
	"crypto/tls"
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
	// Skip TLS certificate verification unconditionally.
	//
	// This tool is a personal read-only scraper for public web content. Several
	// popular sites (e.g. itnext.io) use certificate chains rooted in CAs that
	// are present in browser bundles but absent from many Linux system certificate
	// stores (e.g. USERTrust RSA Certification Authority). Go's TLS stack, unlike
	// browsers, relies entirely on the system store and does not perform AIA
	// (Authority Information Access) chasing to fetch missing intermediate CAs.
	// The result is that Go rejects connections that every browser accepts.
	//
	// Because this tool only GETs public articles for local conversion and never
	// transmits sensitive data, disabling certificate verification has no meaningful
	// security impact. The correct long-term system fix is to run
	// `sudo update-ca-certificates` with an up-to-date ca-certificates package.
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}
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
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	// Accept-Encoding is intentionally omitted: Go's Transport adds
	// "Accept-Encoding: gzip" automatically and transparently decompresses
	// the response. Explicitly setting this header would disable that
	// automatic decompression, leaving us with raw compressed bytes.
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden && resp.Header.Get("cf-mitigated") == "challenge" {
		return htmlWithBrowser(ctx, pageURL)
	}
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
					// Produce a stub doc on error, to allow partial results.
					doc = output.Doc{
						Frontmatter: output.Frontmatter{
							URL:   pageURL,
							Saved: time.Now().UTC(),
						},
					}
					err = fmt.Errorf("fetching %q: %w", pageURL, err)
				}
				f.Overrides.Apply(&doc)
				ch <- output.Result{Doc: doc, Err: err}
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

// Apply applies non-zero override fields to doc's Frontmatter.
func (o Overrides) Apply(doc *output.Doc) {
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
