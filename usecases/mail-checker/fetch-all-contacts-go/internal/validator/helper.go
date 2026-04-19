package validator

import (
	"regexp"

	"github.com/nyaruka/phonenumbers"
)

var digitCheck = regexp.MustCompile(`^[0-9]+$`)

// ValidatePhoneNumber checks if the contact key is a valid international phone number
func ValidatePhoneNumber(contactKey string, defaultRegion string) bool {
	// 1. Parse the string.
	// defaultRegion is an ISO 3166-1 alpha-2 country code (e.g., "US", "GB")
	// used if the number doesn't start with a "+".
	num, err := phonenumbers.Parse(contactKey, defaultRegion)
	if err != nil {
		return false
	}

	// 2. Verify if it's a valid number for that region/format
	return phonenumbers.IsValidNumber(num)
}

// IsNumber checks if the string contains only characters 0-9.
func IsNumber(s string) bool {
	return digitCheck.MatchString(s)
}
