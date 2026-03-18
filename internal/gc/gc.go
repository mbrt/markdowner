// Package gc implements garbage collection of orphaned image files.
package gc

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
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

	referenced, err := collectReferencedImages(root)
	if err != nil {
		return Stats{}, err
	}

	existing, err := collectImgFiles(root)
	if err != nil {
		return Stats{}, err
	}

	var stats Stats
	for path, info := range existing {
		if referenced[path] {
			continue
		}
		if dryRun {
			slog.Info("[dry-run] would delete", "path", path)
		} else {
			if err := os.Remove(path); err != nil {
				return stats, fmt.Errorf("removing %q: %w", path, err)
			}
			slog.Info("deleted", "path", path)
		}
		stats.DeletedFiles++
		stats.FreedBytes += info.size
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

// imageRefRe matches Markdown image links of the form ![alt](img/filename).
var imageRefRe = regexp.MustCompile(`!\[[^\]]*\]\((img/[^)\s]+)`)

// collectReferencedImages walks all *.md files under root and returns the set
// of absolute paths of image files they reference via img/ links.
func collectReferencedImages(root string) (map[string]bool, error) {
	refs := make(map[string]bool)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %q: %w", path, err)
		}
		dir := filepath.Dir(path)
		for _, imgPath := range extractImageRefs(string(content)) {
			refs[filepath.Join(dir, imgPath)] = true
		}
		return nil
	})
	return refs, err
}

// extractImageRefs returns the img/-relative paths referenced in md.
func extractImageRefs(md string) []string {
	matches := imageRefRe.FindAllStringSubmatch(md, -1)
	var refs []string
	for _, m := range matches {
		refs = append(refs, m[1])
	}
	return refs
}

// collectImgFiles walks root and returns all files inside img/ subdirectories,
// keyed by their absolute path. For symlinks, size reflects the target file.
func collectImgFiles(root string) (map[string]imageInfo, error) {
	files := make(map[string]imageInfo)
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
		size := lstat.Size()
		// For symlinks, report the target's size so FreedBytes reflects
		// the store data that would be freed when the store is cleaned.
		if lstat.Mode()&os.ModeSymlink != 0 {
			if stat, err := os.Stat(path); err == nil {
				size = stat.Size()
			}
		}
		files[path] = imageInfo{size: size}
		return nil
	})
	return files, err
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
