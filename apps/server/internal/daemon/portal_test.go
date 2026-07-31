package daemon

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPortalRoutesAreAbsentWhenDisabled is exposure-gate item 1 at the routing
// layer: with the portal off there must be no /p/ path to probe at all, not
// merely one that refuses.
func TestPortalRoutesAreAbsentWhenDisabled(t *testing.T) {
	private := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler := mountPortal(private, nil)

	for _, path := range []string{"/p/", "/p/some-token", "/p/some-token/availability", "/p/assets/portal.css"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		// The private handler answers everything, which is exactly the point:
		// no portal mux was mounted, so /p/ is not special.
		if recorder.Code != http.StatusTeapot {
			t.Errorf("%s returned %d; a portal route appears to be mounted while disabled", path, recorder.Code)
		}
	}
}

func TestPortalRoutesAreMountedWhenEnabled(t *testing.T) {
	private := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	public := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
	})
	handler := mountPortal(private, public)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/p/token", nil))
	if recorder.Code != http.StatusGone {
		t.Errorf("/p/token returned %d, want the public handler's 410", recorder.Code)
	}

	// The device API must keep working alongside it.
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/status", nil))
	if recorder.Code != http.StatusTeapot {
		t.Errorf("/v1/status returned %d, want the private handler", recorder.Code)
	}
}
