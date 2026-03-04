// Package instapaper provides a client for the Instapaper API.
package instapaper

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gomodule/oauth1/oauth"
)

const defaultBaseURL = "https://www.instapaper.com/api/1.1"

// Client is the Instapaper API client.
type Client struct {
	baseURL     string
	username    string
	password    string
	oauthClient oauth.Client
	credentials *oauth.Credentials
}

// NewClient creates a new Instapaper API client.
func NewClient(consumerKey, consumerSecret, username, password string) *Client {
	return &Client{
		oauthClient: oauth.Client{
			SignatureMethod: oauth.HMACSHA1,
			Credentials: oauth.Credentials{
				Token:  consumerKey,
				Secret: consumerSecret,
			},
			TokenRequestURI: defaultBaseURL + "/oauth/access_token",
		},
		username: username,
		password: password,
		baseURL:  defaultBaseURL,
	}
}

// Authenticate exchanges credentials for OAuth tokens.
func (c *Client) Authenticate(ctx context.Context) error {
	creds, _, err := c.oauthClient.RequestTokenXAuthContext(ctx, nil, c.username, c.password)
	if err != nil {
		return err
	}
	c.credentials = creds
	return nil
}

// ListBookmarks returns bookmarks from the given folder.
func (c *Client) ListBookmarks(ctx context.Context, p BookmarkListParams) (*BookmarkListResponse, error) {
	params := url.Values{}
	params.Set("limit", strconv.Itoa(p.Limit))
	if p.Folder != "" {
		params.Set("folder_id", p.Folder)
	}
	if len(p.Skip) > 0 {
		var ids []string
		for _, b := range p.Skip {
			ids = append(ids, strconv.Itoa(b.ID))
		}
		params.Set("have", strings.Join(ids, ","))
	}

	res, err := c.makeRequest(ctx, "/bookmarks/list", params)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, &APIError{StatusCode: res.StatusCode, Message: err.Error(), ErrorCode: ErrHTTPError, WrappedError: err}
	}

	// The response is a JSON array of mixed objects; extract only bookmarks.
	var raw []json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, &APIError{StatusCode: res.StatusCode, Message: err.Error(), ErrorCode: ErrUnmarshalError, WrappedError: err}
	}

	resp := &BookmarkListResponse{RawResponse: string(body)}
	for _, item := range raw {
		var typed struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(item, &typed); err != nil {
			continue
		}
		if typed.Type == "bookmark" {
			var b Bookmark
			if err := json.Unmarshal(item, &b); err == nil {
				resp.Bookmarks = append(resp.Bookmarks, b)
			}
		}
	}
	return resp, nil
}

// GetText returns the processed text-view HTML for a bookmark.
func (c *Client) GetText(ctx context.Context, bookmarkID int) (string, error) {
	params := url.Values{}
	params.Set("bookmark_id", strconv.Itoa(bookmarkID))
	res, err := c.makeRequest(ctx, "/bookmarks/get_text", params)
	if err != nil {
		return "", err
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", &APIError{StatusCode: res.StatusCode, Message: err.Error(), ErrorCode: ErrHTTPError, WrappedError: err}
	}
	return string(body), nil
}

func (c *Client) makeRequest(ctx context.Context, path string, params url.Values) (*http.Response, error) {
	if c.credentials == nil {
		return nil, &APIError{Message: "call Authenticate() first", ErrorCode: ErrNotAuthenticated}
	}
	res, err := c.oauthClient.PostContext(ctx, c.credentials, c.baseURL+path, params)
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusOK {
		return res, nil
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, &APIError{StatusCode: res.StatusCode, Message: err.Error(), ErrorCode: ErrHTTPError, WrappedError: err}
	}
	var apiErrors []APIError
	if err := json.Unmarshal(body, &apiErrors); err != nil || len(apiErrors) == 0 {
		return nil, &APIError{StatusCode: res.StatusCode, Message: string(body), ErrorCode: ErrHTTPError}
	}
	apiErrors[0].StatusCode = res.StatusCode
	return nil, &apiErrors[0]
}
