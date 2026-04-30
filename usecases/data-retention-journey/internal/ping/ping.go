package ping

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"sfmc-retention/internal/client"
)

const maxRetries = 3

// VerifyOrPrompt pings the API. If it fails with auth error,
// it prompts the user to re-enter credentials and retries up to maxRetries times.
// Returns updated Credentials on success, or error if all retries fail.
func VerifyOrPrompt(creds client.Credentials, debug bool) (client.Credentials, error) {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		c := client.New(creds, debug)
		err := c.Ping()
		if err == nil {
			fmt.Println("✓ Authentication verified")
			return creds, nil
		}

		fmt.Printf("✗ Auth check failed (attempt %d/%d): %v\n", attempt, maxRetries, err)
		if attempt == maxRetries {
			break
		}

		fmt.Println("\nPlease re-enter credentials:")
		creds = promptCredentials(creds)
	}
	return creds, fmt.Errorf("authentication failed after %d attempts", maxRetries)
}

func promptCredentials(current client.Credentials) client.Credentials {
	reader := bufio.NewReader(os.Stdin)

	current.JBCookie = promptField(reader, "JB Session Cookie", current.JBCookie)
	current.JBCSRFToken = promptField(reader, "JB CSRF Token", current.JBCSRFToken)
	current.MCCookie = promptField(reader, "MC Session Cookie", current.MCCookie)
	current.MCCSRFToken = promptField(reader, "MC CSRF Token", current.MCCSRFToken)
	current.BearerToken = promptField(reader, "Bearer Token", current.BearerToken)
	return current
}

func promptField(r *bufio.Reader, label, current string) string {
	display := current
	if len(display) > 20 {
		display = display[:20] + "..."
	}
	fmt.Printf("  %s [current: %s]: ", label, display)
	input, _ := r.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return current
	}
	return input
}
