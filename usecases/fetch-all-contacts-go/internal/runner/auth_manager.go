package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"sf/usecases/fetch-all-contacts-go/internal/api"
)

type AuthManager struct {
	mu          sync.RWMutex
	auth        api.Auth
	in          io.Reader
	out         io.Writer
	maxAttempts int
	attempts    int
	promptMu    sync.Mutex
}

func NewAuthManager(initial api.Auth, in io.Reader, out io.Writer, maxAttempts int) *AuthManager {
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	return &AuthManager{
		auth:        initial,
		in:          in,
		out:         out,
		maxAttempts: maxAttempts,
	}
}

func (m *AuthManager) GetAuth() api.Auth {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.auth
}

func (m *AuthManager) SetAuth(a api.Auth) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.auth = a
}

func (m *AuthManager) Reauthenticate(ctx context.Context, validate func(context.Context, api.Auth) error) error {
	m.promptMu.Lock()
	defer m.promptMu.Unlock()
	return m.reauthenticateLocked(ctx, validate)
}

func (m *AuthManager) ReauthenticateIfUnchanged(ctx context.Context, failedAuth api.Auth, validate func(context.Context, api.Auth) error) error {
	m.promptMu.Lock()
	defer m.promptMu.Unlock()

	current := m.GetAuth()
	if sameAuth(current, failedAuth) {
		return m.reauthenticateLocked(ctx, validate)
	}
	return nil
}

func (m *AuthManager) reauthenticateLocked(ctx context.Context, validate func(context.Context, api.Auth) error) error {
	if m.attempts >= m.maxAttempts {
		return fmt.Errorf("max re-auth attempts reached (%d)", m.maxAttempts)
	}

	reader := bufio.NewReader(m.in)
	for m.attempts < m.maxAttempts {
		m.attempts++
		fmt.Fprintf(m.out, "\nAuth expired (403). Enter new credentials (attempt %d/%d)\n", m.attempts, m.maxAttempts)

		bearer, err := promptLine(reader, m.out, "Bearer token: ")
		if err != nil {
			return err
		}
		csrf, err := promptLine(reader, m.out, "CSRF token: ")
		if err != nil {
			return err
		}
		cookie, err := promptLine(reader, m.out, "Cookie: ")
		if err != nil {
			return err
		}

		next := api.Auth{
			BearerToken: strings.TrimSpace(bearer),
			CsrfToken:   strings.TrimSpace(csrf),
			Cookie:      strings.TrimSpace(cookie),
		}
		if next.BearerToken == "" || next.CsrfToken == "" || next.Cookie == "" {
			fmt.Fprintln(m.out, "All values are required.")
			continue
		}
		if err := validate(ctx, next); err != nil {
			fmt.Fprintf(m.out, "Credentials invalid: %v\n", err)
			continue
		}

		m.SetAuth(next)
		fmt.Fprintln(m.out, "Credentials updated.")
		return nil
	}
	return fmt.Errorf("max re-auth attempts reached (%d)", m.maxAttempts)
}

func sameAuth(a, b api.Auth) bool {
	return strings.TrimSpace(a.BearerToken) == strings.TrimSpace(b.BearerToken) &&
		strings.TrimSpace(a.CsrfToken) == strings.TrimSpace(b.CsrfToken) &&
		strings.TrimSpace(a.Cookie) == strings.TrimSpace(b.Cookie)
}

func promptLine(r *bufio.Reader, out io.Writer, label string) (string, error) {
	fmt.Fprint(out, label)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

