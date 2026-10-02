package instapaper

import (
	"encoding/json"
	"fmt"
)

// APIError represents an error returned by the Instapaper API.
type APIError struct {
	StatusCode int
	Message    string
}

func (r *APIError) Error() string {
	return fmt.Sprintf("instapaper API: status %d: %s", r.StatusCode, r.Message)
}

// newAPIError builds an APIError from a non-200 response. Bodies that don't
// follow the documented error format are reported verbatim.
func newAPIError(status int, body []byte) *APIError {
	var resp struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Error.Message == "" {
		return &APIError{StatusCode: status, Message: string(body)}
	}
	return &APIError{StatusCode: status, Message: resp.Error.Message}
}
