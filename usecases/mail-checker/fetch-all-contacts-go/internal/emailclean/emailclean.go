package emailclean

import (
	"regexp"
	"strings"
)

// This regex finds the valid email part.
// It excludes leading underscores or dots that aren't usually the start of an email
// but allows them inside the name.
var emailRegex = regexp.MustCompile(`[a-zA-Z0-9][a-zA-Z0-9._%+-]*@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)

type Result struct {
	Raw        string `json:"raw"`
	Cleaned    string `json:"cleaned"`
	Normalized string `json:"normalized"`
}

func Extract(raw string) Result {
	searchArea := raw

	// 1. Handle the <brackets> first to narrow the scope
	if i := strings.Index(searchArea, "<"); i >= 0 {
		if j := strings.Index(searchArea[i:], ">"); j > 0 {
			searchArea = searchArea[i+1 : i+j]
		}
	}

	// 2. Find the first valid email-looking match in the string
	// This ignores leading %% and trailing %%... [38]
	match := emailRegex.FindString(searchArea)

	// 3. Special case: if the email found has a numeric prefix with underscore
	// like "13352543_zafaraldi", we strip the numeric part.
	cleaned := match
	if idx := strings.Index(cleaned, "_"); idx > -1 {
		// Check if everything before the underscore is just digits
		prefix := cleaned[:idx]
		isNumeric := true
		for _, r := range prefix {
			if r < '0' || r > '9' {
				isNumeric = false
				break
			}
		}
		if isNumeric && len(prefix) > 0 {
			cleaned = cleaned[idx+1:]
		}
	}

	// 4. Final safety fallback
	if cleaned == "" && raw != "" {
		cleaned = strings.Trim(strings.TrimSpace(raw), "%$<> ")
	}

	return Result{
		Raw:        raw,
		Cleaned:    cleaned,
		Normalized: strings.ToLower(cleaned),
	}
}
