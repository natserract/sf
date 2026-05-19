package main

import "testing"

func TestNormalizeChannelFilter(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantOp    string
		wantValue string
		wantErr   bool
	}{
		{name: "empty unfiltered", input: "", wantOp: "Is", wantValue: ""},
		{name: "mobile lowercase", input: "mobile", wantOp: "Is", wantValue: "MOBILE"},
		{name: "push uppercase", input: "PUSH", wantOp: "Is", wantValue: "PUSH"},
		{name: "whitespace trimmed", input: "  push  ", wantOp: "Is", wantValue: "PUSH"},
		{name: "invalid value", input: "EMAIL", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOp, gotValue, err := normalizeChannelFilter(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeChannelFilter(%q) expected error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeChannelFilter(%q): %v", tt.input, err)
			}
			if gotOp != tt.wantOp || gotValue != tt.wantValue {
				t.Fatalf("normalizeChannelFilter(%q) = (%q,%q) want (%q,%q)", tt.input, gotOp, gotValue, tt.wantOp, tt.wantValue)
			}
		})
	}
}

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

