// markdowner converts web pages to markdown using the Instapaper API.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/mbrt/markdowner/internal/images"
	"github.com/mbrt/markdowner/internal/output"
)

var rootCmd = &cobra.Command{
	Use:               "markdowner",
	Short:             "Convert web pages to Markdown",
	PersistentPreRunE: initWriter,
}

// Persistent flags available to all subcommands
var (
	outDir            string
	outMode           string
	downloadImages    bool
	imageStoreDir     string
	parallel          int
	maxImageSize      string
	maxImageSizeBytes int64
	ignoreFailures    bool
	overwriteFlag     string
	timeout           time.Duration
)

var writer output.Writer

func init() {
	rootCmd.PersistentFlags().StringVar(&outDir, "out-dir", ".", "output directory (ignored in stdout mode)")
	rootCmd.PersistentFlags().StringVar(&outMode, "out-mode", string(output.ModeStdout), `output mode: "stdout" (default), "flat" (all files in one dir), or "week" (subdirs by ISO week)`)
	rootCmd.PersistentFlags().BoolVar(&downloadImages, "download-images", false, "download external images and rewrite references to local paths")
	rootCmd.PersistentFlags().StringVar(&imageStoreDir, "image-store", "", "shared image store directory to deduplicate downloaded images")
	rootCmd.PersistentFlags().IntVarP(&parallel, "parallel", "j", 4, "number of parallel fetches")
	rootCmd.PersistentFlags().StringVar(&maxImageSize, "max-image-size", "", `max size for downloaded images (e.g. 500KB, 2MB); oversized images are converted to JPEG`)
	rootCmd.PersistentFlags().BoolVar(&ignoreFailures, "ignore-failures", false, "on fetch failure, write a stub file with frontmatter only instead of skipping")
	rootCmd.PersistentFlags().DurationVar(&timeout, "timeout", 10*time.Second, "per-item timeout")
	rootCmd.PersistentFlags().StringVar(&overwriteFlag, "overwrite", string(output.OverwriteNone), `overwrite behavior when output file exists: "all" (markdown+images), "md" (markdown only), "empty" (markdown only if body is empty), "none" (skip)`)
	// When --overwrite is given without a value, default to "all".
	rootCmd.PersistentFlags().Lookup("overwrite").NoOptDefVal = string(output.OverwriteAll)
}

func initWriter(cmd *cobra.Command, _ []string) error {
	mode := output.Mode(outMode)

	if mode == output.ModeStdout {
		flags := cmd.Root().PersistentFlags()
		for _, name := range []string{"out-dir", "overwrite", "image-store", "download-images", "max-image-size"} {
			if flags.Changed(name) {
				return fmt.Errorf("--%s cannot be used with --out-mode stdout", name)
			}
		}
		writer = output.NewWithWriter(cmd.OutOrStdout())
		writer.IgnoreFailures = ignoreFailures
		return nil
	}

	if mode != output.ModeFlat && mode != output.ModeWeek {
		return fmt.Errorf("invalid --out-mode %q: must be one of %q, %q, or %q", outMode, output.ModeStdout, output.ModeFlat, output.ModeWeek)
	}
	overwrite, err := output.ParseOverwriteMode(overwriteFlag)
	if err != nil {
		return err
	}
	writer = output.NewWithDir(outDir, mode, imageStoreDir)
	writer.IgnoreFailures = ignoreFailures
	writer.Overwrite = overwrite

	if maxImageSize != "" {
		n, err := images.ParseSize(maxImageSize)
		if err != nil {
			return fmt.Errorf("invalid --max-image-size: %w", err)
		}
		maxImageSizeBytes = n
	}
	return nil
}

// fatal logs a runtime error and exits. Use for errors that are not caused by
// invalid flags or arguments (those should use fatalUsage instead).
func fatal(err error) {
	slog.Error("command failed", "err", err)
	os.Exit(1)
}

// fatalUsage prints an error and the command usage, then exits. Use for errors
// caused by invalid flags or arguments.
func fatalUsage(cmd *cobra.Command, err error) {
	fmt.Fprintln(os.Stderr, "Error:", err)
	_ = cmd.Usage()
	os.Exit(1)
}

func main() {
	// cobra already prints the error and usage for flag/arg parse errors;
	// we just need to exit.
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
