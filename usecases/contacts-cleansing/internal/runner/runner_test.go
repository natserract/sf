package runner

import (
	"testing"
	"time"
)

func TestBackoffWithJitter_StaysWithinBounds(t *testing.T) {
	initial := 100 * time.Millisecond
	max := 2 * time.Second
	for attempt := 1; attempt <= 20; attempt++ {
		d := backoffWithJitter(attempt, initial, max)
		if d < initial {
			t.Fatalf("attempt=%d: backoff %s below initial %s", attempt, d, initial)
		}
		if d > max {
			t.Fatalf("attempt=%d: backoff %s above max %s", attempt, d, max)
		}
	}
}

