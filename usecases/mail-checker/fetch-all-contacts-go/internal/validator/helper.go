package validator

import "github.com/nyaruka/phonenumbers"

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
