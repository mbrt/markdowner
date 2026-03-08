package fetch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// chromeAvailable reports whether a Chrome/Chromium binary can be found on
// PATH. Tests that require a real browser are skipped when Chrome is absent.
func chromeAvailable() bool {
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}

func TestHtmlWithBrowser_FetchesHTML(t *testing.T) {
	if !chromeAvailable() {
		t.Skip("Chrome/Chromium not found")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, testHTML)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	html, err := htmlWithBrowser(ctx, srv.URL)
	require.NoError(t, err)
	assert.Contains(t, html, "Test Page")
}

func TestHTML_FallsBackToBrowserOnCFChallenge(t *testing.T) {
	if !chromeAvailable() {
		t.Skip("Chrome/Chromium not found")
	}

	// The first request (from Go's HTTP client) returns a Cloudflare-style
	// 403 challenge. All subsequent requests (from Chrome) receive real HTML.
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if callCount.Add(1) == 1 {
			w.Header().Set("cf-mitigated", "challenge")
			http.Error(w, "CF challenge", http.StatusForbidden)
			return
		}
		fmt.Fprint(w, testHTML)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	html, err := Client{}.HTML(ctx, srv.URL)
	require.NoError(t, err)
	assert.Contains(t, html, "Test Page")
}
