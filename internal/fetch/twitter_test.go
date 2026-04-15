package fetch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestHTMLFromXTweet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/i/status/111":
			fmt.Fprint(w, fxTweetJSON)
		case "/i/status/222":
			fmt.Fprint(w, fxTweetJSONNoMedia)
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
