// Package instapaper provides a client for the Instapaper API v2.
package instapaper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

const defaultBaseURL = "https://www.instapaper.com/api/2"

// Client is the Instapaper API client.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new Instapaper API client authenticating with the given
// OAuth 2 bearer token (e.g. a personal access token).
func NewClient(token string) *Client {
	return &Client{
		baseURL:    defaultBaseURL,
		token:      token,
		httpClient: http.DefaultClient,
	}
}

// ListBookmarks returns a page of bookmarks from the given section.
func (c *Client) ListBookmarks(ctx context.Context, p BookmarkListParams) (BookmarkListResponse, error) {
	params := url.Values{}
	params.Set("limit", strconv.Itoa(p.Limit))
	params.Set("offset", strconv.Itoa(p.Offset))
	if p.Section != "" {
		params.Set("section", p.Section)
	}

	var resp BookmarkListResponse
	err := c.get(ctx, "/bookmarks?"+params.Encode(), &resp)
	return resp, err
}

// GetText returns Instapaper's parsed article HTML for a bookmark.
func (c *Client) GetText(ctx context.Context, bookmarkID int) (string, error) {
	var resp ParsedBookmark
	if err := c.get(ctx, fmt.Sprintf("/bookmarks/%d/parse", bookmarkID), &resp); err != nil {
		return "", err
	}
	return resp.Content.Body, nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return newAPIError(res.StatusCode, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}
