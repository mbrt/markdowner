// Package gc implements garbage collection of orphaned image files.
package gc

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	ahocorasick "github.com/petar-dambovaliev/aho-corasick"
)

// Stats holds summary information about a gc run.
type Stats struct {
	DeletedFiles int
	FreedBytes   int64
}

// Run garbage collects orphaned image files under root. An image file is
// considered orphaned when no *.md file in the subtree references it. When
// storeDir is non-empty, image store files that are no longer referenced by any
// remaining symlink in the subtree are also removed. When dryRun is true, no
// files are deleted; only informational log messages are emitted.
func Run(root, storeDir string, dryRun bool) (Stats, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Stats{}, fmt.Errorf("resolving root: %w", err)
	}

	w, err := walkRoot(root)
	if err != nil {
		return Stats{}, err
	}

	paths := make([]string, 0, len(w.imgFiles))
	for p := range w.imgFiles {
		paths = append(paths, p)
	}
	tracker := newImageTracker(paths)

	for _, mdPath := range w.mdPaths {
		content, err := os.ReadFile(mdPath)
		if err != nil {
			return Stats{}, fmt.Errorf("reading %q: %w", mdPath, err)
		}
		tracker.ScanMarkdown(string(content))
	}

	var stats Stats
	for _, path := range tracker.Unvisited() {
		if dryRun {
			slog.Info("[dry-run] would delete", "path", path)
		} else {
			if err := os.Remove(path); err != nil {
				return stats, fmt.Errorf("removing %q: %w", path, err)
			}
			slog.Info("deleted", "path", path)
		}
		stats.DeletedFiles++
		stats.FreedBytes += w.imgFiles[path].size
	}

	if storeDir != "" {
		storeDir, err = filepath.Abs(storeDir)
		if err != nil {
			return stats, fmt.Errorf("resolving store dir: %w", err)
		}
		storeStats, err := cleanStore(root, storeDir, dryRun)
		if err != nil {
			return stats, err
		}
		stats.DeletedFiles += storeStats.DeletedFiles
		stats.FreedBytes += storeStats.FreedBytes
	}

	return stats, nil
}

// imageInfo holds metadata about an image file found in an img/ directory.
type imageInfo struct {
	// size is the number of bytes of actual image data (follows symlinks).
	size int64
}

// imageTracker tracks which image files are referenced by markdown content.
// It is initialized with image file paths and then fed markdown content one
// file at a time via ScanMarkdown. Unreferenced files are returned by
// Unvisited.
//
// Internally it uses an Aho-Corasick automaton to match all basenames in a
// single pass over each markdown file, giving O(content_length) per file
// instead of O(num_images * content_length).
type imageTracker struct {
	byName  map[string][]string // basename -> []abspath
	visited map[string]bool     // abspath -> true
	ac      *ahocorasick.AhoCorasick
}

func newImageTracker(paths []string) *imageTracker {
	byName := make(map[string][]string)
	for _, p := range paths {
		name := filepath.Base(p)
		byName[name] = append(byName[name], p)
	}

	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	builder := ahocorasick.NewAhoCorasickBuilder(ahocorasick.Opts{
		DFA: false,
	})
	ac := builder.Build(names)

	return &imageTracker{
		byName:  byName,
		visited: make(map[string]bool),
		ac:      &ac,
	}
}

// ScanMarkdown marks any tracked image whose basename appears in content as
// visited.
func (t *imageTracker) ScanMarkdown(content string) {
	for _, match := range t.ac.FindAll(content) {
		name := content[match.Start():match.End()]
		for _, p := range t.byName[name] {
			t.visited[p] = true
		}
	}
}

// Unvisited returns the absolute paths of image files that were never matched
// by any ScanMarkdown call.
func (t *imageTracker) Unvisited() []string {
	var result []string
	for _, paths := range t.byName {
		for _, p := range paths {
			if !t.visited[p] {
				result = append(result, p)
			}
		}
	}
	return result
}

// walkResult holds data collected during a single pass over the root tree.
type walkResult struct {
	imgFiles map[string]imageInfo // img/ files keyed by absolute path
	mdPaths  []string             // paths to *.md files
}

// walkRoot performs a single pass over the root tree, collecting both image
// files (inside img/ directories) and markdown file paths.
func walkRoot(root string) (walkResult, error) {
	var res walkResult
	res.imgFiles = make(map[string]imageInfo)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(filepath.Dir(path)) == "img" {
			lstat, err := os.Lstat(path)
			if err != nil {
				return fmt.Errorf("stat %q: %w", path, err)
			}
			size := lstat.Size()
			// For symlinks, report the target's size so FreedBytes
			// reflects the store data that would be freed.
			if lstat.Mode()&os.ModeSymlink != 0 {
				if stat, err := os.Stat(path); err == nil {
					size = stat.Size()
				}
			}
			res.imgFiles[path] = imageInfo{size: size}
		} else if filepath.Ext(path) == ".md" {
			res.mdPaths = append(res.mdPaths, path)
		}
		return nil
	})
	return res, err
}

// cleanStore removes store files that are no longer referenced by any symlink
// inside img/ subdirectories under root.
func cleanStore(root, storeDir string, dryRun bool) (Stats, error) {
	targets, err := collectStoreTargets(root)
	if err != nil {
		return Stats{}, fmt.Errorf("collecting store targets: %w", err)
	}

	var stats Stats
	err = filepath.WalkDir(storeDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if targets[path] {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %q: %w", path, err)
		}
		if dryRun {
			slog.Info("[dry-run] would delete store file", "path", path)
		} else {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("removing store file %q: %w", path, err)
			}
			slog.Info("deleted store file", "path", path)
		}
		stats.DeletedFiles++
		stats.FreedBytes += info.Size()
		return nil
	})
	return stats, err
}

// collectStoreTargets walks img/ subdirectories under root and returns the set
// of absolute paths that symlinks point to.
func collectStoreTargets(root string) (map[string]bool, error) {
	targets := make(map[string]bool)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Base(filepath.Dir(path)) != "img" {
			return nil
		}
		lstat, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("stat %q: %w", path, err)
		}
		if lstat.Mode()&os.ModeSymlink == 0 {
			return nil
		}
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			// Dangling symlink — target already gone; skip.
			return nil
		}
		targets[target] = true
		return nil
	})
	return targets, err
}
