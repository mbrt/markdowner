package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/spf13/cobra"

	"github.com/mbrt/markdowner/internal/fetch"
	"github.com/mbrt/markdowner/internal/output"
	"github.com/mbrt/markdowner/internal/timeutil"
)

var urlCmd = &cobra.Command{
	Use:   "url <URL>...",
	Short: "Fetch one or more URLs and convert them to Markdown",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runURL,
}

var (
	urlTimeout time.Duration
	urlTitle   string
	urlAuthor  string
	urlDate    string
	urlSource  string
	urlTags    []string
)

func init() {
	rootCmd.AddCommand(urlCmd)
	urlCmd.Flags().DurationVar(&urlTimeout, "timeout", 2*time.Minute, "per-URL timeout")
	urlCmd.Flags().StringVar(&urlTitle, "title", "", "override article title (single URL only)")
	urlCmd.Flags().StringVar(&urlAuthor, "author", "", "override article author (single URL only)")
	urlCmd.Flags().StringVar(&urlDate, "date", "", "override article date in RFC3339 or YYYY-MM-DD (single URL only)")
	urlCmd.Flags().StringVar(&urlSource, "source", "", "set the source field in the output frontmatter")
	urlCmd.Flags().StringArrayVar(&urlTags, "tags", nil, "set tags on the output (repeatable)")
}

func runURL(_ *cobra.Command, args []string) error {
	if len(args) > 1 && (urlTitle != "" || urlAuthor != "" || urlDate != "") {
		return fmt.Errorf("--title, --author, and --date cannot be used with multiple URLs")
	}

	var parsedDate *time.Time
	if urlDate != "" {
		t, err := timeutil.ParseDate(urlDate)
		if err != nil {
			return fmt.Errorf("parsing --date: %w", err)
		}
		parsedDate = &t
	}

	for _, pageURL := range args {
		ctx, cancel := context.WithTimeout(context.Background(), urlTimeout)
		defer cancel()

		doc, err := fetch.URL(ctx, pageURL, downloadImages)
		if err != nil {
			return fmt.Errorf("fetching %q: %w", pageURL, err)
		}
		applyURLOverrides(&doc, urlTitle, urlAuthor, urlSource, parsedDate, urlTags)
		path, err := writer.WriteDoc(doc)
		if err != nil {
			return err
		}
		slog.Info("written", "path", path)
	}
	return nil
}

// applyURLOverrides applies non-zero flag values over the fetched frontmatter.
func applyURLOverrides(doc *output.Doc, title, author, source string, date *time.Time, tags []string) {
	if title != "" {
		doc.Frontmatter.Title = title
	}
	if author != "" {
		doc.Frontmatter.Author = author
	}
	if source != "" {
		doc.Frontmatter.Source = source
	}
	if date != nil {
		doc.Frontmatter.Date = date
	}
	if len(tags) > 0 {
		doc.Frontmatter.Tags = tags
	}
}
