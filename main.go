// markdowner converts web pages to markdown using the Instapaper API.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/mbrt/markdowner/internal/output"
)

var rootCmd = &cobra.Command{
	Use:               "markdowner",
	Short:             "Convert web pages to Markdown",
	PersistentPreRunE: initWriter,
}

// Persistent flags available to all subcommands
var (
	outDir         string
	outMode        string
	downloadImages bool
)

var writer output.Writer

func init() {
	rootCmd.PersistentFlags().StringVar(&outDir, "out-dir", ".", "output directory")
	rootCmd.PersistentFlags().StringVar(&outMode, "out-mode", string(output.ModeFlat), `output organization mode ("flat" or "week")`)
	rootCmd.PersistentFlags().BoolVar(&downloadImages, "download-images", false, "download external images and rewrite references to local paths")
}

func initWriter(*cobra.Command, []string) error {
	mode := output.Mode(outMode)
	if mode != output.ModeFlat && mode != output.ModeWeek {
		return fmt.Errorf("invalid --out-mode %q: must be %q or %q", outMode, output.ModeFlat, output.ModeWeek)
	}
	writer = output.NewWriter(outDir, mode)
	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		slog.Error("command failed", "err", err)
		os.Exit(1)
	}
}
