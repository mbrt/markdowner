// Package images handles downloading external images referenced in Markdown and
// rewriting the references to point to locally saved copies.
package images

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/crypto/sha3"
)

// reImage matches Markdown image syntax with an absolute http/https URL:
//
//	![alt text](https://example.com/img.png)
//	![alt text](https://example.com/img.png "title")
var reImage = regexp.MustCompile(`!\[([^\]]*)\]\((https?://[^\s)"']+)([^)]*)\)`)

// Rewrite scans markdown for external image references, downloads each image,
// and returns the updated markdown (with references replaced by relative local
// paths of the form "img/<sha3-128hex>.<ext>") together with a map from those
// local paths to the raw image bytes.
//
// Images that fail to download are left unchanged and a warning is logged.
func Rewrite(ctx context.Context, markdown string) (string, map[string][]byte, error) {
	// cache maps image URL -> local relative path (empty string = download failed).
	cache := map[string]string{}
	images := map[string][]byte{}

	result := reImage.ReplaceAllStringFunc(markdown, func(match string) string {
		sub := reImage.FindStringSubmatch(match)
		if sub == nil {
			return match
		}
		alt, imgURL, rest := sub[1], sub[2], sub[3]

		if lp, seen := cache[imgURL]; seen {
			if lp == "" {
				return match
			}
			return fmt.Sprintf("![%s](%s%s)", alt, lp, rest)
		}

		lp, data, err := downloadImage(ctx, imgURL)
		if err != nil {
			slog.Warn("downloading image", "url", imgURL, "err", err)
			cache[imgURL] = ""
			return match
		}
		cache[imgURL] = lp
		images[lp] = data
		return fmt.Sprintf("![%s](%s%s)", alt, lp, rest)
	})

	return result, images, nil
}

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
