package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/mbrt/markdowner/internal/convert"
	"github.com/mbrt/markdowner/internal/instapaper"
	"github.com/mbrt/markdowner/internal/output"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	var err error
	switch os.Args[1] {
	case "url":
		err = runURL(os.Args[2:])
	case "instapaper":
		err = runInstapaper(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage:
  markdowner url <URL> --out-dir <dir>
  markdowner instapaper --out-dir <dir> [--since <date>]`)
}

// runURL fetches a single URL and converts it to Markdown.
func runURL(args []string) error {
	fs := flag.NewFlagSet("url", flag.ExitOnError)
	outDir := fs.String("out-dir", ".", "output directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: markdowner url <URL> --out-dir <dir>")
	}
	pageURL := fs.Arg(0)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	html, err := fetchURL(ctx, pageURL)
	if err != nil {
		return fmt.Errorf("fetching %q: %w", pageURL, err)
	}
	contents, err := convert.FromHTML(pageURL, html)
	if err != nil {
		return fmt.Errorf("converting %q: %w", pageURL, err)
	}

	title := contents.Title
	if title == "" {
		title = pageURL
	}
	fm := output.Frontmatter{
		Title: title,
		URL:   pageURL,
		Date:  time.Now().UTC(),
	}
	filename := output.Slugify(title)
	if err := output.WriteFile(*outDir, filename, fm, contents.Markdown); err != nil {
		return err
	}
	fmt.Printf("Written: %s/%s.md\n", *outDir, filename)
	return nil
}

// runInstapaper fetches Instapaper articles and converts them to Markdown.
func runInstapaper(args []string) error {
	fs := flag.NewFlagSet("instapaper", flag.ExitOnError)
	outDir := fs.String("out-dir", ".", "output directory")
	sinceStr := fs.String("since", "", "only fetch articles added after this date (RFC3339 or YYYY-MM-DD)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var since time.Time
	if *sinceStr != "" {
		var err error
		since, err = parseDate(*sinceStr)
		if err != nil {
			return fmt.Errorf("parsing --since: %w", err)
		}
	}

	consumerKey := requireEnv("INSTAPAPER_CONSUMER_KEY")
	consumerSecret := requireEnv("INSTAPAPER_CONSUMER_SECRET")
	username := requireEnv("INSTAPAPER_USERNAME")
	password := requireEnv("INSTAPAPER_PASSWORD")

	ctx := context.Background()
	client := instapaper.NewClient(consumerKey, consumerSecret, username, password)
	if err := client.Authenticate(ctx); err != nil {
		return fmt.Errorf("authenticating with Instapaper: %w", err)
	}

	folders := []string{instapaper.FolderIDUnread, instapaper.FolderIDArchive}
	written := 0

	for _, folder := range folders {
		items, err := listFolder(ctx, client, folder, since)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to fetch folder %q: %v\n", folder, err)
			continue
		}
		for _, b := range items {
			if err := processBookmark(ctx, client, b, *outDir); err != nil {
				fmt.Fprintf(os.Stderr, "warning: skipping bookmark %d (%q): %v\n", b.ID, b.Title, err)
				continue
			}
			written++
		}
	}

	fmt.Printf("Written %d articles to %s\n", written, *outDir)
	return nil
}

// listFolder fetches all bookmarks from a folder, filtered by since.
func listFolder(ctx context.Context, client *instapaper.Client, folder string, since time.Time) ([]instapaper.Bookmark, error) {
	params := instapaper.DefaultBookmarkListParams
	params.Folder = folder

	var all []instapaper.Bookmark
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

// processBookmark fetches text for one bookmark and writes the Markdown file.
func processBookmark(ctx context.Context, client *instapaper.Client, b instapaper.Bookmark, outDir string) error {
	html, err := client.GetText(ctx, b.ID)
	if err != nil {
		return fmt.Errorf("getting text: %w", err)
	}
	contents, err := convert.FromHTML(b.URL, html)
	if err != nil {
		return fmt.Errorf("converting: %w", err)
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

	fm := output.Frontmatter{
		Title: title,
		URL:   b.URL,
		Date:  time.Unix(int64(b.Time), 0).UTC(),
		Tags:  tags,
	}
	// Prefer the converted markdown; fall back to excerpt if empty.
	body := contents.Markdown
	if body == "" {
		body = b.Description
	}

	filename := strconv.Itoa(b.ID) + "-" + output.Slugify(title)
	if err := output.WriteFile(outDir, filename, fm, body); err != nil {
		return err
	}
	fmt.Printf("  Written: %s/%s.md\n", outDir, filename)
	return nil
}

func fetchURL(ctx context.Context, pageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "markdowner/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "error: environment variable %s is required\n", key)
		os.Exit(1)
	}
	return v
}

func parseDate(s string) (time.Time, error) {
	formats := []string{time.RFC3339, "2006-01-02"}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse %q as RFC3339 or YYYY-MM-DD", s)
}
