package portal

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func forwarded(remoteAddr string, headers ...string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/p/token", nil)
	request.RemoteAddr = remoteAddr
	for _, header := range headers {
		request.Header.Add("X-Forwarded-For", header)
	}
	return request
}

func TestTheForwardedAddressIsBelievedOnlyFromANamedProxy(t *testing.T) {
	proxies, err := ParseTrustedProxies([]string{"10.0.0.0/8", " 192.0.2.7 ", ""})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name         string
		request      *http.Request
		want         string
		throughProxy bool
	}{
		{
			// Anyone can send the header. From a visitor it is their own
			// words, and believing it would let them pick an address a request.
			name:    "a visitor's own header is ignored",
			request: forwarded("203.0.113.10:5555", "198.51.100.1"),
			want:    "203.0.113.10:5555",
		},
		{
			name:         "the proxy's report is taken",
			request:      forwarded("10.1.2.3:443", "198.51.100.1"),
			want:         "198.51.100.1",
			throughProxy: true,
		},
		{
			// The proxy appends what it saw. Everything left of that is
			// whatever the visitor sent, and is not the visitor.
			name:         "what the visitor wrote before the proxy's entry is not read",
			request:      forwarded("10.1.2.3:443", "1.2.3.4, 198.51.100.1"),
			want:         "198.51.100.1",
			throughProxy: true,
		},
		{
			name:         "a chain of named proxies is read through",
			request:      forwarded("192.0.2.7:443", "198.51.100.1, 10.9.9.9", "10.1.2.3"),
			want:         "198.51.100.1",
			throughProxy: true,
		},
		{
			name:         "no header leaves the proxy's own address",
			request:      forwarded("10.1.2.3:443"),
			want:         "10.1.2.3:443",
			throughProxy: true,
		},
		{
			name:         "an unreadable entry is not read past",
			request:      forwarded("10.1.2.3:443", "198.51.100.1, unknown"),
			want:         "10.1.2.3:443",
			throughProxy: true,
		},
		{
			name:         "only proxies in the header leaves the proxy's own address",
			request:      forwarded("10.1.2.3:443", "10.4.4.4"),
			want:         "10.1.2.3:443",
			throughProxy: true,
		},
	}
	for _, test := range cases {
		got, through := visitorAddr(test.request, proxies)
		if got != test.want || through != test.throughProxy {
			t.Errorf("%s: got %q through proxy %v, want %q %v", test.name, got, through, test.want, test.throughProxy)
		}
	}

	// With no proxy named, nothing is believed, as before the setting existed.
	if got, through := visitorAddr(forwarded("10.1.2.3:443", "198.51.100.1"), nil); got != "10.1.2.3:443" || through {
		t.Errorf("with no trusted proxy: got %q through proxy %v", got, through)
	}
}

func TestTrustedProxiesMustBeAddressesOrRanges(t *testing.T) {
	if _, err := ParseTrustedProxies([]string{"proxy.example.test"}); err == nil {
		t.Fatal("a host name was accepted as a trusted proxy")
	}
	proxies, err := ParseTrustedProxies([]string{"2001:db8::/32", "::ffff:192.0.2.7"})
	if err != nil || len(proxies) != 2 {
		t.Fatalf("proxies = %v, err = %v", proxies, err)
	}
}

// Two visitors behind one named proxy are counted apart, and everything the
// proxy forwards still shares a wider allowance.
func TestVisitorsBehindANamedProxyAreLimitedApart(t *testing.T) {
	h := newHarness(t)
	proxies, err := ParseTrustedProxies([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(HandlerConfig{
		Store:          h.store,
		Now:            h.clock.Now,
		PublicOrigin:   testOrigin,
		TrustedProxies: proxies,
		// The floor that hides which links exist would make 122 requests slow.
		ResolutionFloor: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	routes := handler.Routes()
	page := func(visitor string) int {
		request := httptest.NewRequest(http.MethodGet, "/p/"+h.token, nil)
		request.RemoteAddr = "10.1.2.3:443"
		request.Header.Set("X-Forwarded-For", visitor)
		recorder := httptest.NewRecorder()
		routes.ServeHTTP(recorder, request)
		return recorder.Code
	}
	for i := 0; i < ReadLimitPerHour; i++ {
		if code := page("198.51.100.1"); code == http.StatusTooManyRequests {
			t.Fatalf("the first visitor was limited after %d requests", i)
		}
	}
	if code := page("198.51.100.1"); code != http.StatusTooManyRequests {
		t.Fatalf("the first visitor's next request got %d, want 429", code)
	}
	if code := page("198.51.100.2"); code == http.StatusTooManyRequests {
		t.Fatal("a second visitor behind the same proxy was limited by the first")
	}
}
