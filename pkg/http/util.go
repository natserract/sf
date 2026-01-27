package http

import (
	"fmt"
	"net/url"
)

func BuildURL(baseURL, path string, queryParams map[string]string) (string, error) {
	// Parse the base URL
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("error parsing base URL: %w", err)
	}

	// Append the path
	parsedURL.Path = path

	// Set query parameters dynamically
	q := url.Values{}
	for key, value := range queryParams {
		q.Set(key, value)
	}
	parsedURL.RawQuery = q.Encode()

	// Return the full URL as a string
	return parsedURL.String(), nil
}
