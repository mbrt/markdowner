package images

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRewrite_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		fmt.Fprint(w, "fakepng")
	}))
	defer srv.Close()

	input := fmt.Sprintf("Before\n\n![an image](%s/photo.png)\n\nAfter", srv.URL)

	gotMD, imgs, err := Rewrite(context.Background(), input)
	require.NoError(t, err)

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

func TestRewrite_FailedDownloadKeepsOriginal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	imgURL := srv.URL + "/missing.jpg"
	input := fmt.Sprintf("![img](%s)", imgURL)

	gotMD, imgs, err := Rewrite(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, input, gotMD)
	assert.Empty(t, imgs)
}

func TestRewrite_NonHTTPURLUntouched(t *testing.T) {
	input := "![local](./local.png) and ![data](data:image/png;base64,abc)"

	gotMD, imgs, err := Rewrite(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, input, gotMD)
	assert.Empty(t, imgs)
}

func TestRewrite_DuplicateURLDownloadedOnce(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "image/gif")
		fmt.Fprint(w, "fakegif")
	}))
	defer srv.Close()

	imgURL := fmt.Sprintf("%s/anim.gif", srv.URL)
	input := fmt.Sprintf("![a](%s) and ![b](%s)", imgURL, imgURL)

	gotMD, imgs, err := Rewrite(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, 1, calls, "image should only be downloaded once")

	// Both references should point to the same local path.
	require.Len(t, imgs, 1)
	var localRef string
	for k := range imgs {
		localRef = k
	}
	assert.Equal(t, fmt.Sprintf("![a](%s) and ![b](%s)", localRef, localRef), gotMD)
}

func TestRewrite_ExtensionFromContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/webp")
		fmt.Fprint(w, "fakewebp")
	}))
	defer srv.Close()

	// URL has no recognisable extension.
	input := fmt.Sprintf("![img](%s/image)", srv.URL)

	gotMD, imgs, err := Rewrite(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, gotMD, ".webp)")
	require.Len(t, imgs, 1)
	for k := range imgs {
		assert.Contains(t, k, ".webp")
	}
}

func TestRewrite_NoImages(t *testing.T) {
	input := "Just some text without any images."

	gotMD, imgs, err := Rewrite(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, input, gotMD)
	assert.Empty(t, imgs)
}
