package instapaper

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookmarkUnmarshal(t *testing.T) {
	raw := `{
		"bookmarks": [
			{"bookmark_id":42,"title":"Test Article","url":"https://example.com","time":1709294400,"description":"A test","starred":"0","tags":[]},
			{"bookmark_id":99,"title":"Tagged","url":"https://tagged.com","time":1709380800,"description":"","starred":"1","tags":[{"id":1,"name":"tech"},{"id":2,"name":"go"}]}
		],
		"highlights": []
	}`

	var resp BookmarkListResponse
	require.NoError(t, json.Unmarshal([]byte(raw), &resp))

	require.Equal(t, []Bookmark{
		{ID: 42, Title: "Test Article", URL: "https://example.com", Time: 1709294400, Description: "A test", Starred: "0", Tags: []Tag{}},
		{ID: 99, Title: "Tagged", URL: "https://tagged.com", Time: 1709380800, Description: "", Starred: "1", Tags: []Tag{{ID: 1, Name: "tech"}, {ID: 2, Name: "go"}}},
	}, resp.Bookmarks)
}

func TestAPIError_Error(t *testing.T) {
	err := &APIError{StatusCode: 400, ErrorCode: ErrInvalidURL, Message: "bad url"}
	got := err.Error()
	assert.NotEmpty(t, got)
	assert.Contains(t, got, "400")
	assert.Contains(t, got, "1240")
}
