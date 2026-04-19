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
