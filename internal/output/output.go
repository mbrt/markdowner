// Package output handles writing converted content to files or stdout.
package output

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
)

// Mode controls how the output directory is structured.
type Mode string

const (
	// ModeFlat writes all files directly into the output directory (default).
	ModeFlat Mode = "flat"
	// ModeWeek organizes files into YYYY/wWW subdirectories based on saved time.
	ModeWeek Mode = "week"
)

// OverwriteMode controls behavior when a target file already exists.
type OverwriteMode string

const (
	// OverwriteAll overwrites both markdown and image files unconditionally.
	OverwriteAll OverwriteMode = "all"
	// OverwriteMD overwrites the markdown file but preserves existing images.
	OverwriteMD OverwriteMode = "md"
	// OverwriteEmpty overwrites the markdown file only when its body is empty
	// (frontmatter only, no content). Images are always preserved.
	OverwriteEmpty OverwriteMode = "empty"
	// OverwriteNone skips writing when the target file already exists.
	OverwriteNone OverwriteMode = "none"
)

// ParseOverwriteMode validates and returns an OverwriteMode from a string.
func ParseOverwriteMode(s string) (OverwriteMode, error) {
	switch OverwriteMode(s) {
	case OverwriteAll, OverwriteMD, OverwriteEmpty, OverwriteNone:
		return OverwriteMode(s), nil
	default:
		return "", fmt.Errorf("invalid --overwrite %q: must be one of all, md, empty, none", s)
	}
}

// Frontmatter holds the metadata written at the top of each Markdown file.
type Frontmatter struct {
	Title  string     `yaml:"title"`
	Author string     `yaml:"author,omitempty"`
	URL    string     `yaml:"url"`
	Source string     `yaml:"source,omitempty"`
	Date   *time.Time `yaml:"date,omitempty"`
	Saved  time.Time  `yaml:"saved"`
	Tags   []string   `yaml:"tags,omitempty"`
}

// Doc holds the complete content of a fetched page, ready to write to disk.
type Doc struct {
	Frontmatter Frontmatter
	Markdown    string
	// Images maps relative local paths ("img/<sha>.<ext>") to raw image bytes.
	// When non-empty, WriteDoc writes each image blob to <outDir>/<key>.
	Images map[string][]byte
}

// Writer writes Docs to the filesystem according to its configuration.
type Writer struct {
	outDir        string
	mode          Mode
	imageStoreDir string
	// IgnoreFailures, when true, causes WriteDocs to write a stub file
	// (frontmatter only, no body) for results that have an error but carry
	// partial Doc metadata. The stub is skipped when the target file already
	// exists.
	IgnoreFailures bool
	// Overwrite controls how existing output files are handled.
	// Defaults to OverwriteNone (skip if exists).
	Overwrite OverwriteMode
}

// NewWriter creates a Writer that writes to outDir using the given mode.
// When imageStoreDir is non-empty, downloaded images are stored in a shared
// directory with a two-character subdirectory prefix for deduplication, and
// each article's img/ directory contains relative symlinks into that store.
func NewWriter(outDir string, mode Mode, imageStoreDir string) Writer {
	return Writer{
		outDir:        outDir,
		mode:          mode,
		imageStoreDir: imageStoreDir,
		Overwrite:     OverwriteNone,
	}
}

// Result holds either a successfully converted doc or an error.
type Result struct {
	Doc Doc
	Err error
}

// WriteDoc writes a Markdown file with YAML frontmatter, and any image blobs,
// to the appropriate subdirectory under the configured output directory.
// It returns the path of the written Markdown file, or an empty string when
// the file was skipped due to the configured Overwrite policy.
func (w Writer) WriteDoc(doc Doc) (string, error) {
	dir := w.outDir
	if w.mode == ModeWeek {
		dir = weekSubDir(w.outDir, doc.Frontmatter.Saved)
	}
	return writeFile(dir, w.imageStoreDir, doc, w.Overwrite)
}

// WriteDocs consumes results from a channel, writes each successful doc to
// disk, and logs warnings for any errors. It returns the number of
// successfully written docs and the number of failures.
func (w Writer) WriteDocs(results <-chan Result) (written, failed int) {
	for res := range results {
		if res.Err != nil {
			slog.Warn("fetching article", "err", res.Err)
			if w.IgnoreFailures && res.Doc.Frontmatter.URL != "" {
				if err := w.WriteStub(res.Doc); err != nil {
					slog.Warn("writing stub article", "url", res.Doc.Frontmatter.URL, "err", err)
				}
			}
			failed++
			continue
		}
		path, err := w.WriteDoc(res.Doc)
		if err != nil {
			slog.Warn("writing article", "title", res.Doc.Frontmatter.Title, "err", err)
			failed++
			continue
		}
		if path == "" {
			slog.Info("skipped (file exists)", "title", res.Doc.Frontmatter.Title)
			continue
		}
		slog.Info("written", "path", path)
		written++
	}
	return written, failed
}

// WriteStub writes a frontmatter-only Markdown file for doc. It is a no-op
// (with an info log) when the target file already exists, so it never
// overwrites existing content.
func (w Writer) WriteStub(doc Doc) error {
	dir := w.outDir
	if w.mode == ModeWeek {
		dir = weekSubDir(w.outDir, doc.Frontmatter.Saved)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	filename := slugify(targetName(doc))
	path := filepath.Join(dir, filename+".md")
	if _, err := os.Stat(path); err == nil {
		slog.Info("skipping stub (file already exists)", "path", path)
		return nil
	}
	stub := Doc{Frontmatter: doc.Frontmatter}
	path, err := writeFile(dir, "", stub, OverwriteAll)
	if err != nil {
		return err
	}
	slog.Info("written stub", "path", path)
	return nil
}

// writeImageToStore writes image data to the shared store and creates a
// relative symlink inside <docDir>/img/ pointing to the store file.
//
// relPath is of the form "img/<hash><ext>" as produced by the images plugin.
// The store places the file at <storeDir>/<hash[0:2]>/<hash[2:]><ext>, using
// the first two characters of the filename as a subdirectory to avoid large
// flat directories. The store write is skipped when overwrite is false and
// the target already exists. The symlink creation is always idempotent.
func writeImageToStore(docDir, storeDir, relPath string, data []byte, overwrite bool) error {
	// relPath = "img/<hash><ext>", e.g. "img/0a1b2c….png"
	name := strings.TrimPrefix(relPath, "img/")
	if len(name) < 2 {
		return fmt.Errorf("image path too short to split into subdirectory: %q", relPath)
	}
	subDir := name[0:2]
	fileName := name[2:]

	storePath := filepath.Join(storeDir, subDir, fileName)

	// Write to store (skip if already present — deduplication).
	if err := os.MkdirAll(filepath.Dir(storePath), 0o755); err != nil {
		return fmt.Errorf("creating image store directory: %w", err)
	}
	if _, err := os.Stat(storePath); errors.Is(err, os.ErrNotExist) || overwrite {
		if err := os.WriteFile(storePath, data, 0o644); err != nil {
			return fmt.Errorf("writing image to store %q: %w", storePath, err)
		}
	}

	// Create symlink in <docDir>/img/ pointing to the store file.
	imgDir := filepath.Join(docDir, "img")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		return fmt.Errorf("creating img directory: %w", err)
	}
	linkPath := filepath.Join(imgDir, name)

	// Compute a relative symlink target so the tree stays portable.
	absImgDir, err := filepath.Abs(imgDir)
	if err != nil {
		return fmt.Errorf("resolving img directory: %w", err)
	}
	absStorePath, err := filepath.Abs(storePath)
	if err != nil {
		return fmt.Errorf("resolving store path: %w", err)
	}
	target, err := filepath.Rel(absImgDir, absStorePath)
	if err != nil {
		return fmt.Errorf("computing relative symlink target: %w", err)
	}

	if err := os.Symlink(target, linkPath); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("creating symlink %q -> %q: %w", linkPath, target, err)
	}
	return nil
}

// weekSubDir returns baseDir/YYYY/wWW for the ISO week containing t.
func weekSubDir(baseDir string, t time.Time) string {
	year, week := t.ISOWeek()
	return filepath.Join(baseDir, fmt.Sprintf("%04d", year), fmt.Sprintf("w%02d", week))
}

// isBodyEmpty reports whether the markdown file at path has an empty body
// (i.e., contains only YAML frontmatter with no content after the closing
// delimiter). Files that cannot be parsed as frontmatter are treated as empty.
func isBodyEmpty(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	// Format: "---\n<yaml>---\n\n<body>"
	parts := strings.SplitN(string(data), "---\n", 3)
	if len(parts) < 3 {
		return true, nil
	}
	return strings.TrimSpace(parts[2]) == "", nil
}

// shouldSkipMD reports whether writing to path should be skipped given the
// overwrite mode. It returns true when the file exists and the mode does not
// allow overwriting it.
func shouldSkipMD(path string, overwrite OverwriteMode) (bool, error) {
	switch overwrite {
	case OverwriteNone:
		if _, err := os.Stat(path); err == nil {
			return true, nil
		}
	case OverwriteEmpty:
		if _, err := os.Stat(path); err == nil {
			empty, err := isBodyEmpty(path)
			if err != nil {
				return true, nil
			}
			return !empty, nil
		}
	}
	return false, nil
}

// writeImage writes a single image blob to dest, honoring the overwrite flag.
func writeImage(dest string, data []byte, overwrite bool) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("creating image directory: %w", err)
	}
	// Skip writing the image if the target already exists, to avoid
	// churn on slightly different image data across runs.
	if !overwrite {
		if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
			return nil
		}
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return fmt.Errorf("writing image %q: %w", dest, err)
	}
	return nil
}

func writeFile(outDir string, imageStoreDir string, doc Doc, overwrite OverwriteMode) (string, error) {
	filename := slugify(targetName(doc))
	fm := doc.Frontmatter
	body := doc.Markdown
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("creating output directory: %w", err)
	}
	fm.Saved = fm.Saved.Truncate(time.Second)
	if fm.Date != nil {
		t := fm.Date.Truncate(time.Second)
		fm.Date = &t
	}
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return "", fmt.Errorf("marshaling frontmatter: %w", err)
	}
	content := "---\n" + string(fmBytes) + "---\n\n" + body
	path := filepath.Join(outDir, filename+".md")

	skip, err := shouldSkipMD(path, overwrite)
	if err != nil || skip {
		return "", err
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("writing file %q: %w", path, err)
	}
	overwriteImages := overwrite == OverwriteAll
	for relPath, data := range doc.Images {
		if imageStoreDir != "" {
			if err := writeImageToStore(outDir, imageStoreDir, relPath, data, overwriteImages); err != nil {
				return "", err
			}
		} else {
			dest := filepath.Join(outDir, relPath)
			if err := writeImage(dest, data, overwriteImages); err != nil {
				return "", err
			}
		}
	}
	return path, nil
}

func targetName(doc Doc) string {
	if doc.Frontmatter.Title != "" {
		return doc.Frontmatter.Title
	}
	u, err := url.Parse(doc.Frontmatter.URL)
	if err != nil {
		return doc.Frontmatter.URL
	}
	return u.Host + u.Path
}

var reNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(s)
	// Replace non-ASCII with spaces.
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return '-'
	}, s)
	s = reNonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 80 {
		s = s[:80]
		s = strings.TrimRight(s, "-")
	}
	if s == "" {
		s = "untitled"
	}
	return s
}
