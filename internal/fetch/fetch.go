// Package fetch provides utilities for fetching web page content.
package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mbrt/markdowner/internal/convert"
	"github.com/mbrt/markdowner/internal/output"
)

// Maximize compatibility with various sites by using a common desktop browser user agent.
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36"

// URL fetches the page at pageURL, converts it to Markdown, and returns a Doc.
// When downloadImages is true, external images are downloaded and stored in
// Doc.Images.
func URL(ctx context.Context, pageURL string, downloadImages bool) (output.Doc, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return output.Doc{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return output.Doc{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return output.Doc{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return output.Doc{}, err
	}

	contents, err := convert.FromHTML(ctx, pageURL, string(b), downloadImages)
	if err != nil {
		return output.Doc{}, fmt.Errorf("converting %q: %w", pageURL, err)
	}

	return output.Doc{
		Frontmatter: output.Frontmatter{
			Title: contents.Title,
			URL:   pageURL,
			Date:  time.Now().UTC(),
		},
		Markdown: contents.Markdown,
		Images:   contents.Images,
	}, nil
}
