package gc

import (
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFile creates a file at path with the given content, creating parent dirs.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// symlink creates a symlink at linkPath pointing to target.
func symlink(t *testing.T, target, linkPath string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(linkPath), 0o755))
	require.NoError(t, os.Symlink(target, linkPath))
}

func TestImageTracker(t *testing.T) {
	tests := []struct {
		name      string
		images    []string
		markdown  string
		wantVisit []string
	}{
		{
			name:      "no images tracked",
			images:    nil,
			markdown:  "# Hello\n\nJust text.",
			wantVisit: nil,
		},
		{
			name:      "single image found",
			images:    []string{"/root/article/img/abc123.png"},
			markdown:  "![alt](img/abc123.png)",
			wantVisit: nil,
		},
		{
			name:      "image not referenced",
			images:    []string{"/root/article/img/orphan.png"},
			markdown:  "# No images here.",
			wantVisit: []string{"/root/article/img/orphan.png"},
		},
		{
			name:      "linked image with escaped brackets",
			images:    []string{"/root/article/img/abc123.png"},
			markdown:  `[![alt\[dot\]text](img/abc123.png)](https://example.com)`,
			wantVisit: nil,
		},
		{
			name:      "mixed referenced and orphaned",
			images:    []string{"/root/article/img/used.png", "/root/article/img/unused.png"},
			markdown:  "![a](img/used.png)",
			wantVisit: []string{"/root/article/img/unused.png"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tracker := newImageTracker(tc.images)
			tracker.ScanMarkdown(tc.markdown)
			assert.ElementsMatch(t, tc.wantVisit, tracker.Unvisited())
		})
	}
}

func TestRun_NoImages(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "article", "index.md"), "# Hello\n\nNo images here.\n")

	stats, err := Run(root, "", false)
	require.NoError(t, err)
	assert.Equal(t, Stats{}, stats)
}

func TestRun_AllImagesReferenced(t *testing.T) {
	root := t.TempDir()
	imgPath := filepath.Join(root, "article", "img", "abc.png")
	writeFile(t, filepath.Join(root, "article", "index.md"),
		"# Title\n\n![photo](img/abc.png)\n")
	writeFile(t, imgPath, "fake-image-data")

	stats, err := Run(root, "", false)
	require.NoError(t, err)
	assert.Equal(t, Stats{}, stats)
	_, err = os.Stat(imgPath)
	assert.NoError(t, err, "referenced image should not be deleted")
}

func TestRun_OrphanedImageDeleted(t *testing.T) {
	root := t.TempDir()
	imgPath := filepath.Join(root, "article", "img", "orphan.png")
	writeFile(t, filepath.Join(root, "article", "index.md"), "# No images here.\n")
	writeFile(t, imgPath, "fake-image-data")

	stats, err := Run(root, "", false)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.DeletedFiles)
	assert.Greater(t, stats.FreedBytes, int64(0))
	_, err = os.Stat(imgPath)
	assert.ErrorIs(t, err, os.ErrNotExist, "orphaned image should be deleted")
}

func TestRun_OrphanedAndReferenced(t *testing.T) {
	root := t.TempDir()
	referenced := filepath.Join(root, "article", "img", "used.png")
	orphaned := filepath.Join(root, "article", "img", "unused.png")
	writeFile(t, filepath.Join(root, "article", "index.md"),
		"# Title\n\n![photo](img/used.png)\n")
	writeFile(t, referenced, "used-data")
	writeFile(t, orphaned, "unused-data")

	stats, err := Run(root, "", false)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.DeletedFiles)
	_, err = os.Stat(referenced)
	assert.NoError(t, err, "used image must survive")
	_, err = os.Stat(orphaned)
	assert.ErrorIs(t, err, os.ErrNotExist, "unused image must be deleted")
}

func TestRun_MultipleArticles_SharedStore(t *testing.T) {
	root := t.TempDir()
	store := t.TempDir()

	// Store file referenced by both articles.
	storeFile := filepath.Join(store, "ab", "cdef.png")
	writeFile(t, storeFile, "shared-image")

	// Article 1 references the image.
	art1ImgDir := filepath.Join(root, "art1", "img")
	require.NoError(t, os.MkdirAll(art1ImgDir, 0o755))
	writeFile(t, filepath.Join(root, "art1", "index.md"),
		"# Art1\n\n![img](img/abcdef.png)\n")
	symlink(t,
		filepath.Join(store, "ab", "cdef.png"),
		filepath.Join(art1ImgDir, "abcdef.png"),
	)

	// Article 2 also references the same store file.
	art2ImgDir := filepath.Join(root, "art2", "img")
	require.NoError(t, os.MkdirAll(art2ImgDir, 0o755))
	writeFile(t, filepath.Join(root, "art2", "index.md"),
		"# Art2\n\n![img](img/abcdef.png)\n")
	symlink(t,
		filepath.Join(store, "ab", "cdef.png"),
		filepath.Join(art2ImgDir, "abcdef.png"),
	)

	stats, err := Run(root, store, false)
	require.NoError(t, err)
	assert.Equal(t, Stats{}, stats)
	_, err = os.Stat(storeFile)
	assert.NoError(t, err, "shared store file must survive")
}

func TestRun_OrphanedSymlink_StoreFileCleaned(t *testing.T) {
	root := t.TempDir()
	store := t.TempDir()

	storeFile := filepath.Join(store, "ab", "cdef.png")
	writeFile(t, storeFile, "image-data")

	imgDir := filepath.Join(root, "article", "img")
	require.NoError(t, os.MkdirAll(imgDir, 0o755))
	writeFile(t, filepath.Join(root, "article", "index.md"), "# No images.\n")
	symlink(t, storeFile, filepath.Join(imgDir, "abcdef.png"))

	stats, err := Run(root, store, false)
	require.NoError(t, err)
	// Both the symlink and the store file should be deleted.
	assert.Equal(t, 2, stats.DeletedFiles)
	_, err = os.Stat(filepath.Join(imgDir, "abcdef.png"))
	assert.ErrorIs(t, err, os.ErrNotExist, "orphaned symlink must be deleted")
	_, err = os.Stat(storeFile)
	assert.ErrorIs(t, err, os.ErrNotExist, "orphaned store file must be deleted")
}

func TestRun_DryRun_NoDeletions(t *testing.T) {
	root := t.TempDir()
	imgPath := filepath.Join(root, "article", "img", "orphan.png")
	writeFile(t, filepath.Join(root, "article", "index.md"), "# No images.\n")
	writeFile(t, imgPath, "data")

	stats, err := Run(root, "", true)
	require.NoError(t, err)
	// Dry run reports what would be deleted but makes no changes.
	assert.Equal(t, 1, stats.DeletedFiles)
	_, err = os.Stat(imgPath)
	assert.NoError(t, err, "dry run must not delete files")
}

func TestRun_StoreFileKeptWhenOneArticleStillReferences(t *testing.T) {
	root := t.TempDir()
	store := t.TempDir()

	storeFile := filepath.Join(store, "ab", "cdef.png")
	writeFile(t, storeFile, "shared-image")

	// Article 1 references the image.
	art1ImgDir := filepath.Join(root, "art1", "img")
	require.NoError(t, os.MkdirAll(art1ImgDir, 0o755))
	writeFile(t, filepath.Join(root, "art1", "index.md"),
		"# Art1\n\n![img](img/abcdef.png)\n")
	symlink(t, storeFile, filepath.Join(art1ImgDir, "abcdef.png"))

	// Article 2 does NOT reference its image in the markdown, but the
	// basename matches art1's markdown so it is kept (content-hashed
	// filenames make this safe).
	art2ImgDir := filepath.Join(root, "art2", "img")
	require.NoError(t, os.MkdirAll(art2ImgDir, 0o755))
	writeFile(t, filepath.Join(root, "art2", "index.md"), "# Art2 — no images.\n")
	symlink(t, storeFile, filepath.Join(art2ImgDir, "abcdef.png"))

	stats, err := Run(root, store, false)
	require.NoError(t, err)
	assert.Equal(t, Stats{}, stats)
	_, err = os.Stat(filepath.Join(art1ImgDir, "abcdef.png"))
	assert.NoError(t, err, "art1 symlink must survive")
	_, err = os.Stat(filepath.Join(art2ImgDir, "abcdef.png"))
	assert.NoError(t, err, "art2 symlink must survive (basename matched globally)")
	_, err = os.Stat(storeFile)
	assert.NoError(t, err, "store file must survive")
}

// fakeName generates a deterministic content-hashed image filename.
func fakeName(i int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("image-%d", i)))
	return fmt.Sprintf("%x.png", h[:12])
}

// benchPaths returns numImages fake image paths spread across 50 articles.
func benchPaths(numImages int) []string {
	paths := make([]string, numImages)
	for i := range paths {
		paths[i] = fmt.Sprintf("/root/article-%d/img/%s", i%50, fakeName(i))
	}
	return paths
}

// benchContent builds ~contentSize bytes of markdown that references ~20% of
// the images from benchPaths.
func benchContent(numImages, contentSize int) string {
	rng := rand.New(rand.NewPCG(42, 0))
	var buf strings.Builder
	buf.WriteString("# Benchmark Article\n\nLorem ipsum dolor sit amet.\n\n")
	referencedCount := numImages / 5
	for buf.Len() < contentSize {
		if referencedCount > 0 && rng.IntN(5) == 0 {
			idx := rng.IntN(numImages)
			fmt.Fprintf(&buf, "![img](img/%s)\n\n", fakeName(idx))
			referencedCount--
		} else {
			buf.WriteString("Paragraph of filler text that does not contain any image references at all. ")
			buf.WriteString("More words to pad the content so the benchmark is realistic.\n\n")
		}
	}
	return buf.String()
}

func BenchmarkNewImageTracker(b *testing.B) {
	paths := benchPaths(1000)
	for b.Loop() {
		newImageTracker(paths)
	}
}

func BenchmarkScanMarkdown(b *testing.B) {
	const numImages = 1000
	paths := benchPaths(numImages)
	content := benchContent(numImages, 10_000)
	tracker := newImageTracker(paths)

	for b.Loop() {
		tracker.visited = make(map[string]bool)
		tracker.ScanMarkdown(content)
	}
}

func BenchmarkRun(b *testing.B) {
	const numArticles = 200
	const numImagesPerArticle = 3
	const orphanFraction = 0.2

	root := b.TempDir()
	rng := rand.New(rand.NewPCG(99, 0))

	imgIdx := 0
	for a := range numArticles {
		artDir := filepath.Join(root, fmt.Sprintf("article-%d", a))
		imgDir := filepath.Join(artDir, "img")
		if err := os.MkdirAll(imgDir, 0o755); err != nil {
			b.Fatal(err)
		}

		var md strings.Builder
		md.WriteString("# Article\n\nSome introductory text.\n\n")
		for i := range numImagesPerArticle {
			name := fakeName(imgIdx + i)
			if err := os.WriteFile(filepath.Join(imgDir, name), []byte("data"), 0o644); err != nil {
				b.Fatal(err)
			}
			if rng.Float64() >= orphanFraction {
				fmt.Fprintf(&md, "![photo](img/%s)\n\n", name)
			}
			md.WriteString("More filler text for realism. ")
		}
		imgIdx += numImagesPerArticle

		if err := os.WriteFile(filepath.Join(artDir, "index.md"), []byte(md.String()), 0o644); err != nil {
			b.Fatal(err)
		}
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	for b.Loop() {
		if _, err := Run(root, "", true); err != nil {
			b.Fatal(err)
		}
	}
}
