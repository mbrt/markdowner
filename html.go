package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/mbrt/markdowner/internal/convert"
	"github.com/mbrt/markdowner/internal/fetch"
	"github.com/mbrt/markdowner/internal/output"
	"github.com/mbrt/markdowner/internal/timeutil"
)

var htmlCmd = &cobra.Command{
	Use:   "html [file...]",
	Short: "Convert HTML to Markdown",
	Long: `Convert HTML files to Markdown. With no file arguments, reads HTML from stdin.

The --url flag sets the base URL used to resolve relative links and is written
to the url field in the output frontmatter. For file inputs it defaults to the
file:// path of the input file. For stdin it defaults to empty (relative links
will not resolve).`,
	Run: runHTML,
}

var (
	htmlTitle  string
	htmlAuthor string
	htmlDate   string
	htmlSaved  string
	htmlSource string
	htmlTags   []string
	htmlURL    string
)

func init() {
	rootCmd.AddCommand(htmlCmd)
	htmlCmd.Flags().StringVar(&htmlTitle, "title", "", "override article title (single file only)")
	htmlCmd.Flags().StringVar(&htmlAuthor, "author", "", "override article author (single file only)")
	htmlCmd.Flags().StringVar(&htmlDate, "date", "", "override article date in RFC3339 or YYYY-MM-DD (single file only)")
	htmlCmd.Flags().StringVar(&htmlSaved, "saved", "", "override saved date in RFC3339 or YYYY-MM-DD")
	htmlCmd.Flags().StringVar(&htmlSource, "source", "", "set the source field in the output frontmatter")
	htmlCmd.Flags().StringArrayVar(&htmlTags, "tags", nil, "set tags on the output (repeatable)")
	htmlCmd.Flags().StringVar(&htmlURL, "url", "", "base URL for resolving relative links and the url frontmatter field")
}

func runHTML(cmd *cobra.Command, args []string) {
	if len(args) > 1 && (htmlTitle != "" || htmlAuthor != "" || htmlDate != "") {
		fatalUsage(cmd, fmt.Errorf("--title, --author, and --date cannot be used with multiple files"))
	}

	var (
		parsedDate  *time.Time
		parsedSaved *time.Time
	)
	if htmlDate != "" {
		t, err := timeutil.ParseDate(htmlDate)
		if err != nil {
			fatalUsage(cmd, fmt.Errorf("parsing --date: %w", err))
		}
		parsedDate = &t
	}
	if htmlSaved != "" {
		t, err := timeutil.ParseDate(htmlSaved)
		if err != nil {
			fatalUsage(cmd, fmt.Errorf("parsing --saved: %w", err))
		}
		parsedSaved = &t
	}

	overrides := fetch.Overrides{
		Title:  htmlTitle,
		Author: htmlAuthor,
		Source: htmlSource,
		Date:   parsedDate,
		Saved:  parsedSaved,
		Tags:   htmlTags,
	}

	ctx := context.Background()
	var results <-chan output.Result

	if len(args) == 0 {
		results = convertHTMLStdin(ctx, htmlURL, overrides)
	} else {
		results = convertHTMLFiles(ctx, args, htmlURL, overrides)
	}

	_, failed := writer.WriteDocs(results)
	if failed > 0 {
		fatal(fmt.Errorf("%d file(s) failed to convert or write", failed))
	}
}

// convertHTMLStdin reads HTML from stdin and produces a single result.
func convertHTMLStdin(ctx context.Context, baseURL string, overrides fetch.Overrides) <-chan output.Result {
	ch := make(chan output.Result, 1)
	go func() {
		defer close(ch)
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			ch <- output.Result{Err: fmt.Errorf("reading stdin: %w", err)}
			return
		}
		ch <- convertHTMLBytes(ctx, string(b), baseURL, overrides)
	}()
	return ch
}

// convertHTMLFiles reads HTML from each file and produces one result per file.
func convertHTMLFiles(ctx context.Context, paths []string, urlOverride string, overrides fetch.Overrides) <-chan output.Result {
	ch := make(chan output.Result, len(paths))
	go func() {
		defer close(ch)
		for _, p := range paths {
			b, err := os.ReadFile(p)
			if err != nil {
				ch <- output.Result{Err: fmt.Errorf("reading %q: %w", p, err)}
				continue
			}
			baseURL := urlOverride
			if baseURL == "" {
				abs, err := filepath.Abs(p)
				if err == nil {
					baseURL = "file://" + abs
				}
			}
			ch <- convertHTMLBytes(ctx, string(b), baseURL, overrides)
		}
	}()
	return ch
}

// convertHTMLBytes converts raw HTML to an output.Result, applying overrides.
func convertHTMLBytes(ctx context.Context, html, baseURL string, overrides fetch.Overrides) output.Result {
	contents, err := convert.FromHTML(ctx, baseURL, html, downloadImages, maxImageSizeBytes)
	if err != nil {
		doc := output.Doc{
			Frontmatter: output.Frontmatter{
				URL:   baseURL,
				Saved: time.Now().UTC(),
			},
		}
		overrides.Apply(&doc)
		return output.Result{Doc: doc, Err: fmt.Errorf("converting HTML: %w", err)}
	}

	doc := output.Doc{
		Frontmatter: output.Frontmatter{
			Title:  contents.Title,
			Author: contents.Author,
			URL:    baseURL,
			Date:   contents.Date,
			Saved:  time.Now().UTC(),
		},
		Markdown: contents.Markdown,
		Images:   contents.Images,
	}
	overrides.Apply(&doc)
	return output.Result{Doc: doc}
}
