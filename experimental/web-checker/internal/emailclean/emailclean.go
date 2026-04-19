package emailclean

import (
	"regexp"
	"strings"
)

var trimGarbage = regexp.MustCompile(`^[^a-zA-Z0-9]+|[^a-zA-Z0-9]+$`)

type Result struct {
	Raw        string `json:"raw"`
	Cleaned    string `json:"cleaned"`
	Normalized string `json:"normalized"`
}

func Extract(raw string) Result {
	s := strings.TrimSpace(raw)
	cleaned := s

	if i := strings.Index(cleaned, "<"); i >= 0 {
		if j := strings.Index(cleaned[i:], ">"); j > 0 {
			cleaned = cleaned[i+1 : i+j]
		}
	}

	cleaned = strings.TrimSpace(cleaned)
	cleaned = trimGarbage.ReplaceAllString(cleaned, "")
	cleaned = strings.Trim(cleaned, "%$\"'` ")
	cleaned = strings.TrimSpace(cleaned)

	return Result{
		Raw:        raw,
		Cleaned:    cleaned,
		Normalized: strings.ToLower(cleaned),
	}
}
