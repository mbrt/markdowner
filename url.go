package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/mbrt/markdowner/internal/fetch"
	"github.com/mbrt/markdowner/internal/timeutil"
)

var urlCmd = &cobra.Command{
	Use:   "url <URL>...",
	Short: "Fetch one or more URLs and convert them to Markdown",
	Args:  cobra.MinimumNArgs(1),
	Run:   runURL,
}

var (
	urlTimeout time.Duration
	urlTitle   string
	urlAuthor  string
	urlDate    string
	urlSaved   string
	urlSource  string
	urlTags    []string
)

func init() {
	rootCmd.AddCommand(urlCmd)
	urlCmd.Flags().DurationVar(&urlTimeout, "timeout", 2*time.Minute, "per-URL timeout")
	urlCmd.Flags().StringVar(&urlTitle, "title", "", "override article title (single URL only)")
	urlCmd.Flags().StringVar(&urlAuthor, "author", "", "override article author (single URL only)")
	urlCmd.Flags().StringVar(&urlDate, "date", "", "override article date in RFC3339 or YYYY-MM-DD (single URL only)")
	urlCmd.Flags().StringVar(&urlSaved, "saved", "", "override saved date in RFC3339 or YYYY-MM-DD")
	urlCmd.Flags().StringVar(&urlSource, "source", "", "set the source field in the output frontmatter")
	urlCmd.Flags().StringArrayVar(&urlTags, "tags", nil, "set tags on the output (repeatable)")
}

func runURL(cmd *cobra.Command, args []string) {
	if len(args) > 1 && (urlTitle != "" || urlAuthor != "" || urlDate != "") {
		fatalUsage(cmd, fmt.Errorf("--title, --author, and --date cannot be used with multiple URLs"))
	}

	var (
		parsedDate  *time.Time
		parsedSaved *time.Time
	)
	if urlDate != "" {
		t, err := timeutil.ParseDate(urlDate)
		if err != nil {
			fatalUsage(cmd, fmt.Errorf("parsing --date: %w", err))
		}
		parsedDate = &t
	}
	if urlSaved != "" {
		t, err := timeutil.ParseDate(urlSaved)
		if err != nil {
			fatalUsage(cmd, fmt.Errorf("parsing --saved: %w", err))
		}
		parsedSaved = &t
	}

	fetcher := fetch.Fetcher{
		Parallel:       parallel,
		Timeout:        urlTimeout,
		DownloadImages: downloadImages,
		MaxImageSize:   maxImageSizeBytes,
		Overrides: fetch.Overrides{
			Title:  urlTitle,
			Author: urlAuthor,
			Source: urlSource,
			Date:   parsedDate,
			Saved:  parsedSaved,
			Tags:   urlTags,
		},
	}
	_, failed := writer.WriteDocs(fetcher.FetchURLs(context.Background(), args))
	if failed > 0 {
		fatal(fmt.Errorf("%d article(s) failed to fetch or write", failed))
	}
}
