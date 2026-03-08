package instapaper

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/mbrt/markdowner/internal/convert"
	"github.com/mbrt/markdowner/internal/fetch"
	"github.com/mbrt/markdowner/internal/output"
	"github.com/mbrt/markdowner/internal/timeutil"
)

// ParseDate parses a date string in RFC3339 or YYYY-MM-DD format.
func ParseDate(s string) (time.Time, error) {
	return timeutil.ParseDate(s)
}

// Fetcher bundles options for fetching Instapaper bookmarks.
type Fetcher struct {
	Client         *Client
	Parallel       int
	Timeout        time.Duration
	DownloadImages bool
	MaxImageSize   int64
}

// FetchDocs returns a channel of Result values. Each result is either a
// successfully converted doc or an error. The channel is closed when all
// bookmarks have been processed.
func (f Fetcher) FetchDocs(ctx context.Context, since time.Time) <-chan output.Result {
	ch := make(chan output.Result)

	go func() {
		defer close(ch)

		folders := []string{FolderIDUnread, FolderIDArchive}
		for _, folder := range folders {
			bookmarks, err := listFolder(ctx, f.Client, folder, since)
			if err != nil {
				ch <- output.Result{Err: fmt.Errorf("fetching folder %q: %w", folder, err)}
				continue
			}
			f.processBookmarks(ctx, ch, bookmarks)
		}
	}()

	return ch
}

func (f Fetcher) timeout() time.Duration {
	if f.Timeout > 0 {
		return f.Timeout
	}
	return 10 * time.Second
}

func (f Fetcher) processBookmarks(ctx context.Context, ch chan<- output.Result, bookmarks []Bookmark) {
	parallel := f.Parallel
	if parallel <= 0 {
		parallel = 1
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(parallel)

	for _, b := range bookmarks {
		g.Go(func() error {
			fetchCtx, cancel := context.WithTimeout(ctx, f.timeout())
			defer cancel()
			doc, err := f.bookmarkToDoc(fetchCtx, b)
			res := output.Result{Doc: doc}
			if err != nil {
				res.Err = fmt.Errorf("bookmark %d (%q): %w", b.ID, b.Title, err)
			}
			ch <- res
			return nil
		})
	}

	g.Wait()
}

func (f Fetcher) bookmarkToDoc(ctx context.Context, b Bookmark) (output.Doc, error) {
	html, err := f.Client.GetText(ctx, b.ID)
	if err != nil {
		slog.Warn("GetText failed, fetching from URL", "id", b.ID, "url", b.URL, "err", err)
		html, err = fetch.HTML(ctx, b.URL)
		if err != nil {
			return output.Doc{}, fmt.Errorf("fetching from URL: %w", err)
		}
	}
	contents, err := convert.FromHTML(ctx, b.URL, html, f.DownloadImages, f.MaxImageSize)
	if err != nil {
		return output.Doc{}, fmt.Errorf("converting: %w", err)
	}

	title := b.Title
	if title == "" {
		title = contents.Title
	}
	if title == "" {
		title = b.URL
	}

	var tags []string
	for _, t := range b.Tags {
		tags = append(tags, t.Name)
	}

	body := contents.Markdown
	if body == "" {
		body = b.Description
	}

	date := time.Unix(int64(b.Time), 0).UTC()

	return output.Doc{
		Frontmatter: output.Frontmatter{
			Title:  title,
			Author: contents.Author,
			URL:    b.URL,
			Source: "instapaper",
			Date:   contents.Date,
			Saved:  date,
			Tags:   tags,
		},
		Markdown: body,
		Images:   contents.Images,
	}, nil
}

func listFolder(ctx context.Context, client *Client, folder string, since time.Time) ([]Bookmark, error) {
	params := DefaultBookmarkListParams
	params.Folder = folder

	var all []Bookmark
outer:
	for {
		resp, err := client.ListBookmarks(ctx, params)
		if err != nil {
			return nil, err
		}
		if len(resp.Bookmarks) == 0 {
			break
		}
		for _, b := range resp.Bookmarks {
			t := time.Unix(int64(b.Time), 0)
			if !since.IsZero() && t.Before(since) {
				break outer
			}
			all = append(all, b)
		}
		if len(resp.Bookmarks) < params.Limit {
			break
		}
		params.Skip = append(params.Skip, resp.Bookmarks...)
	}
	return all, nil
}
