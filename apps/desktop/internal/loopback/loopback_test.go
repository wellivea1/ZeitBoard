package loopback

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func get(t *testing.T, address, host string) int {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, address, nil)
	if err != nil {
		t.Fatal(err)
	}
	if host != "" {
		request.Host = host
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	return response.StatusCode
}

// Only a GET to the path, with the exact host and the state once, answers
// the sign-in, and nothing before it ends the wait.
func TestOnlyTheSignInsOwnAnswerIsTaken(t *testing.T) {
	receiver, err := Listen("/auth/callback")
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()
	redirect := receiver.Redirect()
	if !strings.HasPrefix(redirect, "http://127.0.0.1:") || !strings.HasSuffix(redirect, "/auth/callback") {
		t.Fatalf("redirect %s", redirect)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	type result struct {
		query url.Values
		err   error
	}
	done := make(chan result, 1)
	go func() {
		query, err := receiver.Wait(ctx, "expected-state")
		done <- result{query, err}
	}()

	base := strings.TrimSuffix(redirect, "/auth/callback")
	port := strings.TrimPrefix(base, "http://127.0.0.1:")
	for name, tc := range map[string]struct {
		address, host string
		want          int
	}{
		"another path":  {base + "/?state=expected-state&code=c", "", http.StatusNotFound},
		"another host":  {redirect + "?state=expected-state&code=c", "localhost:" + port, http.StatusNotFound},
		"a rebinding":   {redirect + "?state=expected-state&code=c", "attacker.example:" + port, http.StatusNotFound},
		"another state": {redirect + "?state=other&code=c", "", http.StatusBadRequest},
		"two states":    {redirect + "?state=expected-state&state=expected-state&code=c", "", http.StatusBadRequest},
		"two codes":     {redirect + "?state=expected-state&code=c&code=d", "", http.StatusBadRequest},
		"no code":       {redirect + "?state=expected-state", "", http.StatusBadRequest},
	} {
		if status := get(t, tc.address, tc.host); status != tc.want {
			t.Errorf("%s answered %d, not %d", name, status, tc.want)
		}
	}
	select {
	case got := <-done:
		t.Fatalf("a stray request ended the wait: %+v", got)
	default:
	}

	if status := get(t, redirect+"?state=expected-state&code=the-code&client_id=client-1", ""); status != http.StatusOK {
		t.Fatalf("the answer was turned away: %d", status)
	}
	got := <-done
	if got.err != nil || got.query.Get("code") != "the-code" || got.query.Get("client_id") != "client-1" {
		t.Fatalf("waited for %+v", got)
	}
}

// The owner declining in the browser ends the wait with the provider's code,
// kept to what reads as one.
func TestARefusalEndsTheWait(t *testing.T) {
	for raw, want := range map[string]string{"access_denied": "access_denied", "<script>": "refused"} {
		receiver, err := Listen("/auth/callback")
		if err != nil {
			t.Fatal(err)
		}
		errs := make(chan error, 1)
		go func() {
			_, err := receiver.Wait(context.Background(), "s")
			errs <- err
		}()
		if status := get(t, receiver.Redirect()+"?state=s&error="+url.QueryEscape(raw), ""); status != http.StatusOK {
			t.Fatalf("a refusal answered %d", status)
		}
		var refused Refused
		if err := <-errs; !errors.As(err, &refused) || refused.Code != want {
			t.Errorf("%q ended the wait as %v", raw, err)
		}
		receiver.Close()
	}

	receiver, err := Listen("/auth/callback")
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()
	stopped, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := receiver.Wait(stopped, "s"); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled wait ended as %v", err)
	}
}

// A receiver that answers by localhost, as a calendar provider's address
// is, takes 127.0.0.1, localhost and ::1 by name, at the root, and still no
// other host.
func TestALocalhostReceiverAnswersByItsOwnNames(t *testing.T) {
	for _, name := range []string{"127.0.0.1", "localhost", "::1", "attacker.example"} {
		receiver, err := ListenLocalhost("")
		if err != nil {
			t.Fatal(err)
		}
		redirect := receiver.RedirectAs("localhost")
		if !strings.HasPrefix(redirect, "http://localhost:") || strings.HasSuffix(redirect, "/") {
			t.Fatalf("redirect %s", redirect)
		}
		port := strings.TrimPrefix(redirect, "http://localhost:")
		host := name + ":" + port
		if name == "::1" {
			host = "[::1]:" + port
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		done := make(chan error, 1)
		go func() {
			_, err := receiver.Wait(ctx, "s")
			done <- err
		}()
		// Sent to 127.0.0.1, with the Host header of each name.
		status := get(t, receiver.Redirect()+"/?state=s&code=c", host)
		want := http.StatusOK
		if name == "attacker.example" {
			want = http.StatusNotFound
		}
		if status != want {
			t.Errorf("%s answered %d, not %d", name, status, want)
		}
		if err := <-done; (err == nil) != (want == http.StatusOK) {
			t.Errorf("%s ended the wait as %v", name, err)
		}
		cancel()
		receiver.Close()
	}
}
