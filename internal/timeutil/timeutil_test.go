package timeutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDateRelativeTo(t *testing.T) {
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		input   string
		want    time.Time
		wantErr string
	}{
		{
			name:  "RFC3339",
			input: "2024-01-15T10:30:00Z",
			want:  time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:  "YYYY-MM-DD",
			input: "2024-01-15",
			want:  time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "relative 7 days",
			input: "7d",
			want:  time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC),
		},
		{
			name:  "relative 0 days",
			input: "0d",
			want:  now,
		},
		{
			name:  "relative 1 day",
			input: "1d",
			want:  time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC),
		},
		{
			name:    "negative days rejected",
			input:   "-3d",
			wantErr: `cannot parse "-3d"`,
		},
		{
			name:    "invalid format",
			input:   "not-a-date",
			wantErr: `cannot parse "not-a-date"`,
		},
		{
			name:    "d suffix with non-number",
			input:   "foood",
			wantErr: `cannot parse "foood"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseDateRelativeTo(tc.input, now)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
