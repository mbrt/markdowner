package main

import (
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/mbrt/markdowner/internal/gc"
)

var gcCmd = &cobra.Command{
	Use:   "gc [root-dir]",
	Short: "Remove image files not referenced by any Markdown file",
	Long: `gc walks the given root directory (or the current directory if omitted) and
removes image files inside img/ subdirectories that are not referenced by any
Markdown file in the subtree.

When --image-store is set, image store files that are no longer symlinked from
any remaining img/ entry are also removed.

Use --dry-run to see what would be deleted without making any changes.`,
	Args: cobra.MaximumNArgs(1),
	// Override the root's PersistentPreRunE so initWriter is not called;
	// gc does not write articles and does not need the output flags.
	PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
	Run:               runGC,
}

var gcDryRun bool

func init() {
	rootCmd.AddCommand(gcCmd)
	gcCmd.Flags().BoolVar(&gcDryRun, "dry-run", false, "report deletions without performing them")
}

func runGC(_ *cobra.Command, args []string) {
	root := "."
	if len(args) > 0 {
		root = args[0]
	}

	stats, err := gc.Run(root, imageStoreDir, gcDryRun)
	if err != nil {
		fatal(err)
	}

	slog.Info("gc complete",
		"deleted_files", stats.DeletedFiles,
		"freed_bytes", stats.FreedBytes,
		"dry_run", gcDryRun,
	)
}
