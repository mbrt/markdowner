package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/mbrt/markdowner/internal/instapaper"
	"github.com/mbrt/markdowner/internal/output"
	"github.com/spf13/cobra"
)

var instapaperCmd = &cobra.Command{
	Use:   "instapaper",
	Short: "Fetch Instapaper articles and convert them to Markdown",
	RunE:  runInstapaper,
}

var (
	instapaperOutDir string
	instapaperSince  string
)

func init() {
	instapaperCmd.Flags().StringVar(&instapaperOutDir, "out-dir", ".", "output directory")
	instapaperCmd.Flags().StringVar(&instapaperSince, "since", "", "only fetch articles added after this date (RFC3339 or YYYY-MM-DD)")
}

func runInstapaper(*cobra.Command, []string) error {
	var since time.Time
	if instapaperSince != "" {
		var err error
		since, err = instapaper.ParseDate(instapaperSince)
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

	docs, errs := instapaper.FetchDocs(ctx, client, since)
	for _, err := range errs {
		slog.Warn("fetching article", "err", err)
	}

	written := 0
	for _, doc := range docs {
		path, err := output.WriteFile(instapaperOutDir, doc)
		if err != nil {
			slog.Warn("writing article", "title", doc.Frontmatter.Title, "err", err)
			continue
		}
		slog.Info("written", "path", path)
		written++
	}
	slog.Info("done", "written", written, "out_dir", instapaperOutDir)
	return nil
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("required environment variable is not set", "key", key)
		os.Exit(1)
	}
	return v
}
