package output

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"  leading and trailing  ", "leading-and-trailing"},
		{"multiple   spaces", "multiple-spaces"},
		{"Special! @#$ Characters", "special-characters"},
		{"café au lait", "caf-au-lait"},
		{"", "untitled"},
		{"   ", "untitled"},
		{"already-slug", "already-slug"},
		{"UPPER CASE", "upper-case"},
		{strings.Repeat("a", 100), strings.Repeat("a", 80)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, slugify(tt.input))
		})
	}
}

func TestSlugify_NoTrailingDash(t *testing.T) {
	s := strings.Repeat("a", 79) + "-extra"
	got := slugify(s)
	assert.False(t, strings.HasSuffix(got, "-"), "result should not have trailing dash: %q", got)
}

func TestWriteDoc(t *testing.T) {
	dir := t.TempDir()
	d := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	fm := Frontmatter{
		Title: "Test Article",
		URL:   "https://example.com/test",
		Date:  &d,
		Saved: time.Date(2024, 4, 1, 8, 0, 0, 0, time.UTC),
		Tags:  []string{"go", "testing"},
	}
	body := "# Test\n\nHello world."

	doc := Doc{
		Frontmatter: fm,
		Markdown:    body,
	}
	_, err := NewWriter(dir, ModeFlat, "").WriteDoc(doc)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "test-article.md"))
	require.NoError(t, err)
	s := string(content)

	assert.True(t, strings.HasPrefix(s, "---\n"), "file should start with YAML frontmatter delimiter")
	assert.Contains(t, s, "date: 2024-03-01T12:00:00Z")
	assert.Contains(t, s, "saved: 2024-04-01T08:00:00Z")
	assert.Contains(t, s, "title: Test Article")
	assert.Contains(t, s, "url: https://example.com/test")
	assert.Contains(t, s, "- go")
	assert.Contains(t, s, "Hello world.")
}

func TestWriteDoc_NoTags(t *testing.T) {
	dir := t.TempDir()
	fm := Frontmatter{
		Title: "No Tags",
		URL:   "https://example.com/notags",
		Saved: time.Now(),
	}

	_, err := NewWriter(dir, ModeFlat, "").WriteDoc(Doc{Frontmatter: fm, Markdown: "body"})
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "no-tags.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(content), "tags:")
	assert.NotContains(t, string(content), "date:")
}

func TestWriteDoc_CreatesDirectory(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "sub", "dir")
	fm := Frontmatter{Title: "X", URL: "https://x.com", Saved: time.Now()}

	_, err := NewWriter(dir, ModeFlat, "").WriteDoc(Doc{Frontmatter: fm, Markdown: ""})
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "x.md"))
	assert.NoError(t, err)
}

func TestWriteDoc_WritesImages(t *testing.T) {
	dir := t.TempDir()
	fm := Frontmatter{Title: "Img Test", URL: "https://example.com/img", Saved: time.Now()}
	doc := Doc{
		Frontmatter: fm,
		Markdown:    "![pic](img/abc123.png)",
		Images: map[string][]byte{
			"img/abc123.png": []byte("fakepng"),
		},
	}

	_, err := NewWriter(dir, ModeFlat, "").WriteDoc(doc)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "img", "abc123.png"))
	require.NoError(t, err)
	assert.Equal(t, []byte("fakepng"), data)
}

func TestWriteDoc_EmptyTitleFallsBackToURL(t *testing.T) {
	dir := t.TempDir()
	fm := Frontmatter{Title: "", URL: "https://example.com/some/page", Saved: time.Now()}

	path, err := NewWriter(dir, ModeFlat, "").WriteDoc(Doc{Frontmatter: fm, Markdown: "body"})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "example-com-some-page.md"), path)
}

func TestWeekSubDir(t *testing.T) {
	tests := []struct {
		name    string
		saved   time.Time
		wantSub string
	}{
		{
			name:    "mid year week",
			saved:   time.Date(2024, 3, 18, 0, 0, 0, 0, time.UTC), // ISO week 12
			wantSub: filepath.Join("2024", "w12"),
		},
		{
			name:    "first week zero-padded",
			saved:   time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC), // ISO week 2
			wantSub: filepath.Join("2024", "w02"),
		},
		{
			name:    "week 1 of year",
			saved:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), // ISO week 1 of 2024
			wantSub: filepath.Join("2024", "w01"),
		},
		{
			name:    "late December in next ISO year",
			saved:   time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC), // ISO week 53 of 2020
			wantSub: filepath.Join("2020", "w53"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := weekSubDir("base", tt.saved)
			assert.Equal(t, filepath.Join("base", tt.wantSub), got)
		})
	}
}

func TestWriteDoc_ModeWeek(t *testing.T) {
	dir := t.TempDir()
	saved := time.Date(2024, 3, 18, 0, 0, 0, 0, time.UTC) // ISO week 12
	fm := Frontmatter{Title: "Week Test", URL: "https://example.com/w", Saved: saved}

	path, err := NewWriter(dir, ModeWeek, "").WriteDoc(Doc{Frontmatter: fm, Markdown: "body"})
	require.NoError(t, err)

	expected := filepath.Join(dir, "2024", "w12", "week-test.md")
	assert.Equal(t, expected, path)
	_, err = os.Stat(expected)
	assert.NoError(t, err)
}

func TestWriteDoc_ImageStore_SubdirStructure(t *testing.T) {
	outDir := t.TempDir()
	storeDir := t.TempDir()

	fm := Frontmatter{Title: "Store Test", URL: "https://example.com/store", Saved: time.Now()}
	// Use a name long enough to split: "ab" subdir + "cdef.png" filename.
	doc := Doc{
		Frontmatter: fm,
		Markdown:    "![pic](img/abcdef.png)",
		Images:      map[string][]byte{"img/abcdef.png": []byte("imgdata")},
	}

	_, err := NewWriter(outDir, ModeFlat, storeDir).WriteDoc(doc)
	require.NoError(t, err)

	// Image must be in store at <storeDir>/ab/cdef.png.
	storePath := filepath.Join(storeDir, "ab", "cdef.png")
	data, err := os.ReadFile(storePath)
	require.NoError(t, err)
	assert.Equal(t, []byte("imgdata"), data)
}

func TestWriteDoc_ImageStore_SymlinkCreated(t *testing.T) {
	outDir := t.TempDir()
	storeDir := t.TempDir()

	fm := Frontmatter{Title: "Symlink Test", URL: "https://example.com/sym", Saved: time.Now()}
	doc := Doc{
		Frontmatter: fm,
		Markdown:    "![pic](img/abcdef.png)",
		Images:      map[string][]byte{"img/abcdef.png": []byte("imgdata")},
	}

	_, err := NewWriter(outDir, ModeFlat, storeDir).WriteDoc(doc)
	require.NoError(t, err)

	linkPath := filepath.Join(outDir, "img", "abcdef.png")

	// Must be a symlink.
	fi, err := os.Lstat(linkPath)
	require.NoError(t, err)
	assert.True(t, fi.Mode()&os.ModeSymlink != 0, "expected a symlink at %s", linkPath)

	// Symlink target must be relative (not absolute).
	target, err := os.Readlink(linkPath)
	require.NoError(t, err)
	assert.False(t, filepath.IsAbs(target), "symlink target should be relative, got %q", target)

	// Following the symlink must reach the actual store file.
	resolved, err := filepath.EvalSymlinks(linkPath)
	require.NoError(t, err)
	absStore, err := filepath.Abs(filepath.Join(storeDir, "ab", "cdef.png"))
	require.NoError(t, err)
	assert.Equal(t, absStore, resolved)
}

func TestWriteDoc_ImageStore_Deduplication(t *testing.T) {
	outDir := t.TempDir()
	storeDir := t.TempDir()

	fm := Frontmatter{Title: "Dedup Test", URL: "https://example.com/dedup", Saved: time.Now()}
	doc := Doc{
		Frontmatter: fm,
		Markdown:    "![pic](img/abcdef.png)",
		Images:      map[string][]byte{"img/abcdef.png": []byte("imgdata")},
	}
	w := NewWriter(outDir, ModeFlat, storeDir)

	// Write the same document twice; both should succeed without error.
	_, err := w.WriteDoc(doc)
	require.NoError(t, err)
	_, err = w.WriteDoc(doc)
	require.NoError(t, err)

	// The store file is written exactly once (unchanged).
	data, err := os.ReadFile(filepath.Join(storeDir, "ab", "cdef.png"))
	require.NoError(t, err)
	assert.Equal(t, []byte("imgdata"), data)
}

func TestWriteDoc_ImageStore_WeekMode_RelativeSymlink(t *testing.T) {
	outDir := t.TempDir()
	storeDir := t.TempDir()

	saved := time.Date(2024, 3, 18, 0, 0, 0, 0, time.UTC) // ISO week 12
	fm := Frontmatter{Title: "Week Store", URL: "https://example.com/ws", Saved: saved}
	doc := Doc{
		Frontmatter: fm,
		Markdown:    "![pic](img/abcdef.png)",
		Images:      map[string][]byte{"img/abcdef.png": []byte("imgdata")},
	}

	_, err := NewWriter(outDir, ModeWeek, storeDir).WriteDoc(doc)
	require.NoError(t, err)

	linkPath := filepath.Join(outDir, "2024", "w12", "img", "abcdef.png")

	fi, err := os.Lstat(linkPath)
	require.NoError(t, err)
	assert.True(t, fi.Mode()&os.ModeSymlink != 0, "expected a symlink at %s", linkPath)

	target, err := os.Readlink(linkPath)
	require.NoError(t, err)
	assert.False(t, filepath.IsAbs(target), "symlink target should be relative, got %q", target)

	// Symlink must resolve to the store file.
	resolved, err := filepath.EvalSymlinks(linkPath)
	require.NoError(t, err)
	absStore, err := filepath.Abs(filepath.Join(storeDir, "ab", "cdef.png"))
	require.NoError(t, err)
	assert.Equal(t, absStore, resolved)
}

func TestWriteStub_WritesOnlyFrontmatter(t *testing.T) {
	dir := t.TempDir()
	saved := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	doc := Doc{
		Frontmatter: Frontmatter{
			Title: "Stub Article",
			URL:   "https://example.com/stub",
			Saved: saved,
			Tags:  []string{"stub"},
		},
	}

	w := NewWriter(dir, ModeFlat, "")
	err := w.WriteStub(doc)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "stub-article.md"))
	require.NoError(t, err)
	s := string(content)

	assert.True(t, strings.HasPrefix(s, "---\n"), "file should start with YAML frontmatter delimiter")
	assert.Contains(t, s, "title: Stub Article")
	assert.Contains(t, s, "url: https://example.com/stub")
	assert.Contains(t, s, "saved: 2024-06-01T10:00:00Z")
	assert.Contains(t, s, "- stub")
	// No markdown body after the closing delimiter.
	parts := strings.SplitN(s, "---\n", 3)
	require.Len(t, parts, 3, "expected opening ---, frontmatter, and closing ---")
	assert.Empty(t, strings.TrimSpace(parts[2]), "stub should have no markdown body")
}

func TestWriteStub_SkipsExistingFile(t *testing.T) {
	dir := t.TempDir()
	saved := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	doc := Doc{
		Frontmatter: Frontmatter{
			Title: "Existing Article",
			URL:   "https://example.com/existing",
			Saved: saved,
		},
	}

	// Pre-create the file with different content.
	path := filepath.Join(dir, "existing-article.md")
	original := "original content"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	w := NewWriter(dir, ModeFlat, "")
	err := w.WriteStub(doc)
	require.NoError(t, err)

	// File must not have been overwritten.
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(content))
}

func TestWriteDocs_IgnoreFailures_WritesStub(t *testing.T) {
	dir := t.TempDir()
	saved := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	results := make(chan Result, 2)
	results <- Result{
		Doc: Doc{
			Frontmatter: Frontmatter{
				Title: "Failed Article",
				URL:   "https://example.com/fail",
				Saved: saved,
			},
		},
		Err: assert.AnError,
	}
	results <- Result{
		Doc: Doc{
			Frontmatter: Frontmatter{Title: "OK Article", URL: "https://example.com/ok", Saved: saved},
			Markdown:    "body",
		},
	}
	close(results)

	w := NewWriter(dir, ModeFlat, "")
	w.IgnoreFailures = true
	written, failed := w.WriteDocs(results)

	assert.Equal(t, 1, written)
	assert.Equal(t, 1, failed)

	// Stub for the failed article must exist.
	content, err := os.ReadFile(filepath.Join(dir, "failed-article.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "url: https://example.com/fail")
	// No body in stub.
	parts := strings.SplitN(string(content), "---\n", 3)
	require.Len(t, parts, 3)
	assert.Empty(t, strings.TrimSpace(parts[2]))

	// Full article must also exist.
	_, err = os.Stat(filepath.Join(dir, "ok-article.md"))
	require.NoError(t, err)
}

func TestWriteDocs_IgnoreFailures_False_NoStub(t *testing.T) {
	dir := t.TempDir()
	saved := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	results := make(chan Result, 1)
	results <- Result{
		Doc: Doc{
			Frontmatter: Frontmatter{
				Title: "Failed Article",
				URL:   "https://example.com/fail",
				Saved: saved,
			},
		},
		Err: assert.AnError,
	}
	close(results)

	w := NewWriter(dir, ModeFlat, "")
	// IgnoreFailures defaults to false.
	written, failed := w.WriteDocs(results)

	assert.Equal(t, 0, written)
	assert.Equal(t, 1, failed)

	// No stub should have been written.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestIsBodyEmpty(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "empty body",
			content: "---\ntitle: Test\n---\n\n",
			want:    true,
		},
		{
			name:    "whitespace-only body",
			content: "---\ntitle: Test\n---\n\n   \n\t\n",
			want:    true,
		},
		{
			name:    "non-empty body",
			content: "---\ntitle: Test\n---\n\n# Hello\n\nSome content.",
			want:    false,
		},
		{
			name:    "no frontmatter delimiters",
			content: "just some text",
			want:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "test.md")
			require.NoError(t, os.WriteFile(path, []byte(tt.content), 0o644))
			got, err := isBodyEmpty(path)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseOverwriteMode(t *testing.T) {
	tests := []struct {
		input   string
		want    OverwriteMode
		wantErr bool
	}{
		{"all", OverwriteAll, false},
		{"md", OverwriteMD, false},
		{"empty", OverwriteEmpty, false},
		{"none", OverwriteNone, false},
		{"invalid", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseOverwriteMode(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestWriteDoc_OverwriteModes(t *testing.T) {
	saved := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	baseFM := Frontmatter{
		Title: "My Article",
		URL:   "https://example.com/article",
		Saved: saved,
	}

	makeDoc := func(body string) Doc {
		return Doc{
			Frontmatter: baseFM,
			Markdown:    body,
			Images:      map[string][]byte{"img/pic.png": []byte("newimage")},
		}
	}

	// existingBody is the content we pre-write as the "existing" file.
	existingBody := "# Old\n\nExisting content."
	existingFile := func(dir string) string {
		content := "---\ntitle: My Article\nurl: https://example.com/article\nsaved: 2024-06-01T10:00:00Z\n---\n\n" + existingBody
		path := filepath.Join(dir, "my-article.md")
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
		return path
	}
	existingEmptyFile := func(dir string) string {
		content := "---\ntitle: My Article\nurl: https://example.com/article\nsaved: 2024-06-01T10:00:00Z\n---\n\n"
		path := filepath.Join(dir, "my-article.md")
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
		return path
	}

	t.Run("none_skips_existing_md", func(t *testing.T) {
		dir := t.TempDir()
		path := existingFile(dir)
		w := NewWriter(dir, ModeFlat, "")
		w.Overwrite = OverwriteNone
		got, err := w.WriteDoc(makeDoc("# New content"))
		require.NoError(t, err)
		assert.Empty(t, got, "should return empty path when skipped")
		content, _ := os.ReadFile(path)
		assert.Contains(t, string(content), existingBody, "existing file should be unchanged")
	})

	t.Run("none_writes_new_md", func(t *testing.T) {
		dir := t.TempDir()
		w := NewWriter(dir, ModeFlat, "")
		w.Overwrite = OverwriteNone
		got, err := w.WriteDoc(makeDoc("# New content"))
		require.NoError(t, err)
		assert.NotEmpty(t, got, "should return path when file is new")
	})

	t.Run("all_overwrites_md", func(t *testing.T) {
		dir := t.TempDir()
		existingFile(dir)
		w := NewWriter(dir, ModeFlat, "")
		w.Overwrite = OverwriteAll
		got, err := w.WriteDoc(makeDoc("# New content"))
		require.NoError(t, err)
		assert.NotEmpty(t, got)
		content, _ := os.ReadFile(got)
		assert.Contains(t, string(content), "# New content")
		assert.NotContains(t, string(content), existingBody)
	})

	t.Run("all_overwrites_image", func(t *testing.T) {
		dir := t.TempDir()
		imgPath := filepath.Join(dir, "img", "pic.png")
		require.NoError(t, os.MkdirAll(filepath.Dir(imgPath), 0o755))
		require.NoError(t, os.WriteFile(imgPath, []byte("oldimage"), 0o644))
		w := NewWriter(dir, ModeFlat, "")
		w.Overwrite = OverwriteAll
		_, err := w.WriteDoc(makeDoc("body"))
		require.NoError(t, err)
		data, _ := os.ReadFile(imgPath)
		assert.Equal(t, []byte("newimage"), data, "image should be overwritten with all mode")
	})

	t.Run("md_overwrites_md_preserves_image", func(t *testing.T) {
		dir := t.TempDir()
		existingFile(dir)
		imgPath := filepath.Join(dir, "img", "pic.png")
		require.NoError(t, os.MkdirAll(filepath.Dir(imgPath), 0o755))
		require.NoError(t, os.WriteFile(imgPath, []byte("oldimage"), 0o644))
		w := NewWriter(dir, ModeFlat, "")
		w.Overwrite = OverwriteMD
		got, err := w.WriteDoc(makeDoc("# New content"))
		require.NoError(t, err)
		assert.NotEmpty(t, got)
		content, _ := os.ReadFile(got)
		assert.Contains(t, string(content), "# New content", "md should be overwritten")
		data, _ := os.ReadFile(imgPath)
		assert.Equal(t, []byte("oldimage"), data, "image should be preserved with md mode")
	})

	t.Run("empty_overwrites_empty_md", func(t *testing.T) {
		dir := t.TempDir()
		existingEmptyFile(dir)
		w := NewWriter(dir, ModeFlat, "")
		w.Overwrite = OverwriteEmpty
		got, err := w.WriteDoc(makeDoc("# New content"))
		require.NoError(t, err)
		assert.NotEmpty(t, got, "should write when existing file has empty body")
		content, _ := os.ReadFile(got)
		assert.Contains(t, string(content), "# New content")
	})

	t.Run("empty_skips_nonempty_md", func(t *testing.T) {
		dir := t.TempDir()
		existingFile(dir)
		w := NewWriter(dir, ModeFlat, "")
		w.Overwrite = OverwriteEmpty
		got, err := w.WriteDoc(makeDoc("# New content"))
		require.NoError(t, err)
		assert.Empty(t, got, "should skip when existing file has content")
	})
}
