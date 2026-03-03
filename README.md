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
go build -o markdowner .
```

## Usage

### Convert a single URL

```sh
markdowner url https://example.com/article --out-dir ./output
```

Fetches the page, extracts the article content (via readability), converts it to Markdown, and writes `./output/<title-slug>.md`.

### Convert Instapaper articles

```sh
markdowner instapaper --out-dir ./output --since 2024-01-01
```

Fetches all articles from your Instapaper account (unread + archive folders), optionally filtered by date, and writes one `.md` file per article.

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--out-dir` | `.` | Directory to write Markdown files |
| `--since` | (all) | Only include articles added after this date (`YYYY-MM-DD` or RFC3339) |

## Output format

Each `.md` file contains YAML frontmatter followed by the article body:

```markdown
---
title: "Example Article"
url: https://example.com/article
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
