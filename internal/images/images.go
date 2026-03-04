// Package images handles downloading external images referenced in HTML and
// rewriting the references to point to locally saved copies.
package images

import (
	"context"
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
