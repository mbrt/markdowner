package instapaper

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBookmarkUnmarshal(t *testing.T) {
	raw := `[
		{"type":"meta"},
		{"type":"user","user_id":123,"username":"test@example.com"},
		{"type":"bookmark","bookmark_id":42,"title":"Test Article","url":"https://example.com","time":1709294400,"description":"A test","starred":"0","tags":[]},
		{"type":"bookmark","bookmark_id":99,"title":"Tagged","url":"https://tagged.com","time":1709380800,"description":"","starred":"1","tags":[{"id":1,"name":"tech"},{"id":2,"name":"go"}]}
	]`

	var items []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}

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

	if len(bookmarks) != 2 {
		t.Fatalf("expected 2 bookmarks, got %d", len(bookmarks))
	}

	b := bookmarks[0]
	if b.ID != 42 {
		t.Errorf("bookmark[0].ID = %d, want 42", b.ID)
	}
	if b.Title != "Test Article" {
		t.Errorf("bookmark[0].Title = %q, want %q", b.Title, "Test Article")
	}
	if b.URL != "https://example.com" {
		t.Errorf("bookmark[0].URL = %q, want %q", b.URL, "https://example.com")
	}

	b2 := bookmarks[1]
	if len(b2.Tags) != 2 {
		t.Fatalf("bookmark[1] expected 2 tags, got %d", len(b2.Tags))
	}
	if b2.Tags[0].Name != "tech" {
		t.Errorf("bookmark[1].Tags[0].Name = %q, want %q", b2.Tags[0].Name, "tech")
	}
}

func TestAPIError_Error(t *testing.T) {
	err := &APIError{StatusCode: 400, ErrorCode: ErrInvalidURL, Message: "bad url"}
	got := err.Error()
	if got == "" {
		t.Error("APIError.Error() returned empty string")
	}
	// Should contain the status code and error code.
	if !strings.Contains(got, "400") || !strings.Contains(got, "1240") {
		t.Errorf("APIError.Error() = %q, want to contain status and error code", got)
	}
}
