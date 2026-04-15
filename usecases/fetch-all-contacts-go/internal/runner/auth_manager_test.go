package runner

import (
	"context"
	"strings"
	"sync"
	"testing"

	"sf/usecases/fetch-all-contacts-go/internal/api"
)

func TestAuthManager_ReauthenticateMaxAttempts(t *testing.T) {
	in := strings.NewReader("" +
		"b1\nc1\nk1\n" +
		"b2\nc2\nk2\n" +
		"b3\nc3\nk3\n")
	var out strings.Builder
	m := NewAuthManager(api.Auth{}, in, &out, 3)

	validate := func(context.Context, api.Auth) error { return context.DeadlineExceeded }
	err := m.Reauthenticate(context.Background(), validate)
	if err == nil || !strings.Contains(err.Error(), "max re-auth attempts reached") {
		t.Fatalf("expected max attempts error, got %v", err)
	}
}

func TestAuthManager_ReauthenticateSharedPrompt(t *testing.T) {
	initial := api.Auth{BearerToken: "oldBearer", CsrfToken: "oldCsrf", Cookie: "oldCookie"}
	in := strings.NewReader("newBearer\nnewCsrf\nnewCookie\n")
	var out strings.Builder
	m := NewAuthManager(initial, in, &out, 3)

	var calls int
	var mu sync.Mutex
	validate := func(context.Context, api.Auth) error {
		mu.Lock()
		defer mu.Unlock()
		calls++
		return nil
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- m.ReauthenticateIfUnchanged(context.Background(), initial, validate)
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if count := strings.Count(out.String(), "Auth expired (403). Enter new credentials"); count != 1 {
		t.Fatalf("expected single prompt, got %d prompts", count)
	}
	got := m.GetAuth()
	if got.BearerToken != "newBearer" || got.CsrfToken != "newCsrf" || got.Cookie != "newCookie" {
		t.Fatalf("unexpected auth update: %#v", got)
	}
}

