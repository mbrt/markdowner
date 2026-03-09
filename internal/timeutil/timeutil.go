// Package timeutil provides shared time parsing utilities.
package timeutil

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseDate parses a date string in RFC3339, YYYY-MM-DD, or relative format
// (e.g. "7d" means 7 days ago from now).
func ParseDate(s string) (time.Time, error) {
	return ParseDateRelativeTo(s, time.Now())
}

// ParseDateRelativeTo parses a date string like ParseDate but uses now as the
// reference time for relative formats.
func ParseDateRelativeTo(s string, now time.Time) (time.Time, error) {
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(s[:len(s)-1])
		if err != nil || days < 0 {
			return time.Time{}, fmt.Errorf("cannot parse %q: expected a non-negative number of days (e.g. \"7d\")", s)
		}
		return now.AddDate(0, 0, -days), nil
	}
	formats := []string{time.RFC3339, "2006-01-02"}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse %q as RFC3339, YYYY-MM-DD, or relative days (e.g. \"7d\")", s)
}
