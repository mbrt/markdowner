package instapaper

const (
	// SectionHome is the default section - unread bookmarks.
	SectionHome = "home"
	// SectionArchive is the section of archived bookmarks.
	SectionArchive = "archive"
)

// Bookmark represents an Instapaper bookmark.
type Bookmark struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Author      string `json:"author"`
	// Time is when the bookmark was saved, as a Unix timestamp.
	Time int64 `json:"time"`
	// Pubtime is when the article was published, as a Unix timestamp.
	Pubtime *int64 `json:"pubtime"`
	Liked   bool   `json:"liked"`
	Tags    []Tag  `json:"tags"`
}

// Tag represents a tag on a bookmark.
type Tag struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// BookmarkListResponse is the response from the list bookmarks endpoint.
type BookmarkListResponse struct {
	Bookmarks []Bookmark `json:"bookmarks"`
	Total     int        `json:"total"`
}

// BookmarkListParams defines filtering options for ListBookmarks.
type BookmarkListParams struct {
	Section string
	Limit   int
	Offset  int
}

// DefaultBookmarkListParams provides sane defaults. 500 is the maximum page
// size allowed by the API.
var DefaultBookmarkListParams = BookmarkListParams{
	Section: SectionHome,
	Limit:   500,
}

// ParsedBookmark is the response from the parse bookmark endpoint.
type ParsedBookmark struct {
	Content struct {
		Body string `json:"body"`
	} `json:"content"`
}
