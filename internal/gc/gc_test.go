package gc

import (
	"os"
	"path/filepath"
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

func TestExtractImageRefs(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		want     []string
	}{
		{
			name:     "no images",
			markdown: "# Hello\n\nJust text.",
			want:     nil,
		},
		{
			name:     "single image",
			markdown: "![alt](img/abc123.png)",
			want:     []string{"img/abc123.png"},
		},
		{
			name:     "multiple images",
			markdown: "![a](img/one.png)\n\n![b](img/two.jpg)",
			want:     []string{"img/one.png", "img/two.jpg"},
		},
		{
			name:     "non-img link ignored",
			markdown: "![a](https://example.com/photo.png)",
			want:     nil,
		},
		{
			name:     "mixed img and external",
			markdown: "![a](img/local.png) ![b](https://example.com/x.png)",
			want:     []string{"img/local.png"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractImageRefs(tc.markdown)
			assert.Equal(t, tc.want, got)
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

	// Article 2 does NOT reference its image in the markdown.
	art2ImgDir := filepath.Join(root, "art2", "img")
	require.NoError(t, os.MkdirAll(art2ImgDir, 0o755))
	writeFile(t, filepath.Join(root, "art2", "index.md"), "# Art2 — no images.\n")
	symlink(t, storeFile, filepath.Join(art2ImgDir, "abcdef.png"))

	stats, err := Run(root, store, false)
	require.NoError(t, err)
	// Only the orphaned symlink from art2 is deleted; store file survives.
	assert.Equal(t, 1, stats.DeletedFiles)
	_, err = os.Stat(filepath.Join(art1ImgDir, "abcdef.png"))
	assert.NoError(t, err, "art1 symlink must survive")
	_, err = os.Stat(filepath.Join(art2ImgDir, "abcdef.png"))
	assert.ErrorIs(t, err, os.ErrNotExist, "art2 orphaned symlink must be deleted")
	_, err = os.Stat(storeFile)
	assert.NoError(t, err, "store file still referenced by art1 must survive")
}
