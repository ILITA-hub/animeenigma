package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCalculateBackoff(t *testing.T) {
	tests := []struct {
		name     string
		attempt  int
		expected time.Duration
	}{
		{
			name:     "first attempt",
			attempt:  0,
			expected: 30 * time.Second, // 30s * 2^0 = 30s
		},
		{
			name:     "second attempt",
			attempt:  1,
			expected: 60 * time.Second, // 30s * 2^1 = 60s (1m)
		},
		{
			name:     "third attempt",
			attempt:  2,
			expected: 120 * time.Second, // 30s * 2^2 = 120s (2m)
		},
		{
			name:     "fourth attempt",
			attempt:  3,
			expected: 240 * time.Second, // 30s * 2^3 = 240s (4m)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateBackoff(tt.attempt)
			assert.Equal(t, tt.expected, result)
		})
	}
}
