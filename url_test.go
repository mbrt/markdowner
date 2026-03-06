package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
