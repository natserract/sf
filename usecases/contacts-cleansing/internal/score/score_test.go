package score

import "testing"

func TestFromStatus(t *testing.T) {
	allPass := FromStatus(true, true, true, true)
	if allPass.Total != 100 {
		t.Fatalf("all pass total = %d, want 100", allPass.Total)
	}
	partial := FromStatus(true, false, true, false)
	if partial.Total != 50 {
		t.Fatalf("partial total = %d, want 50", partial.Total)
	}
}
