package instapaper

import "fmt"

// Error codes returned by the Instapaper API and internal error constants.
const (
	ErrRateLimitExceeded    = 1040
	ErrNotPremiumAccount    = 1041
	ErrApplicationSuspended = 1042

	ErrInvalidURL        = 1240
	ErrInvalidBookmarkID = 1241

	ErrGeneric = 1500
	ErrTextGen = 1550

	ErrNotAuthenticated = 666
	ErrUnmarshalError   = 667
	ErrHTTPError        = 668
)

// APIError represents an error returned by the Instapaper API.
type APIError struct {
	ErrorCode    int `json:"error_code"`
	StatusCode   int
	Message      string
	WrappedError error
}

func (r *APIError) Error() string {
	return fmt.Sprintf("status %d: err #%d - %v", r.StatusCode, r.ErrorCode, r.Message)
}
