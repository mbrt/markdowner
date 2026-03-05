package instapaper

import (
	"context"
	"fmt"
	"time"

	"github.com/mbrt/markdowner/internal/convert"
	"github.com/mbrt/markdowner/internal/output"
)

// ParseDate parses a date string in RFC3339 or YYYY-MM-DD format.
func ParseDate(s string) (time.Time, error) {
	formats := []string{time.RFC3339, "2006-01-02"}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse %q as RFC3339 or YYYY-MM-DD", s)
}

// FetchDocs fetches all bookmarks from the unread and archive folders,
// converts them to Docs, and returns them. Errors for individual items are
// returned alongside the partial results so callers can decide how to handle them.
func FetchDocs(ctx context.Context, client *Client, since time.Time, downloadImages bool) ([]output.Doc, []error) {
	folders := []string{FolderIDUnread, FolderIDArchive}
	var docs []output.Doc
	var errs []error

	for _, folder := range folders {
		bookmarks, err := listFolder(ctx, client, folder, since)
		if err != nil {
			errs = append(errs, fmt.Errorf("fetching folder %q: %w", folder, err))
			continue
		}
		for _, b := range bookmarks {
			doc, err := bookmarkToDoc(ctx, client, b, downloadImages)
			if err != nil {
				errs = append(errs, fmt.Errorf("bookmark %d (%q): %w", b.ID, b.Title, err))
				continue
			}
			docs = append(docs, doc)
		}
	}
	return docs, errs
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

func bookmarkToDoc(ctx context.Context, client *Client, b Bookmark, downloadImages bool) (output.Doc, error) {
	html, err := client.GetText(ctx, b.ID)
	if err != nil {
		return output.Doc{}, fmt.Errorf("getting text: %w", err)
	}
	contents, err := convert.FromHTML(ctx, b.URL, html, downloadImages)
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

	return output.Doc{
		Frontmatter: output.Frontmatter{
			Title:  title,
			Author: contents.Author,
			URL:    b.URL,
			Date:   time.Unix(int64(b.Time), 0).UTC(),
			Tags:   tags,
		},
		Markdown: body,
		Images:   contents.Images,
	}, nil
}
