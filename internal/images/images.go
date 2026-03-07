// Package images handles downloading external images referenced in HTML and
// rewriting the references to point to locally saved copies.
package images

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // register PNG decoder
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	_ "golang.org/x/image/webp" // register WebP decoder

	"golang.org/x/crypto/sha3"

	_ "image/gif" // register GIF decoder
)

func downloadImage(ctx context.Context, imgURL string) (string, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imgURL, nil)
	if err != nil {
		return "", nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}

	ext := extFromURL(imgURL)
	if ext == "" {
		ext = extFromContentType(resp.Header.Get("Content-Type"))
	}

	h := sha3.NewShake128()
	h.Write(data)
	hashBytes := make([]byte, 16)
	h.Read(hashBytes)
	hashHex := hex.EncodeToString(hashBytes)

	localPath := "img/" + hashHex + ext
	return localPath, data, nil
}

func extFromURL(imgURL string) string {
	u, err := url.Parse(imgURL)
	if err != nil {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(u.Path))
	// Sanity-check: extensions should be short and contain only letters.
	if len(ext) > 6 || strings.ContainsAny(ext, "?#/ ") {
		return ""
	}
	return ext
}

func extFromContentType(ct string) string {
	if ct == "" {
		return ""
	}
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return ""
	}
	switch mt {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/svg+xml":
		return ".svg"
	case "image/avif":
		return ".avif"
	default:
		return ""
	}
}

// ParseSize parses a human-readable size string (e.g. "500KB", "2MB", "1GB")
// into bytes. Plain numbers without a suffix are treated as bytes.
// Uses base-10 units: 1KB = 1000 bytes, 1MB = 1,000,000 bytes.
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size string")
	}

	suffix := strings.ToUpper(s)
	multiplier := int64(1)
	for _, u := range []struct {
		suffix string
		mult   int64
	}{
		{"GB", 1_000_000_000},
		{"MB", 1_000_000},
		{"KB", 1_000},
	} {
		if strings.HasSuffix(suffix, u.suffix) {
			s = strings.TrimSpace(s[:len(s)-len(u.suffix)])
			multiplier = u.mult
			break
		}
	}

	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing size %q: %w", s, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("negative size: %s", s)
	}
	return int64(n * float64(multiplier)), nil
}

// compressToJPEG compresses image data to fit within maxSize bytes by
// re-encoding as JPEG at progressively lower quality. Returns the
// (possibly compressed) data, a new extension (".jpg" if converted, or ""
// if unchanged), and any error.
//
// Images already under the limit, GIFs (to preserve animation), and
// undecodable formats are returned unchanged.
func compressToJPEG(data []byte, maxSize int64) ([]byte, string, error) {
	if int64(len(data)) <= maxSize {
		return data, "", nil
	}

	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		// Can't decode (SVG, AVIF, unknown) — keep as-is.
		return data, "", nil
	}
	if format == "gif" {
		// Preserve animated GIFs.
		return data, "", nil
	}

	// Binary search for highest quality that fits.
	lo, hi := 1, 100
	var best []byte
	for lo <= hi {
		mid := (lo + hi) / 2
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: mid}); err != nil {
			return nil, "", fmt.Errorf("encoding JPEG at quality %d: %w", mid, err)
		}
		if int64(buf.Len()) <= maxSize {
			best = buf.Bytes()
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}

	if best == nil {
		// Even quality=1 exceeds the limit; use it as a last resort.
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 1}); err != nil {
			return nil, "", fmt.Errorf("encoding JPEG at quality 1: %w", err)
		}
		best = buf.Bytes()
	}
	return best, ".jpg", nil
}

// decodeDataURI parses a data URI of the form "data:<mediatype>;base64,<data>"
// and returns the file extension (e.g. ".png") and the decoded bytes.
func decodeDataURI(src string) (string, []byte, error) {
	src, ok := strings.CutPrefix(src, "data:")
	if !ok {
		return "", nil, fmt.Errorf("not a data URI")
	}
	// Split on the first comma to separate the header from the payload.
	header, payload, ok := strings.Cut(src, ",")
	if !ok {
		return "", nil, fmt.Errorf("malformed data URI: missing comma")
	}
	// Header must end with ";base64".
	mediaType, ok := strings.CutSuffix(header, ";base64")
	if !ok {
		return "", nil, fmt.Errorf("unsupported data URI encoding (expected base64)")
	}
	// Strip any MIME line-folding whitespace (newlines, spaces) from the payload
	// before decoding. HTML parsers may normalize line breaks to spaces, and
	// many base64 encoders insert a newline every 76 characters.
	// Additionally, some HTML parsers percent-encode newlines as %0A; decode
	// those first, then strip whitespace.
	decoded, err := url.PathUnescape(payload)
	if err == nil {
		payload = decoded
	}
	payload = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, payload)
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		// Try unpadded variant (some encoders omit trailing '=').
		data, err = base64.RawStdEncoding.DecodeString(payload)
		if err != nil {
			return "", nil, fmt.Errorf("decoding base64 data URI: %w", err)
		}
	}
	ext := extFromContentType(mediaType)
	return ext, data, nil
}
