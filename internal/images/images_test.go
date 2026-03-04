package images

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runPlugin is a test helper that runs html through the converter with the
// image-downloader plugin and returns the resulting markdown and downloaded
// images map.
func runPlugin(t *testing.T, html string) (string, map[string][]byte) {
	t.Helper()
	ctx := context.Background()
	results := map[string][]byte{}
	plugin := NewPlugin(ctx, results)
	conv := converter.NewConverter(
		converter.WithPlugins(
			base.NewBasePlugin(),
			commonmark.NewCommonmarkPlugin(),
			plugin,
		),
	)
	md, err := conv.ConvertString(html, converter.WithContext(ctx))
	require.NoError(t, err)
	return md, results
}

func TestPlugin_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		fmt.Fprint(w, "fakepng")
	}))
	defer srv.Close()

	html := fmt.Sprintf(`<p>Before</p><img src="%s/photo.png" alt="an image"><p>After</p>`, srv.URL)

	gotMD, imgs := runPlugin(t, html)

	assert.Contains(t, gotMD, "![an image](img/")
	assert.Contains(t, gotMD, ".png)")
	assert.NotContains(t, gotMD, srv.URL)

	require.Len(t, imgs, 1)
	for k, v := range imgs {
		assert.Contains(t, k, "img/")
		assert.Contains(t, k, ".png")
		assert.Equal(t, []byte("fakepng"), v)
	}
}

func TestPlugin_FailedDownloadKeepsOriginal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	imgURL := srv.URL + "/missing.jpg"
	html := fmt.Sprintf(`<img src="%s" alt="img">`, imgURL)

	gotMD, imgs := runPlugin(t, html)

	// The commonmark renderer takes over and emits the original URL.
	assert.Contains(t, gotMD, imgURL)
	assert.Empty(t, imgs)
}

func TestPlugin_NonHTTPURLUntouched(t *testing.T) {
	html := `<img src="./local.png" alt="local"><img src="data:image/png;base64,abc" alt="data">`

	gotMD, imgs := runPlugin(t, html)

	assert.Contains(t, gotMD, "./local.png")
	assert.Contains(t, gotMD, "data:image/png;base64,abc")
	assert.Empty(t, imgs)
}

func TestPlugin_DuplicateURLDownloadedOnce(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "image/gif")
		fmt.Fprint(w, "fakegif")
	}))
	defer srv.Close()

	imgURL := fmt.Sprintf("%s/anim.gif", srv.URL)
	html := fmt.Sprintf(`<img src="%s" alt="a"><img src="%s" alt="b">`, imgURL, imgURL)

	gotMD, imgs := runPlugin(t, html)

	assert.Equal(t, 1, calls, "image should only be downloaded once")
	require.Len(t, imgs, 1)

	var localRef string
	for k := range imgs {
		localRef = k
	}
	assert.Contains(t, gotMD, fmt.Sprintf("![a](%s)", localRef))
	assert.Contains(t, gotMD, fmt.Sprintf("![b](%s)", localRef))
}

func TestPlugin_ExtensionFromContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/webp")
		fmt.Fprint(w, "fakewebp")
	}))
	defer srv.Close()

	// URL has no recognisable extension.
	html := fmt.Sprintf(`<img src="%s/image" alt="img">`, srv.URL)

	gotMD, imgs := runPlugin(t, html)

	assert.Contains(t, gotMD, ".webp)")
	require.Len(t, imgs, 1)
	for k := range imgs {
		assert.Contains(t, k, ".webp")
	}
}

func TestPlugin_NoImages(t *testing.T) {
	html := `<p>Just some text without any images.</p>`

	_, imgs := runPlugin(t, html)

	assert.Empty(t, imgs)
}
