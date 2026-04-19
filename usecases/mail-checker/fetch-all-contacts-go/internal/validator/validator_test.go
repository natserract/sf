package validator

import (
	"context"
	"testing"
)

func TestValidateInvalidSyntax(t *testing.T) {
	s := NewService()
	got := s.Validate(context.Background(), "%%bad-email%%")
	if got.Syntax.Status != "failed" {
		t.Fatalf("syntax status = %q, want failed", got.Syntax.Status)
	}
	if got.Domain.Status != "skipped" || got.MX.Status != "skipped" || got.SMTP.Status != "skipped" {
		t.Fatalf("expected downstream checks skipped, got domain=%q mx=%q smtp=%q", got.Domain.Status, got.MX.Status, got.SMTP.Status)
	}
}

func TestIsNumber(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		// Data from your list
		{"34689257191", true},
		{"447405671467", true},
		{"4915210899596", true},
		{"31620268917", true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := IsNumber(tc.input)
			if result != tc.expected {
				t.Errorf("IsNumber(%s) = %v; want %v", tc.input, result, tc.expected)
			}
		})
	}
}
