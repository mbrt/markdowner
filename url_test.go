package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mbrt/markdowner/internal/output"
)

func TestApplyURLOverrides(t *testing.T) {
	baseDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	overrideDate := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		doc    output.Doc
		title  string
		author string
		source string
		date   *time.Time
		tags   []string
		want   output.Frontmatter
	}{
		{
			name: "no overrides leaves frontmatter unchanged",
			doc: output.Doc{Frontmatter: output.Frontmatter{
				Title:  "Original Title",
				Author: "Original Author",
				Date:   &baseDate,
				Tags:   []string{"a"},
			}},
			want: output.Frontmatter{
				Title:  "Original Title",
				Author: "Original Author",
				Date:   &baseDate,
				Tags:   []string{"a"},
			},
		},
		{
			name:  "override title",
			doc:   output.Doc{Frontmatter: output.Frontmatter{Title: "Old", Author: "Auth"}},
			title: "New Title",
			want:  output.Frontmatter{Title: "New Title", Author: "Auth"},
		},
		{
			name:   "override author",
			doc:    output.Doc{Frontmatter: output.Frontmatter{Title: "T", Author: "Old"}},
			author: "New Author",
			want:   output.Frontmatter{Title: "T", Author: "New Author"},
		},
		{
			name:   "override source",
			doc:    output.Doc{Frontmatter: output.Frontmatter{Title: "T"}},
			source: "mysite",
			want:   output.Frontmatter{Title: "T", Source: "mysite"},
		},
		{
			name: "override date",
			doc:  output.Doc{Frontmatter: output.Frontmatter{Title: "T", Date: &baseDate}},
			date: &overrideDate,
			want: output.Frontmatter{Title: "T", Date: &overrideDate},
		},
		{
			name: "override tags",
			doc:  output.Doc{Frontmatter: output.Frontmatter{Title: "T", Tags: []string{"old"}}},
			tags: []string{"new", "tags"},
			want: output.Frontmatter{Title: "T", Tags: []string{"new", "tags"}},
		},
		{
			name:   "override all fields",
			doc:    output.Doc{Frontmatter: output.Frontmatter{Title: "Old", Author: "Old", Date: &baseDate, Tags: []string{"old"}}},
			title:  "New Title",
			author: "New Author",
			source: "web",
			date:   &overrideDate,
			tags:   []string{"x", "y"},
			want:   output.Frontmatter{Title: "New Title", Author: "New Author", Source: "web", Date: &overrideDate, Tags: []string{"x", "y"}},
		},
		{
			name: "empty tags slice does not clear existing tags",
			doc:  output.Doc{Frontmatter: output.Frontmatter{Title: "T", Tags: []string{"keep"}}},
			tags: nil,
			want: output.Frontmatter{Title: "T", Tags: []string{"keep"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyURLOverrides(&tt.doc, tt.title, tt.author, tt.source, tt.date, tt.tags)
			assert.Equal(t, tt.want, tt.doc.Frontmatter)
		})
	}
}

func TestRunURL_MultiURLWithSingleOnlyFlags(t *testing.T) {
	tests := []struct {
		name      string
		title     string
		author    string
		date      string
		wantError string
	}{
		{
			name:      "title with multiple URLs",
			title:     "Override",
			wantError: "--title, --author, and --date cannot be used with multiple URLs",
		},
		{
			name:      "author with multiple URLs",
			author:    "Override",
			wantError: "--title, --author, and --date cannot be used with multiple URLs",
		},
		{
			name:      "date with multiple URLs",
			date:      "2024-01-01",
			wantError: "--title, --author, and --date cannot be used with multiple URLs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore global flag state.
			origTitle, origAuthor, origDate := urlTitle, urlAuthor, urlDate
			defer func() { urlTitle, urlAuthor, urlDate = origTitle, origAuthor, origDate }()

			urlTitle = tt.title
			urlAuthor = tt.author
			urlDate = tt.date

			err := runURL(nil, []string{"https://example.com/1", "https://example.com/2"})
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantError)
		})
	}
}
