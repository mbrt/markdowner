package main

import (
	"fmt"
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
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

