// Package images handles downloading external images referenced in HTML and
// rewriting the references to point to locally saved copies.
package images

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/sha3"
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
