# markdowner

A Go CLI that converts web content to Markdown files.

## Install

```sh
go install github.com/mbrt/markdowner@latest
```

Or build from source:

```sh
git clone https://github.com/mbrt/markdowner
cd markdowner
go build .
```

### Docker

A Docker image supporting both `amd64` and `arm64` is published to GHCR
regularly.

```sh
docker run --rm -v "$PWD/output:/data" ghcr.io/mbrt/markdowner \
  url https://example.com/article --out-mode flat
```

The container writes to `/data` by default, so mount your output directory
there.

To build locally:

```sh
make build-docker
```

## Usage

### Convert a single URL

```sh
markdowner url https://example.com/article
```

Fetches the page, extracts the article content (via readability), converts it to Markdown, and writes to stdout. Use `--out-mode flat --out-dir ./output` to write to a file instead.

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--title` | (from page) | Override article title |
| `--author` | (from page) | Override article author |
| `--date` | (from page) | Override article date (`YYYY-MM-DD` or RFC3339) |
| `--source` | (none) | Set the `source` field in the output frontmatter |
| `--tags` | (none) | Add tags to the output (repeatable: `--tags foo --tags bar`) |

`--title`, `--author`, and `--date` cannot be used when multiple URLs are given.

### Convert local HTML files

```sh
markdowner html article.html
```

Reads the local HTML file, extracts the article content (via readability), converts it to Markdown, and writes to stdout. Multiple files can be given and are processed sequentially.

```sh
# Write to files instead
markdowner html article.html --out-mode flat --out-dir ./output
```

```sh
# Read from stdin
cat article.html | markdowner html --url https://example.com/article
```

When no file arguments are given, HTML is read from stdin. Use `--url` to set the base URL for resolving relative links and the `url` frontmatter field.

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--url` | `file://<path>` for files, empty for stdin | Base URL for relative links and the `url` frontmatter field |
| `--title` | (from page) | Override article title |
| `--author` | (from page) | Override article author |
| `--date` | (from page) | Override article date (`YYYY-MM-DD` or RFC3339) |
| `--saved` | (now) | Override saved date in RFC3339 or YYYY-MM-DD |
| `--source` | (none) | Set the `source` field in the output frontmatter |
| `--tags` | (none) | Add tags to the output (repeatable: `--tags foo --tags bar`) |

`--title`, `--author`, and `--date` cannot be used when multiple files are given.

### Convert Instapaper articles

```sh
markdowner instapaper --out-mode flat --out-dir ./output --since 2024-01-01
```

Fetches all articles from your Instapaper account (unread + archive folders), optionally filtered by date, and writes one `.md` file per article.

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--since` | (all) | Only include articles added after this date (`YYYY-MM-DD` or RFC3339) |

## Global flags

These flags apply to all subcommands (`url`, `html`, `instapaper`):

| Flag | Default | Description |
|------|---------|-------------|
| `--out-mode` | `stdout` | Output mode: `stdout` (print to standard output), `flat` (all files in one dir), or `week` (subdirs by ISO week, e.g. `2024/w12/`) |
| `--out-dir` | `.` | Directory to write Markdown files (ignored in `stdout` mode) |
| `--download-images` | `false` | Download external images and rewrite references to local `img/<hash>.<ext>` paths (not supported in `stdout` mode) |
| `--image-store` | (none) | Shared image store directory (see below; not supported in `stdout` mode) |
| `--max-image-size` | (none) | Maximum image size before re-encoding as JPEG (e.g. `500KB`, `2MB`; not supported in `stdout` mode) |
| `--timeout` | `10s` | Per-item timeout |
| `-j` / `--parallel` | `4` | Number of parallel fetches |

> **Note:** In `stdout` mode, image references are kept as their original URLs (no local file rewriting). The flags `--out-dir`, `--overwrite`, `--download-images`, `--image-store`, and `--max-image-size` are rejected when `--out-mode stdout` is used explicitly alongside them.

### Image store (deduplication)

When `--image-store <dir>` is set alongside `--download-images`, images are
stored once in a shared directory instead of being copied into every article's
`img/` folder. This deduplicates identical images across articles.

**Store layout:** images are placed at `<storeDir>/<first-2-chars>/<rest>.<ext>`
to keep directory sizes manageable:

```
store/
  0a/
    1b2c3d…ef.png   ← full path: store/0a/1b2c3d…ef.png
  ff/
    a1b2c3…de.jpg
```

**`img/` symlinks:** each article's `img/` directory gets a *relative* symlink
pointing into the store, so the Markdown references (`img/<hash>.<ext>`) remain
unchanged:

```
output/
  my-article.md          ← references img/0a1b2c….png
  img/
    0a1b2c….png → ../../store/0a/1b2c….png   (relative symlink)
```

Because both the store write and the symlink creation are skipped when the
target already exists, re-running `markdowner` is safe and idempotent.

**Example:**

```sh
markdowner url https://example.com/article \
  --out-dir ./output \
  --download-images \
  --image-store ./store
```



Each `.md` file contains YAML frontmatter followed by the article body:

```markdown
---
title: "Example Article"
url: https://example.com/article
source: instapaper
date: 2024-03-01T12:00:00Z
tags:
  - tech
  - reading
---

Article content in Markdown...
```

## Instapaper credentials

The `instapaper` subcommand reads credentials from environment variables:

| Variable | Description |
|----------|-------------|
| `INSTAPAPER_CONSUMER_KEY` | OAuth consumer key |
| `INSTAPAPER_CONSUMER_SECRET` | OAuth consumer secret |
| `INSTAPAPER_USERNAME` | Instapaper account username/email |
| `INSTAPAPER_PASSWORD` | Instapaper account password |

You can request a consumer key/secret from [Instapaper's Full API](https://www.instapaper.com/api/full).
