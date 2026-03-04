package instapaper

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookmarkUnmarshal(t *testing.T) {
	raw := `[
		{"type":"meta"},
		{"type":"user","user_id":123,"username":"test@example.com"},
		{"type":"bookmark","bookmark_id":42,"title":"Test Article","url":"https://example.com","time":1709294400,"description":"A test","starred":"0","tags":[]},
		{"type":"bookmark","bookmark_id":99,"title":"Tagged","url":"https://tagged.com","time":1709380800,"description":"","starred":"1","tags":[{"id":1,"name":"tech"},{"id":2,"name":"go"}]}
	]`

	var items []json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &items))

	var bookmarks []Bookmark
	for _, item := range items {
		var typed struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(item, &typed); err != nil {
			continue
		}
		if typed.Type == "bookmark" {
			var b Bookmark
			if err := json.Unmarshal(item, &b); err == nil {
				bookmarks = append(bookmarks, b)
			}
		}
	}

	require.Equal(t, []Bookmark{
		{ID: 42, Title: "Test Article", URL: "https://example.com", Time: 1709294400, Description: "A test", Starred: "0", Tags: []Tag{}},
		{ID: 99, Title: "Tagged", URL: "https://tagged.com", Time: 1709380800, Description: "", Starred: "1", Tags: []Tag{{ID: 1, Name: "tech"}, {ID: 2, Name: "go"}}},
	}, bookmarks)
}

func TestAPIError_Error(t *testing.T) {
	err := &APIError{StatusCode: 400, ErrorCode: ErrInvalidURL, Message: "bad url"}
	got := err.Error()
	assert.NotEmpty(t, got)
	assert.Contains(t, got, "400")
	assert.Contains(t, got, "1240")
}
