package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/mbrt/markdowner/internal/fetch"
	"github.com/mbrt/markdowner/internal/output"
	"github.com/spf13/cobra"
)

var urlCmd = &cobra.Command{
	Use:   "url <URL>...",
	Short: "Fetch one or more URLs and convert them to Markdown",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runURL,
}

var urlOutDir string

func init() {
	urlCmd.Flags().StringVar(&urlOutDir, "out-dir", ".", "output directory")
}

func runURL(_ *cobra.Command, args []string) error {
	for _, pageURL := range args {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		doc, err := fetch.URL(ctx, pageURL, downloadImages)
		if err != nil {
			return fmt.Errorf("fetching %q: %w", pageURL, err)
		}
		path, err := output.WriteFile(urlOutDir, doc)
		if err != nil {
			return err
		}
		slog.Info("written", "path", path)
	}
	return nil
}
