package main

import (
	"context"
	"fmt"
	"time"

	"github.com/mbrt/markdowner/internal/fetch"
	"github.com/mbrt/markdowner/internal/output"
	"github.com/spf13/cobra"
)

var urlCmd = &cobra.Command{
	Use:   "url <URL>",
	Short: "Fetch a single URL and convert it to Markdown",
	Args:  cobra.ExactArgs(1),
	RunE:  runURL,
}

var urlOutDir string

func init() {
	urlCmd.Flags().StringVar(&urlOutDir, "out-dir", ".", "output directory")
}

func runURL(cmd *cobra.Command, args []string) error {
	pageURL := args[0]

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	doc, err := fetch.URL(ctx, pageURL)
	if err != nil {
		return fmt.Errorf("fetching %q: %w", pageURL, err)
	}
	if err := output.WriteFile(urlOutDir, doc); err != nil {
		return err
	}
	fmt.Printf("Written: %s/%s.md\n", urlOutDir, doc.Filename)
	return nil
}
