// Package timeutil provides shared time parsing utilities.
package timeutil

import (
	"fmt"
	"time"
)

// ParseDate parses a date string in RFC3339 or YYYY-MM-DD format.
func ParseDate(s string) (time.Time, error) {
	formats := []string{time.RFC3339, "2006-01-02"}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse %q as RFC3339 or YYYY-MM-DD", s)
}
