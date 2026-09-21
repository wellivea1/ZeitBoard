package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
)

// Existing focused HTTP fixtures expose selected immutable records, rather than
// a complete server. Give their pages the same cursor/sequence semantics as the
// real pull endpoint. Malformed-page tests write their raw responses directly.
func writePullFixture(t *testing.T, w http.ResponseWriter, r *http.Request, fixture syncPullResponse) {
	t.Helper()
	since, err := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	if err != nil {
		t.Error(err)
		w.WriteHeader(400)
		return
	}
	page := syncPullResponse{SchemaVersion: "v1", Cursor: since, Records: []syncEnvelope{}}
	for _, record := range fixture.Records {
		if record.Seq > since {
			page.Records = append(page.Records, record)
			page.Cursor = record.Seq
		}
	}
	if err := json.NewEncoder(w).Encode(page); err != nil {
		t.Error(err)
	}
}
