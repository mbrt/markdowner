// markdowner converts web pages to markdown using the Instapaper API.
package main

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "markdowner",
	Short: "Convert web pages to Markdown",
}

// Persistent flags available to all subcommands
var (
	outDir         string
	downloadImages bool
)

func init() {
	rootCmd.PersistentFlags().StringVar(&outDir, "out-dir", ".", "output directory")
	rootCmd.PersistentFlags().BoolVar(&downloadImages, "download-images", false, "download external images and rewrite references to local paths")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		slog.Error("command failed", "err", err)
		os.Exit(1)
	}
}
