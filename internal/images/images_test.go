package images

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
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
	plugin := NewPlugin(ctx, results, 0)
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
	html := `<img src="./local.png" alt="local">`

	gotMD, imgs := runPlugin(t, html)

	assert.Contains(t, gotMD, "./local.png")
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

func TestPlugin_DataURIExtracted(t *testing.T) {
	// A small valid 1x1 PNG encoded as a data URI.
	const pngB64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	html := fmt.Sprintf(`<p>Before</p><img src="data:image/png;base64,%s" alt="tiny"><p>After</p>`, pngB64)

	gotMD, imgs := runPlugin(t, html)

	// The markdown must reference a local path, not the original data URI.
	assert.Contains(t, gotMD, "![tiny](img/")
	assert.Contains(t, gotMD, ".png)")
	assert.NotContains(t, gotMD, "data:image/png")

	require.Len(t, imgs, 1)
	for k := range imgs {
		assert.Contains(t, k, "img/")
		assert.Contains(t, k, ".png")
	}
}

func TestPlugin_DataURIDuplicateExtractedOnce(t *testing.T) {
	const pngB64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	dataURI := fmt.Sprintf("data:image/png;base64,%s", pngB64)
	html := fmt.Sprintf(`<img src="%s" alt="a"><img src="%s" alt="b">`, dataURI, dataURI)

	gotMD, imgs := runPlugin(t, html)

	require.Len(t, imgs, 1)
	var localRef string
	for k := range imgs {
		localRef = k
	}
	assert.Contains(t, gotMD, fmt.Sprintf("![a](%s)", localRef))
	assert.Contains(t, gotMD, fmt.Sprintf("![b](%s)", localRef))
}

func TestPlugin_DataURIInvalidFallsBack(t *testing.T) {
	// Malformed data URI — no comma separator.
	html := `<img src="data:image/png;base64" alt="bad">`

	gotMD, imgs := runPlugin(t, html)

	// Falls back to default rendering; the broken URI appears in the output.
	assert.Contains(t, gotMD, "data:image/png;base64")
	assert.Empty(t, imgs)
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantErr string
	}{
		{"500KB", 500_000, ""},
		{"2MB", 2_000_000, ""},
		{"1GB", 1_000_000_000, ""},
		{"500kb", 500_000, ""},
		{"1024", 1024, ""},
		{"1.5MB", 1_500_000, ""},
		{"", 0, "empty size string"},
		{"abc", 0, "parsing size"},
		{"-1KB", 0, "negative size"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseSize(tt.input)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// makePNG creates a PNG image of the given dimensions filled with the given color.
func makePNG(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func TestCompressToJPEG_UnderLimit(t *testing.T) {
	data := makePNG(t, 1, 1, color.White)
	got, ext, err := compressToJPEG(data, int64(len(data)+1000))
	require.NoError(t, err)
	assert.Equal(t, data, got, "data under limit should be unchanged")
	assert.Equal(t, "", ext, "extension should be empty when unchanged")
}

// makeLargePNG creates a large PNG with photo-like content that PNG can't
// compress well but JPEG can handle at lower qualities.
func makeLargePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// Use a pattern that looks noisy to PNG (poor compression) but has
	// enough spatial correlation for JPEG to compress at low quality.
	for y := range h {
		for x := range w {
			// Mix a smooth gradient with per-pixel variation.
			r := uint8((x*173 + y*89 + x*y/3) % 256)
			g := uint8((x*59 + y*131 + (x+y)*7) % 256)
			b := uint8((x*97 + y*41 + x*y/5) % 256)
			img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func TestCompressToJPEG_PNGOverLimit(t *testing.T) {
	data := makeLargePNG(t, 800, 800)
	maxSize := int64(100_000)

	got, ext, err := compressToJPEG(data, maxSize)
	require.NoError(t, err)
	assert.Equal(t, ".jpg", ext)
	assert.LessOrEqual(t, int64(len(got)), maxSize)

	// Verify it's valid JPEG.
	_, err = jpeg.Decode(bytes.NewReader(got))
	assert.NoError(t, err)
}

func TestCompressToJPEG_JPEGOverLimit(t *testing.T) {
	// Create a JPEG, then compress it further.
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := range 100 {
		for x := range 100 {
			img.Set(x, y, color.RGBA{R: uint8(x * 2), G: uint8(y * 2), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}))
	data := buf.Bytes()
	maxSize := int64(len(data) / 2)

	got, ext, err := compressToJPEG(data, maxSize)
	require.NoError(t, err)
	assert.Equal(t, ".jpg", ext)
	assert.LessOrEqual(t, int64(len(got)), maxSize)
}

func TestCompressToJPEG_GIFUnchanged(t *testing.T) {
	// Create a GIF image.
	img := image.NewPaletted(image.Rect(0, 0, 10, 10), color.Palette{color.White, color.Black})
	var buf bytes.Buffer
	require.NoError(t, gif.Encode(&buf, img, nil))
	data := buf.Bytes()

	got, ext, err := compressToJPEG(data, 1) // Very small limit.
	require.NoError(t, err)
	assert.Equal(t, data, got, "GIF should be unchanged")
	assert.Equal(t, "", ext)
}

func TestCompressToJPEG_UndecodableUnchanged(t *testing.T) {
	data := []byte("this is not an image format")
	got, ext, err := compressToJPEG(data, 1)
	require.NoError(t, err)
	assert.Equal(t, data, got)
	assert.Equal(t, "", ext)
}

func TestPlugin_OversizedImageCompressed(t *testing.T) {
	pngData := makeLargePNG(t, 800, 800)
	maxSize := int64(100_000)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngData)
	}))
	defer srv.Close()

	ctx := context.Background()
	results := map[string][]byte{}
	plugin := NewPlugin(ctx, results, maxSize)
	conv := converter.NewConverter(
		converter.WithPlugins(
			base.NewBasePlugin(),
			commonmark.NewCommonmarkPlugin(),
			plugin,
		),
	)
	html := fmt.Sprintf(`<img src="%s/photo.png" alt="big">`, srv.URL)
	md, err := conv.ConvertString(html, converter.WithContext(ctx))
	require.NoError(t, err)

	// Path should now have .jpg extension.
	assert.Contains(t, md, ".jpg)")
	assert.NotContains(t, md, ".png)")

	require.Len(t, results, 1)
	for k, v := range results {
		assert.True(t, strings.HasSuffix(k, ".jpg"))
		assert.LessOrEqual(t, int64(len(v)), maxSize)
	}
}
