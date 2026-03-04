package instapaper

import "encoding/json"

const (
	// FolderIDUnread is the default folder - unread bookmarks.
	FolderIDUnread = "unread"
	// FolderIDArchive is a built-in folder for archived bookmarks.
	FolderIDArchive = "archive"
)

// Bookmark represents an Instapaper bookmark.
type Bookmark struct {
	ID          int     `json:"bookmark_id"`
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	Description string  `json:"description"`
	Time        float64 `json:"time"`
	Starred     string  `json:"starred"`
	Tags        []Tag   `json:"tags"`
}

// Tag represents a tag on a bookmark.
type Tag struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// BookmarkListResponse is the response from the bookmarks/list endpoint.
type BookmarkListResponse struct {
	Bookmarks   []Bookmark  `json:"bookmarks"`
	Highlights  []Highlight `json:"highlights"`
	RawResponse string
}

// BookmarkListParams defines filtering options for ListBookmarks.
type BookmarkListParams struct {
	Limit  int
	Skip   []Bookmark
	Folder string
}

// DefaultBookmarkListParams provides sane defaults.
var DefaultBookmarkListParams = BookmarkListParams{
	Limit:  500,
	Folder: FolderIDUnread,
}

// Highlight represents a highlight within a bookmark.
type Highlight struct {
	ID         int         `json:"highlight_id"`
	BookmarkID int         `json:"bookmark_id"`
	Text       string      `json:"text"`
	Time       json.Number `json:"time"`
}
