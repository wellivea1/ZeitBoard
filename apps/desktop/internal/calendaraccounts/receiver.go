package calendaraccounts

import (
	"context"

	"non24.app/desktop/internal/loopback"
)

// Receiver is the one-time loopback address the provider sends the browser
// back to with a code (RFC 8252 §7.3). It is the loopback package's, with
// its checks: the root path, a Host header that names the receiver's own
// address, and the sign-in's state once.
type Receiver struct {
	inner *loopback.Receiver
}

// Listen opens a receiver on a free loopback port: on 127.0.0.1, and on ::1
// at the same port when that is free too, since a browser may try either
// for localhost.
func Listen() (*Receiver, error) {
	inner, err := loopback.ListenLocalhost("")
	if err != nil {
		return nil, err
	}
	return &Receiver{inner: inner}, nil
}

// Redirect is the address to give provider. Google's desktop clients take
// the loopback IP literal RFC 8252 §8.3 recommends; Microsoft's app
// registrations take a loopback redirect only as localhost, and match it at
// any port.
func (r *Receiver) Redirect(provider Provider) string {
	if provider == Microsoft {
		return r.inner.RedirectAs("localhost")
	}
	return r.inner.Redirect()
}

// Close stops listening.
func (r *Receiver) Close() error {
	return r.inner.Close()
}

// Wait serves the browser's return and gives back its code. A request that
// is not this sign-in's answer is turned away without ending the wait; one
// that carries the provider's refusal ends it with the refusal.
func (r *Receiver) Wait(ctx context.Context, state string) (string, error) {
	answer, err := r.inner.Wait(ctx, state)
	if err != nil {
		return "", err
	}
	return answer.Get("code"), nil
}
