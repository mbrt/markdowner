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

func init() {
	rootCmd.AddCommand(urlCmd)
	rootCmd.AddCommand(instapaperCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		slog.Error("command failed", "err", err)
		os.Exit(1)
	}
}
