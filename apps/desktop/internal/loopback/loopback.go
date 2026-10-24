// Package loopback receives the browser's return from a sign-in on this
// computer (RFC 8252 §7.3), as OpenAI's Sign in with ChatGPT asks it to be
// received: on loopback alone, at one path, from a request whose Host names
// the receiver's own address, with the sign-in's state compared in constant
// time. The first answer ends the wait; anything else is turned away without
// ending it, so a stray request cannot spoil the sign-in.
//
// The ChatGPT sign-in returns to 127.0.0.1 alone. Calendar accounts (ADR-0064)
// return to 127.0.0.1 for Google and to localhost for Microsoft, which a
// browser may reach over ::1.
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
	listeners []net.Listener
	port      int
	// path is where the answer comes, "" for an address without one, as a
	// calendar provider's is.
	path string
	// hosts are the Host headers an answer may carry: the receiver's own
	// addresses by name, so a page rebinding its name to loopback is turned
	// away.
	hosts map[string]bool
}

// Listen opens a receiver for path on a free port of 127.0.0.1, answering
// only by that address.
func Listen(path string) (*Receiver, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, errors.New("no loopback port could be opened for signing in")
	}
	r := &Receiver{listeners: []net.Listener{listener}, port: listener.Addr().(*net.TCPAddr).Port, path: path}
	r.hosts = map[string]bool{r.hostAs("127.0.0.1"): true}
	return r, nil
}

// ListenLocalhost opens one that answers by localhost too, listening on ::1
// at the same port when that is free, since a browser may try either for
// localhost.
func ListenLocalhost(path string) (*Receiver, error) {
	r, err := Listen(path)
	if err != nil {
		return nil, err
	}
	if v6, err := net.Listen("tcp", net.JoinHostPort("::1", strconv.Itoa(r.port))); err == nil {
		r.listeners = append(r.listeners, v6)
	}
	r.hosts[r.hostAs("localhost")] = true
	r.hosts[r.hostAs("::1")] = true
	return r, nil
}

// Redirect is the address the sign-in returns to, by 127.0.0.1.
func (r *Receiver) Redirect() string {
	return r.RedirectAs("127.0.0.1")
}

// RedirectAs is the address by another of the receiver's names: localhost,
// for a receiver ListenLocalhost opened.
func (r *Receiver) RedirectAs(host string) string {
	return "http://" + r.hostAs(host) + r.path
}

func (r *Receiver) hostAs(host string) string {
	return net.JoinHostPort(host, strconv.Itoa(r.port))
}

// answersAt reports whether a request's path is the receiver's: an address
// without a path is answered at the root.
func (r *Receiver) answersAt(path string) bool {
	if r.path == "" {
		return path == "/" || path == ""
	}
	return path == r.path
}

// Close stops listening.
func (r *Receiver) Close() error {
	var failures []error
	for _, listener := range r.listeners {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
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
// GET to the path, with Host one of the receiver's own addresses, carrying
// the state once and nothing it names twice. A refusal the provider sent with
// the state ends the wait as Refused.
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
			if request.Method != http.MethodGet || !r.answersAt(request.URL.Path) || !r.hosts[request.Host] {
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
			fmt.Fprintf(w, answerPage, "ZeitBoard has the answer and finishes signing in.")
			answers <- answer{query: query}
		}),
	}
	for _, listener := range r.listeners {
		go func() { _ = server.Serve(listener) }()
	}
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
