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
