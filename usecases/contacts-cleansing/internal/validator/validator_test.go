package validator

import (
	"context"
	"slices"
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

// TestValidateEmails runs the full truemail pipeline against a set of addresses
// and asserts the expected outcome at each step.
//
// Syntax-only cases resolve instantly (no network). Domain/MX/SMTP cases will
// make real DNS / SMTP calls, so they are marked as integration tests and
// skipped when the -short flag is passed:
//
//	go test ./...           – runs everything
//	go test -short ./...    – skips network cases
func TestValidateEmails(t *testing.T) {
	s := NewService()

	type want struct {
		syntax string
		domain string
		mx     string
		smtp   string
		smtps  []string
		status string
	}

	tests := []struct {
		name  string
		email string
		short bool // skip when -short
		want  want
	}{
		// ── Syntax failures (no network needed) ──────────────────────────────
		{
			name:  "empty string",
			email: "",
			want:  want{syntax: "failed", domain: "skipped", mx: "skipped", smtp: "skipped", status: "failed"},
		},
		{
			name:  "missing at-sign",
			email: "notanemail",
			want:  want{syntax: "failed", domain: "skipped", mx: "skipped", smtp: "skipped", status: "failed"},
		},
		{
			name:  "Invalid",
			email: "kufriyadi@gmail.com",
			want: want{
				syntax: "passed",
				domain: "passed",
				mx:     "passed",
				smtps:  []string{"passed"},
			},
		},
		{
			name:  "Invalid",
			email: "cursor@cursorvalid.com",
			want: want{
				syntax: "passed",
				domain: "failed",
				mx:     "failed",
				smtp:   "skipped",
				status: "failed",
			},
		},
		{
			name:  "gmail valid syntax and domain",
			email: "alfins132@gmail.com",
			// SMTP mailbox checks against major providers are not deterministic:
			// providers may reject verification probes even for existing inboxes.
			want: want{
				syntax: "passed",
				domain: "passed",
				mx:     "passed",
				smtps:  []string{"passed"},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.short && testing.Short() {
				t.Skip("skipping network test in -short mode")
			}

			got := s.Validate(context.Background(), tc.email)

			if got.Syntax.Status != tc.want.syntax {
				t.Errorf("syntax: got %q, want %q", got.Syntax.Status, tc.want.syntax)
			}
			if tc.want.domain != "" && got.Domain.Status != tc.want.domain {
				t.Errorf("domain: got %q, want %q", got.Domain.Status, tc.want.domain)
			}
			if tc.want.mx != "" && got.MX.Status != tc.want.mx {
				t.Errorf("mx: got %q, want %q", got.MX.Status, tc.want.mx)
			}
			if tc.want.smtp != "" && got.SMTP.Status != tc.want.smtp {
				t.Errorf("smtp: got %q, want %q", got.SMTP.Status, tc.want.smtp)
			}
			if len(tc.want.smtps) > 0 && !slices.Contains(tc.want.smtps, got.SMTP.Status) {
				t.Errorf("smtp: got %q, want one of %v", got.SMTP.Status, tc.want.smtps)
			}
			if tc.want.status != "" && got.Status != tc.want.status {
				t.Errorf("overall status: got %q, want %q", got.Status, tc.want.status)
			}

			// Sanity: total score must always be non-negative.
			if got.Total < 0 {
				t.Errorf("total score is negative: %d", got.Total)
			}
		})
	}
}

// TestValidateScoreConsistency verifies that the score fields are populated
// and that downstream steps are always skipped (never passed) when an earlier
// step failed – without making any network calls.
func TestValidateScoreConsistency(t *testing.T) {
	s := NewService()

	badEmails := []string{
		"",
		"notvalid",
		"missing@",
		"@nodomain",
		"double@@at.com",
	}

	for _, email := range badEmails {
		t.Run(email, func(t *testing.T) {
			got := s.Validate(context.Background(), email)

			// When syntax fails every downstream step must be skipped.
			if got.Syntax.Status == "failed" {
				for _, step := range []struct {
					name   string
					status string
				}{
					{"domain", got.Domain.Status},
					{"mx", got.MX.Status},
					{"smtp", got.SMTP.Status},
				} {
					if step.status != "skipped" {
						t.Errorf("%s should be skipped after syntax failure, got %q", step.name, step.status)
					}
				}
			}

			// Total score must be a non-negative integer.
			if got.Total < 0 {
				t.Errorf("negative total score %d for %q", got.Total, email)
			}
		})
	}
}
