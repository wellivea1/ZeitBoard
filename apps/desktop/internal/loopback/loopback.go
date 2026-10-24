// Package loopback receives the browser's return from a sign-in on this
// computer (RFC 8252 §7.3), as OpenAI's Sign in with ChatGPT asks it to be
// received: on 127.0.0.1 alone, at one path, from a request whose Host names
// that address exactly, with the sign-in's state compared in constant time.
// The first answer ends the wait; anything else is turned away without
// ending it, so a stray request cannot spoil the sign-in.
package loopback

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// Receiver is one sign-in's loopback address.
type Receiver struct {
	listener net.Listener
	port     int
	path     string
}

// Listen opens a receiver for path on a free port of 127.0.0.1.
func Listen(path string) (*Receiver, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, errors.New("no loopback port could be opened for signing in")
	}
	return &Receiver{listener: listener, port: listener.Addr().(*net.TCPAddr).Port, path: path}, nil
}

// Redirect is the address the sign-in returns to.
func (r *Receiver) Redirect() string {
	return "http://" + r.host() + r.path
}

func (r *Receiver) host() string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(r.port))
}

// Close stops listening.
func (r *Receiver) Close() error {
	if err := r.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}
	return nil
}

// Refused is the browser coming back with the provider's refusal, as when
// the owner declines. Code is the provider's error code.
type Refused struct {
	Code string
}

func (r Refused) Error() string { return "signing in did not finish: " + r.Code }

const answerPage = `<!doctype html><meta charset="utf-8"><title>ZeitBoard</title>` +
	`<body style="font:16px system-ui;margin:3em">%s You can close this tab and return to ZeitBoard.</body>`

// Wait serves the browser's return and gives back its query. An answer is a
// GET to the path, with Host exactly the redirect's, carrying the state once
// and nothing it names twice. A refusal the provider sent with the state ends
// the wait as Refused.
func (r *Receiver) Wait(ctx context.Context, state string) (url.Values, error) {
	type answer struct {
		query url.Values
		err   error
	}
	answers := make(chan answer, 1)
	var mu sync.Mutex
	taken := false
	// take claims the one answer the wait takes.
	take := func() bool {
		mu.Lock()
		defer mu.Unlock()
		first := !taken
		taken = true
		return first
	}
	server := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
			if request.Method != http.MethodGet || request.URL.Path != r.path || request.Host != r.host() {
				http.NotFound(w, request)
				return
			}
			query, err := url.ParseQuery(request.URL.RawQuery)
			if err != nil || len(query["state"]) != 1 ||
				subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(state)) != 1 {
				http.Error(w, "This sign-in is not the one ZeitBoard is waiting for.", http.StatusBadRequest)
				return
			}
			for name, values := range query {
				if len(values) != 1 {
					http.Error(w, "The sign-in's answer names "+name+" twice.", http.StatusBadRequest)
					return
				}
			}
			refusal := query.Get("error")
			if refusal == "" && query.Get("code") == "" {
				http.Error(w, "The sign-in sent no code.", http.StatusBadRequest)
				return
			}
			if !take() {
				http.NotFound(w, request)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if refusal != "" {
				fmt.Fprintf(w, answerPage, "Signing in did not finish.")
				answers <- answer{err: Refused{Code: printableCode(refusal)}}
				return
			}
			fmt.Fprintf(w, answerPage, "ZeitBoard is signed in.")
			answers <- answer{query: query}
		}),
	}
	go func() { _ = server.Serve(r.listener) }()
	// Shut down rather than close, so the page the browser is being sent
	// finishes before the port goes.
	defer func() {
		stopping, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(stopping)
	}()
	select {
	case got := <-answers:
		return got.query, got.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// printableCode keeps a provider's error code to what reads as one.
func printableCode(code string) string {
	if len(code) > 64 {
		code = code[:64]
	}
	for _, r := range code {
		if !(r == '_' || r == '-' || r == '.' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return "refused"
		}
	}
	return code
}
