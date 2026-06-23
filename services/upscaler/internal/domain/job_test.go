package domain

import (
	"testing"
)

func TestJobStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		name     string
		status   JobStatus
		expected bool
	}{
		{
			name:     "queued is not terminal",
			status:   JobQueued,
			expected: false,
		},
		{
			name:     "segmenting is not terminal",
			status:   JobSegmenting,
			expected: false,
		},
		{
			name:     "upscaling is not terminal",
			status:   JobUpscaling,
			expected: false,
		},
		{
			name:     "finalizing is not terminal",
			status:   JobFinalizing,
			expected: false,
		},
		{
			name:     "done is terminal",
			status:   JobDone,
			expected: true,
		},
		{
			name:     "failed is terminal",
			status:   JobFailed,
			expected: true,
		},
		{
			name:     "cancelled is terminal",
			status:   JobCancelled,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.status.IsTerminal()
			if result != tt.expected {
				t.Errorf("IsTerminal() = %v, want %v", result, tt.expected)
			}
		})
	}
}
