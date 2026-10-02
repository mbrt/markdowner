package instapaper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mbrt/markdowner/internal/output"
)

const testToken = "test-token"

// fakeAPI emulates the subset of the Instapaper v2 API used by the client.
// Bookmarks in each section must be sorted newest first, like the real API.
type fakeAPI struct {
	sections map[string][]Bookmark
	bodies   map[int]string
}

func (f fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+testToken {
		writeError(w, http.StatusUnauthorized, "Authentication failed")
		return
	}

	if r.URL.Path == "/bookmarks" {
		q := r.URL.Query()
		section := q.Get("section")
		limit, _ := strconv.Atoi(q.Get("limit"))
		offset, _ := strconv.Atoi(q.Get("offset"))
		all := f.sections[section]
		page := all[min(offset, len(all)):min(offset+limit, len(all))]
		writeJSON(w, BookmarkListResponse{Bookmarks: page, Total: len(all)})
		return
	}

	var id int
	if _, err := fmt.Sscanf(r.URL.Path, "/bookmarks/%d/parse", &id); err == nil {
		body, ok := f.bodies[id]
		if !ok {
			writeError(w, http.StatusInternalServerError, "Error generating text version of this URL")
			return
		}
		writeJSON(w, map[string]any{
			"metadata": map[string]any{},
			"content":  map[string]any{"body": body},
		})
		return
	}
	writeError(w, http.StatusNotFound, "Not found")
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"code": status, "message": msg},
	})
}

func newTestClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewClient(testToken)
	c.baseURL = srv.URL
	return c
}

func bookmarksAt(times ...int64) []Bookmark {
	var res []Bookmark
	for i, ts := range times {
		res = append(res, Bookmark{ID: i + 1, Title: fmt.Sprintf("B%d", i+1), Time: ts})
	}
	return res
}

func TestBookmarkUnmarshal(t *testing.T) {
	raw := `{
		"bookmarks": [
			{"id":42,"title":"Test Article","url":"https://example.com","time":1709294400,"description":"A test","author":null,"pubtime":null,"liked":false,"archived":false,"tags":[],"category":0},
			{"id":99,"title":null,"url":"https://tagged.com","time":1709380800,"description":null,"author":"Jane","pubtime":1709000000,"liked":true,"archived":true,"tags":[{"id":1,"name":"tech","slug":"tech","count":3},{"id":2,"name":"go","slug":"go","count":1}],"category":0}
		],
		"total": 2
	}`

	var resp BookmarkListResponse
	require.NoError(t, json.Unmarshal([]byte(raw), &resp))

	pubtime := int64(1709000000)
	assert.Equal(t, BookmarkListResponse{
		Bookmarks: []Bookmark{
			{ID: 42, Title: "Test Article", URL: "https://example.com", Time: 1709294400, Description: "A test", Tags: []Tag{}},
			{ID: 99, URL: "https://tagged.com", Time: 1709380800, Author: "Jane", Pubtime: &pubtime, Liked: true, Tags: []Tag{{ID: 1, Name: "tech"}, {ID: 2, Name: "go"}}},
		},
		Total: 2,
	}, resp)
}

func TestClientErrors(t *testing.T) {
	api := fakeAPI{bodies: map[int]string{}}

	tests := []struct {
		name    string
		token   string
		call    func(*Client) error
		wantErr string
	}{
		{
			name:  "bad token",
			token: "wrong",
			call: func(c *Client) error {
				_, err := c.ListBookmarks(context.Background(), DefaultBookmarkListParams)
				return err
			},
			wantErr: "instapaper API: status 401: Authentication failed",
		},
		{
			name:  "parse failure",
			token: testToken,
			call: func(c *Client) error {
				_, err := c.GetText(context.Background(), 7)
				return err
			},
			wantErr: "instapaper API: status 500: Error generating text version of this URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, api)
			c.token = tt.token
			err := tt.call(c)
			assert.EqualError(t, err, tt.wantErr)
			var apiErr *APIError
			assert.ErrorAs(t, err, &apiErr)
		})
	}
}

func TestAPIErrorNonJSONBody(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	_, err := c.ListBookmarks(context.Background(), DefaultBookmarkListParams)
	assert.EqualError(t, err, "instapaper API: status 502: bad gateway\n")
}

func TestListSection(t *testing.T) {
	// Use a tiny page size to exercise pagination.
	orig := DefaultBookmarkListParams
	DefaultBookmarkListParams.Limit = 2
	t.Cleanup(func() { DefaultBookmarkListParams = orig })

	home := bookmarksAt(500, 400, 300, 200, 100)
	c := newTestClient(t, fakeAPI{sections: map[string][]Bookmark{SectionHome: home}})

	tests := []struct {
		name  string
		since time.Time
		want  []Bookmark
	}{
		{name: "all pages", want: home},
		{name: "cutoff mid page", since: time.Unix(250, 0), want: home[:3]},
		{name: "cutoff at page boundary", since: time.Unix(350, 0), want: home[:2]},
		{name: "nothing new", since: time.Unix(1000, 0), want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := listSection(context.Background(), c, SectionHome, tt.since)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFetchDocs(t *testing.T) {
	// The article pages are served by the same test server, so that the
	// metadata merge in bookmarkToDoc doesn't hit the network.
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	pubtime := int64(1700000000)
	api := fakeAPI{
		sections: map[string][]Bookmark{
			SectionHome: {{
				ID: 1, Title: "Home Article", URL: srv.URL + "/article/1", Time: 1709380800,
				Author: "Jane", Pubtime: &pubtime, Tags: []Tag{{ID: 1, Name: "tech"}},
			}},
			SectionArchive: {{
				ID: 2, Title: "Archived Article", URL: srv.URL + "/article/2", Time: 1709294400,
			}},
		},
		bodies: map[int]string{
			1: "<p>Parsed by Instapaper.</p>",
			2: "<p>Also parsed.</p>",
		},
	}
	mux.Handle("/api/2/", http.StripPrefix("/api/2", api))
	mux.HandleFunc("/article/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "<html><head><title>Page</title></head><body><p>Raw page.</p></body></html>")
	})

	c := NewClient(testToken)
	c.baseURL = srv.URL + "/api/2"
	f := Fetcher{Client: c, Parallel: 2}

	var docs []output.Doc
	for res := range f.FetchDocs(context.Background(), time.Time{}) {
		require.NoError(t, res.Err)
		docs = append(docs, res.Doc)
	}
	slices.SortFunc(docs, func(a, b output.Doc) int { return strings.Compare(a.Frontmatter.URL, b.Frontmatter.URL) })

	published := time.Unix(pubtime, 0).UTC()
	require.Len(t, docs, 2)
	assert.Equal(t, output.Frontmatter{
		Title:  "Home Article",
		Author: "Jane",
		URL:    srv.URL + "/article/1",
		Source: "instapaper",
		Date:   &published,
		Saved:  time.Unix(1709380800, 0).UTC(),
		Tags:   []string{"tech"},
	}, docs[0].Frontmatter)
	assert.Contains(t, docs[0].Markdown, "Parsed by Instapaper.")
	assert.Equal(t, "Archived Article", docs[1].Frontmatter.Title)
	assert.Contains(t, docs[1].Markdown, "Also parsed.")
}
