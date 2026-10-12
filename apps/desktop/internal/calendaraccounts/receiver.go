package calendaraccounts

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"
)

// Receiver is the one-time loopback address the provider sends the browser
// back to with a code (RFC 8252 §7.3). It listens on the loopback interface
// alone, takes one answer whose state matches, and closes.
type Receiver struct {
	listeners []net.Listener
	port      int
}

// Listen opens a receiver on a free loopback port: on 127.0.0.1, and on ::1
// at the same port when that is free too, since a browser may try either
// for localhost.
func Listen() (*Receiver, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, errors.New("no loopback port could be opened for signing in")
	}
	port := listener.Addr().(*net.TCPAddr).Port
	receiver := &Receiver{listeners: []net.Listener{listener}, port: port}
	if v6, err := net.Listen("tcp", net.JoinHostPort("::1", strconv.Itoa(port))); err == nil {
		receiver.listeners = append(receiver.listeners, v6)
	}
	return receiver, nil
}

// Redirect is the address to give provider. Google's desktop clients take
// the loopback IP literal RFC 8252 §8.3 recommends; Microsoft's app
// registrations take a loopback redirect only as localhost, and match it at
// any port.
func (r *Receiver) Redirect(provider Provider) string {
	host := "127.0.0.1"
	if provider == Microsoft {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(r.port))
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

const answerPage = `<!doctype html><meta charset="utf-8"><title>ZeitBoard</title>` +
	`<body style="font:16px system-ui;margin:3em">%s You can close this tab and return to ZeitBoard.</body>`

// Wait serves the browser's return and gives back its code. A request without
// the expected state is answered and ignored; one that carries the provider's
// refusal ends the wait with it.
func (r *Receiver) Wait(ctx context.Context, state string) (string, error) {
	type answer struct {
		code string
		err  error
	}
	answers := make(chan answer, 1)
	server := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			query := request.URL.Query()
			if subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(state)) != 1 {
				http.Error(w, "This sign-in is not the one ZeitBoard is waiting for.", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if refusal := query.Get("error"); refusal != "" {
				fmt.Fprintf(w, answerPage, "Signing in did not finish.")
				select {
				case answers <- answer{err: fmt.Errorf("signing in did not finish: %s", refusal)}:
				default:
				}
				return
			}
			code := query.Get("code")
			if code == "" {
				http.Error(w, "The provider sent no code.", http.StatusBadRequest)
				return
			}
			fmt.Fprintf(w, answerPage, "ZeitBoard is signed in.")
			select {
			case answers <- answer{code: code}:
			default:
			}
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
		return got.code, got.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
