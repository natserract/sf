package validator

import (
	"context"
	"fmt"
	"time"

	truemail "github.com/truemail-rb/truemail-go"

	"sf/usecases/mail-checker/internal/emailclean"
	"sf/usecases/mail-checker/internal/score"
)

type Step struct {
	Status    string `json:"status"`
	Reason    string `json:"reason"`
	LatencyMS int    `json:"latencyMs"`
	Score     int    `json:"score"`
}

type Result struct {
	Clean  emailclean.Result `json:"clean"`
	Syntax Step              `json:"syntax"`
	Domain Step              `json:"domainDns"`
	MX     Step              `json:"mx"`
	SMTP   Step              `json:"smtp"`
	Total  int               `json:"totalScore"`
	Status string            `json:"status"`
	Reason string            `json:"reason"`
}

type Service struct {
	configuration *truemail.Configuration
	configErr     error
}

func NewService() *Service {
	cfg, err := truemail.NewConfiguration(truemail.ConfigurationAttr{
		VerifierEmail:         "verifier@example.com",
		ValidationTypeDefault: "smtp",
		ConnectionTimeout:     10,
		ResponseTimeout:       10,
		ConnectionAttempts:    2,
		SmtpFailFast:          false,
		SmtpSafeCheck:         true,
	})
	return &Service{
		configuration: cfg,
		configErr:     err,
	}
}

func (s *Service) Validate(ctx context.Context, raw string) Result {
	_ = ctx
	out := Result{Status: "done"}
	out.Clean = emailclean.Extract(raw)
	email := out.Clean.Normalized

	// Guard: if the service was somehow not initialised, fail every step
	// cleanly instead of dereferencing a nil pointer.
	if s == nil || s.configuration == nil {
		const reason = "validator not initialised"
		out.Syntax = failed(reason, 0)
		out.Domain = skipped(reason)
		out.MX = skipped(reason)
		out.SMTP = skipped(reason)
		out.Status = "failed"
		out.Reason = reason
		return out.withScore()
	}

	if s.configErr != nil || s.configuration == nil {
		out.Syntax = failed("validator config error", 0)
		out.Domain = skipped("validator config error")
		out.MX = skipped("validator config error")
		out.SMTP = skipped("validator config error")
		out.Status = "failed"
		out.Reason = "validator config error"
		return out.withScore()
	}

	regexResult, regexErr, regexLatency := s.runValidation(email, "regex")
	out.Syntax = stepFromTruemail(regexResult, regexErr, "regex")
	out.Syntax.LatencyMS = regexLatency
	if out.Syntax.Status != "passed" {
		out.Domain = skipped("syntax failed")
		out.MX = skipped("syntax failed")
		out.SMTP = skipped("syntax failed")
		out.Status = "failed"
		out.Reason = "syntax failed"
		return out.withScore()
	}

	mxResult, mxErr, mxLatency := s.runValidation(email, "mx")
	out.Domain = stepFromTruemail(mxResult, mxErr, "mx")
	out.Domain.LatencyMS = mxLatency
	out.MX = stepFromTruemail(mxResult, mxErr, "mx")
	out.MX.LatencyMS = mxLatency
	if out.MX.Status != "passed" {
		out.SMTP = skipped("mx failed")
		out.Status = "failed"
		out.Reason = "mx failed"
		return out.withScore()
	}

	smtpResult, smtpErr, smtpLatency := s.runValidation(email, "smtp")
	out.SMTP = stepFromTruemail(smtpResult, smtpErr, "smtp")
	out.SMTP.LatencyMS = smtpLatency
	if out.SMTP.Status != "passed" {
		out.Status = "failed"
		out.Reason = "smtp mailbox check failed"
		return out.withScore()
	}
	return out.withScore()
}

func (s *Service) runValidation(email string, validationType string) (*truemail.ValidatorResult, error, int) {
	start := time.Now()
	result, err := truemail.Validate(email, s.configuration, validationType)
	return result, err, int(time.Since(start).Milliseconds())
}

func stepFromTruemail(result *truemail.ValidatorResult, err error, key string) Step {
	if err != nil {
		return failed(err.Error(), 0)
	}
	if result == nil {
		return failed("empty result", 0)
	}
	if result.Success {
		return Step{Status: "passed", Reason: "ok"}
	}
	if reason, ok := result.Errors[key]; ok && reason != "" {
		return Step{Status: "failed", Reason: reason}
	}
	for _, reason := range result.Errors {
		fmt.Println("Getting Error: ", reason)
		if reason != "" {
			return Step{Status: "failed", Reason: reason}
		}
	}
	return Step{Status: "failed", Reason: "validation failed"}
}

func failed(reason string, latencyMS int) Step {
	return Step{
		Status:    "failed",
		Reason:    reason,
		LatencyMS: latencyMS,
	}
}

func skipped(reason string) Step {
	return Step{
		Status: "skipped",
		Reason: reason,
	}
}

func (r Result) withScore() Result {
	b := score.FromStatus(
		r.Syntax.Status == "passed",
		r.Domain.Status == "passed",
		r.MX.Status == "passed",
		r.SMTP.Status == "passed",
	)
	r.Syntax.Score = b.Syntax
	r.Domain.Score = b.Domain
	r.MX.Score = b.MX
	r.SMTP.Score = b.SMTP
	r.Total = b.Total
	return r
}
