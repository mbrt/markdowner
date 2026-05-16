package fetch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFirstNWords(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{"one two three four five six", 5, "one two three four five"},
		{"hello world", 5, "hello world"},
		{"", 5, ""},
		{"   spaced   out   words   ", 3, "spaced out words"},
		{"single", 1, "single"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, firstNWords(tc.s, tc.n))
	}
}

func TestIsXArticleURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://x.com/LangChain/article/2033959303766512006", true},
		{"https://www.x.com/SomeUser/article/1234567890", true},
		{"http://x.com/user/article/999", true},
		// Not article paths
		{"https://x.com/LangChain/status/2033959303766512006", false},
		{"https://x.com/LangChain", false},
		{"https://x.com/LangChain/article", false},
		// Wrong host
		{"https://twitter.com/user/article/123", false},
		{"https://example.com/user/article/123", false},
		// Invalid URL
		{"not a url", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			assert.Equal(t, tc.want, isXArticleURL(tc.url))
		})
	}
}

func TestIsXTweetURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		// Standard tweet URLs.
		{"https://x.com/user/status/1234567890", true},
		{"https://x.com/i/status/2043009647892926712", true},
		{"https://www.x.com/user/status/999", true},
		// Twitter.com variants.
		{"https://twitter.com/user/status/1234567890", true},
		{"https://www.twitter.com/user/status/1234567890", true},
		{"http://twitter.com/user/status/999", true},
		// Trailing segments (e.g. /photo/1).
		{"https://x.com/user/status/123/photo/1", true},
		// Not tweet paths.
		{"https://x.com/user/article/123", false},
		{"https://x.com/user", false},
		{"https://x.com/user/status", false},
		// Wrong host.
		{"https://example.com/user/status/123", false},
		{"https://nitter.net/user/status/123", false},
		// Invalid URL.
		{"not a url", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			assert.Equal(t, tc.want, isXTweetURL(tc.url))
		})
	}
}

func TestTweetID(t *testing.T) {
	tests := []struct {
		url     string
		wantID  string
		wantErr bool
	}{
		{"https://x.com/user/status/123456", "123456", false},
		{"https://x.com/i/status/2043009647892926712", "2043009647892926712", false},
		{"https://x.com/user/status/123/photo/1", "123", false},
		{"https://x.com/user/article/123", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			id, err := tweetID(tc.url)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantID, id)
			}
		})
	}
}

const fxTweetJSON = `{
	"code": 200,
	"message": "OK",
	"tweet": {
		"text": "This is a test tweet with a link https://example.com",
		"created_timestamp": 1712000000,
		"author": {
			"name": "Test User",
			"screen_name": "testuser"
		},
		"media": {
			"all": [
				{"type": "photo", "url": "https://pbs.twimg.com/media/test.jpg"}
			]
		}
	}
}`

const fxTweetJSONNoMedia = `{
	"code": 200,
	"message": "OK",
	"tweet": {
		"text": "Just a plain text tweet.\nWith a newline.",
		"created_timestamp": 1712000000,
		"author": {
			"name": "Plain User",
			"screen_name": "plainuser"
		}
	}
}`

const fxTweetJSONWithArticle = `{
	"code": 200,
	"message": "OK",
	"tweet": {
		"text": "",
		"created_timestamp": 1712000000,
		"author": {
			"name": "Dan Woods",
			"screen_name": "danveloper"
		},
		"article": {
			"id": "2042676487711584257",
			"title": "Small Models are Smart Enough",
			"content": {
				"blocks": [
					{
						"type": "unstyled",
						"text": "Hello world, this is bold and italic text.",
						"inlineStyleRanges": [
							{"offset": 21, "length": 4, "style": "Bold"},
							{"offset": 30, "length": 6, "style": "Italic"}
						],
						"entityRanges": []
					},
					{
						"type": "unstyled",
						"text": "Visit example.com for more.",
						"inlineStyleRanges": [],
						"entityRanges": [
							{"offset": 6, "length": 11, "key": 0}
						]
					},
					{
						"type": "header-two",
						"text": "A Section Header",
						"inlineStyleRanges": [],
						"entityRanges": []
					},
					{
						"type": "unordered-list-item",
						"text": "First item",
						"inlineStyleRanges": [],
						"entityRanges": []
					},
					{
						"type": "unordered-list-item",
						"text": "Second item",
						"inlineStyleRanges": [],
						"entityRanges": []
					}
				],
				"entityMap": [
					{
						"key": "0",
						"value": {
							"type": "LINK",
							"mutability": "Mutable",
							"data": {"url": "https://example.com"}
						}
					}
				]
			}
		}
	}
}`

func TestHTMLFromXTweet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/i/status/111":
			fmt.Fprint(w, fxTweetJSON)
		case "/i/status/222":
			fmt.Fprint(w, fxTweetJSONNoMedia)
		case "/i/status/333":
			fmt.Fprint(w, fxTweetJSONWithArticle)
		case "/i/status/404":
			http.Error(w, `{"code":404,"message":"not found"}`, http.StatusNotFound)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	origBase := fxTwitterAPIBase
	fxTwitterAPIBase = srv.URL
	t.Cleanup(func() { fxTwitterAPIBase = origBase })

	t.Run("tweet with media", func(t *testing.T) {
		html, err := htmlFromXTweet(context.Background(), "https://x.com/testuser/status/111")
		require.NoError(t, err)
		assert.Contains(t, html, "This is a test tweet")
		assert.Contains(t, html, "@testuser")
		assert.Contains(t, html, "Test User (@testuser)")
		assert.Contains(t, html, "https://pbs.twimg.com/media/test.jpg")
	})

	t.Run("tweet without media", func(t *testing.T) {
		html, err := htmlFromXTweet(context.Background(), "https://x.com/i/status/222")
		require.NoError(t, err)
		assert.Contains(t, html, "Just a plain text tweet.")
		assert.Contains(t, html, "<br>")
		assert.Contains(t, html, "@plainuser")
	})

	t.Run("tweet not found", func(t *testing.T) {
		_, err := htmlFromXTweet(context.Background(), "https://x.com/user/status/404")
		assert.ErrorContains(t, err, "HTTP 404")
	})

	t.Run("tweet with embedded article renders article content", func(t *testing.T) {
		h, err := htmlFromXTweet(context.Background(), "https://x.com/i/status/333")
		require.NoError(t, err)
		// Title and author are set.
		assert.Contains(t, h, "Small Models are Smart Enough")
		assert.Contains(t, h, "Dan Woods (@danveloper)")
		// Inline styles are applied.
		assert.Contains(t, h, "<strong>bold</strong>")
		assert.Contains(t, h, "<em>italic</em>")
		// Links from entityMap are rendered.
		assert.Contains(t, h, `<a href="https://example.com">example.com</a>`)
		// Non-paragraph block types are rendered.
		assert.Contains(t, h, "<h2>A Section Header</h2>")
		// List items are wrapped in <ul>.
		assert.Contains(t, h, "<ul>")
		assert.Contains(t, h, "<li>First item</li>")
		assert.Contains(t, h, "<li>Second item</li>")
	})
}

func TestTweetToHTML(t *testing.T) {
	tweet := fxTweet{
		Text:             "Hello <world> & friends",
		CreatedTimestamp: 1712000000,
		Author: fxAuthor{
			Name:       "Test \"User\"",
			ScreenName: "testuser",
		},
		Media: &fxMedia{
			All: []fxMediaItem{
				{Type: "photo", URL: "https://example.com/photo.jpg"},
				{Type: "video", URL: "https://example.com/video.mp4"},
			},
		},
	}

	html := tweetToHTML(tweet)
	// Title includes username and first words of the tweet.
	assert.Contains(t, html, `<title>@testuser - Hello &lt;world&gt; &amp; friends</title>`)
	// HTML entities are escaped.
	assert.Contains(t, html, "Hello &lt;world&gt; &amp; friends")
	assert.Contains(t, html, `content="Test &#34;User&#34; (@testuser)"`)
	// Media is included.
	assert.Contains(t, html, `<img src="https://example.com/photo.jpg"`)
	assert.Contains(t, html, `<a href="https://example.com/video.mp4">Video</a>`)
}

func TestDraftBlockContent(t *testing.T) {
	entityURLs := map[int]string{
		0: "https://example.com",
		1: "https://other.com",
	}

	tests := []struct {
		name string
		b    fxBlock
		want string
	}{
		{
			name: "plain text",
			b:    fxBlock{Text: "hello world"},
			want: "hello world",
		},
		{
			name: "HTML special chars are escaped",
			b:    fxBlock{Text: "a < b & c > d"},
			want: "a &lt; b &amp; c &gt; d",
		},
		{
			name: "bold style",
			b: fxBlock{
				Text:              "say hello there",
				InlineStyleRanges: []fxStyleRange{{Offset: 4, Length: 5, Style: "Bold"}},
			},
			want: "say <strong>hello</strong> there",
		},
		{
			name: "italic style",
			b: fxBlock{
				Text:              "say hello there",
				InlineStyleRanges: []fxStyleRange{{Offset: 4, Length: 5, Style: "Italic"}},
			},
			want: "say <em>hello</em> there",
		},
		{
			name: "overlapping bold and italic",
			b: fxBlock{
				Text: "abcde",
				InlineStyleRanges: []fxStyleRange{
					{Offset: 0, Length: 3, Style: "Bold"},
					{Offset: 1, Length: 3, Style: "Italic"},
				},
			},
			// [0,1): Bold only → <strong>a</strong>
			// [1,3): Bold+Italic → <em><strong>bc</strong></em>
			// [3,4): Italic only → <em>d</em>
			// [4,5): no style → e
			want: "<strong>a</strong><em><strong>bc</strong></em><em>d</em>e",
		},
		{
			name: "entity link",
			b: fxBlock{
				Text:         "visit example.com now",
				EntityRanges: []fxEntityRange{{Offset: 6, Length: 11, Key: 0}},
			},
			want: `visit <a href="https://example.com">example.com</a> now`,
		},
		{
			name: "bold inside link",
			b: fxBlock{
				Text:              "click here please",
				InlineStyleRanges: []fxStyleRange{{Offset: 6, Length: 4, Style: "Bold"}},
				EntityRanges:      []fxEntityRange{{Offset: 6, Length: 4, Key: 0}},
			},
			want: `click <a href="https://example.com"><strong>here</strong></a> please`,
		},
		{
			name: "unicode offsets",
			b: fxBlock{
				Text:              "café latte",
				InlineStyleRanges: []fxStyleRange{{Offset: 5, Length: 5, Style: "Bold"}},
			},
			want: "café <strong>latte</strong>",
		},
		{
			name: "empty text",
			b:    fxBlock{Text: ""},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, draftBlockContent(tc.b, entityURLs))
		})
	}
}

func TestRenderDraftBlocks(t *testing.T) {
	entityURLs := map[int]string{}

	t.Run("unordered list items are wrapped in ul", func(t *testing.T) {
		blocks := []fxBlock{
			{Type: "unstyled", Text: "intro"},
			{Type: "unordered-list-item", Text: "one"},
			{Type: "unordered-list-item", Text: "two"},
			{Type: "unstyled", Text: "outro"},
		}
		var sb strings.Builder
		renderDraftBlocks(&sb, blocks, entityURLs)
		got := sb.String()
		assert.Contains(t, got, "<ul>\n<li>one</li>\n<li>two</li>\n</ul>")
		assert.Contains(t, got, "<p>intro</p>")
		assert.Contains(t, got, "<p>outro</p>")
	})

	t.Run("ordered list items are wrapped in ol", func(t *testing.T) {
		blocks := []fxBlock{
			{Type: "ordered-list-item", Text: "first"},
			{Type: "ordered-list-item", Text: "second"},
		}
		var sb strings.Builder
		renderDraftBlocks(&sb, blocks, entityURLs)
		got := sb.String()
		assert.Contains(t, got, "<ol>\n<li>first</li>\n<li>second</li>\n</ol>")
	})

	t.Run("empty unstyled block renders as br", func(t *testing.T) {
		blocks := []fxBlock{
			{Type: "unstyled", Text: "before"},
			{Type: "unstyled", Text: ""},
			{Type: "unstyled", Text: "after"},
		}
		var sb strings.Builder
		renderDraftBlocks(&sb, blocks, entityURLs)
		got := sb.String()
		assert.Contains(t, got, "<br>")
		assert.Contains(t, got, "<p>before</p>")
		assert.Contains(t, got, "<p>after</p>")
	})

	t.Run("header blocks use correct tags", func(t *testing.T) {
		blocks := []fxBlock{
			{Type: "header-one", Text: "H1"},
			{Type: "header-two", Text: "H2"},
			{Type: "header-three", Text: "H3"},
			{Type: "blockquote", Text: "quote"},
			{Type: "code-block", Text: "code()"},
		}
		var sb strings.Builder
		renderDraftBlocks(&sb, blocks, entityURLs)
		got := sb.String()
		assert.Contains(t, got, "<h1>H1</h1>")
		assert.Contains(t, got, "<h2>H2</h2>")
		assert.Contains(t, got, "<h3>H3</h3>")
		assert.Contains(t, got, "<blockquote>quote</blockquote>")
		assert.Contains(t, got, "<pre>code()</pre>")
	})
}

func TestHTML_UsesChromiumForXArticleURL(t *testing.T) {
	if !chromeAvailable() {
		t.Skip("Chrome/Chromium not found")
	}

	// Serve HTML that mimics the X article DOM: the article container uses
	// data-testid="twitterArticleReadView" (not a semantic <article> element).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<html><head><title>X Article</title></head><body>`+
			`<div data-testid="twitterArticleReadView"><h1>X Article</h1><p>Content here</p></div>`+
			`</body></html>`)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	html, err := htmlFromXArticle(ctx, srv.URL)
	require.NoError(t, err)
	assert.Contains(t, html, "X Article")
}

func TestHTML_DispatchesToTweetFetcher(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, fxTweetJSON)
	}))
	t.Cleanup(srv.Close)

	origBase := fxTwitterAPIBase
	fxTwitterAPIBase = srv.URL
	t.Cleanup(func() { fxTwitterAPIBase = origBase })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	html, err := Client{RetryBackoff: time.Nanosecond}.HTML(ctx, "https://x.com/user/status/111")
	require.NoError(t, err)
	assert.Contains(t, html, "This is a test tweet")
	assert.Contains(t, html, "@testuser")
}
