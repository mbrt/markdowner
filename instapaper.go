package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/mbrt/markdowner/internal/instapaper"
)

var instapaperCmd = &cobra.Command{
	Use:   "instapaper",
	Short: "Fetch Instapaper articles and convert them to Markdown",
	RunE:  runInstapaper,
}

var instapaperSince string

func init() {
	rootCmd.AddCommand(instapaperCmd)
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

	fetcher := instapaper.Fetcher{
		Client:         client,
		Parallel:       parallel,
		DownloadImages: downloadImages,
	}
	written, failed := writer.WriteDocs(fetcher.FetchDocs(ctx, since))
	slog.Info("done", "written", written, "out_dir", outDir)
	if failed > 0 {
		return fmt.Errorf("%d article(s) failed to fetch or write", failed)
	}
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
