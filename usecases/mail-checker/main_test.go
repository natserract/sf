package main

import "testing"

func TestTotalPagesFor(t *testing.T) {
	tests := []struct {
		name       string
		totalCount int
		pageSize   int
		want       int
	}{
		{name: "zero count", totalCount: 0, pageSize: 25, want: 0},
		{name: "exact division", totalCount: 100, pageSize: 25, want: 4},
		{name: "round up", totalCount: 101, pageSize: 25, want: 5},
		{name: "invalid page size", totalCount: 10, pageSize: 0, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := totalPagesFor(tt.totalCount, tt.pageSize); got != tt.want {
				t.Fatalf("totalPagesFor(%d,%d)=%d want %d", tt.totalCount, tt.pageSize, got, tt.want)
			}
		})
	}
}

