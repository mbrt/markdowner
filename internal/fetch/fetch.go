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

// URL fetches the page at pageURL, converts it to Markdown, and returns a Doc.
func URL(ctx context.Context, pageURL string) (output.Doc, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return output.Doc{}, err
	}
	req.Header.Set("User-Agent", "markdowner/1.0")
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

	contents, err := convert.FromHTML(pageURL, string(b))
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
	}, nil
}
