package portal

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// threadHarness makes a link that carries requests and messages, a request on
// it, and the two cookies its author holds.
func (h *harness) threadHarness(t *testing.T) (string, Profile, string, Request, string) {
	t.Helper()
	token, profile := h.createProfile(Grants{WakingWindows: true, AllowRequests: true, AllowMessages: true})
	h.publish(profile.ID, h.sampleSnapshot())
	login := h.loginWithHeaders(token, testPasscode, map[string]string{"Sec-Fetch-Site": "same-origin"})
	session := h.sessionCookie(login)
	created, err := h.store.CreateRequest(context.Background(), profile, session, RequestInput{
		WindowStart: h.clock.Now().Add(3 * time.Hour),
		WindowEnd:   h.clock.Now().Add(6 * time.Hour),
		ZoneID:      "UTC",
		Handle:      "Ada",
	}, false, h.clock.Now())
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	authorized, err := h.store.AuthorizeRequestSecret(context.Background(), profile.ID, created.Request.ID, created.Secret, h.clock.Now())
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	return token, profile, session, created.Request, authorized.Session
}

func (h *harness) postMessage(token, requestID, session, requestCookie, body, csrf string) *httptest.ResponseRecorder {
	h.t.Helper()
	form := url.Values{"message": {body}, "csrf": {csrf}}
	request := httptest.NewRequest(http.MethodPost, "/p/"+token+"/requests/"+requestID+"/messages",
		strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.RemoteAddr = "203.0.113.10:5555"
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session})
	if requestCookie != "" {
		request.AddCookie(&http.Cookie{Name: requestCookieName, Value: requestCookie})
	}
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)
	return recorder
}

func (h *harness) statusPage(token, requestID, session, requestCookie string) string {
	h.t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/p/"+token+"/requests/"+requestID, nil)
	request.RemoteAddr = "203.0.113.10:5555"
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session})
	request.AddCookie(&http.Cookie{Name: requestCookieName, Value: requestCookie})
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)
	return recorder.Body.String()
}

func TestAuthorAndOwnerShareAThread(t *testing.T) {
	h := newHarness(t)
	token, profile, session, request, requestCookie := h.threadHarness(t)

	recorder := h.postMessage(token, request.ID, session, requestCookie, "Could we do <b>Tuesday</b>?\nAfternoon is best.", h.store.CSRFToken(session))
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("post status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if _, err := h.store.AppendMessage(context.Background(), profile, request.ID, AuthorOwner, "Tuesday works.", h.clock.Now()); err != nil {
		t.Fatalf("owner reply: %v", err)
	}

	page := h.statusPage(token, request.ID, session, requestCookie)
	for _, want := range []string{"Could we do &lt;b&gt;Tuesday&lt;/b&gt;?", "Afternoon is best.", "Tuesday works.", "They wrote", `data-live="poll"`} {
		if !strings.Contains(page, want) {
			t.Errorf("status page lacks %q", want)
		}
	}
	if strings.Contains(page, "<b>Tuesday</b>") {
		t.Error("a message was rendered as markup")
	}
}

func TestPostingNeedsAuthorshipAndTheSynchronizerToken(t *testing.T) {
	h := newHarness(t)
	token, _, session, request, requestCookie := h.threadHarness(t)

	if recorder := h.postMessage(token, request.ID, session, "", "hello", h.store.CSRFToken(session)); recorder.Code != http.StatusGone {
		t.Errorf("without proof of authorship status = %d, want 410", recorder.Code)
	}
	if recorder := h.postMessage(token, request.ID, session, requestCookie, "hello", "not-the-token"); recorder.Code != http.StatusForbidden {
		t.Errorf("without the synchronizer token status = %d, want 403", recorder.Code)
	}
	messages, err := h.store.ListMessages(context.Background(), request.ProfileID, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Errorf("a refused post was stored: %+v", messages)
	}
}

func TestLinkWithoutMessagesOffersNoThread(t *testing.T) {
	h := newHarness(t)
	token, profile := h.createProfile(Grants{WakingWindows: true, AllowRequests: true})
	session := h.sessionCookie(h.loginWithHeaders(token, testPasscode, map[string]string{"Sec-Fetch-Site": "same-origin"}))
	created, err := h.store.CreateRequest(context.Background(), profile, session, RequestInput{
		WindowStart: h.clock.Now().Add(3 * time.Hour), WindowEnd: h.clock.Now().Add(6 * time.Hour), ZoneID: "UTC",
	}, false, h.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := h.store.AuthorizeRequestSecret(context.Background(), profile.ID, created.Request.ID, created.Secret, h.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if page := h.statusPage(token, created.Request.ID, session, authorized.Session); strings.Contains(page, "Add a message") {
		t.Error("a link without the messages grant offers a message form")
	}
	if _, err := h.store.AppendMessage(context.Background(), profile, created.Request.ID, AuthorVisitor, "hi", h.clock.Now()); !errors.Is(err, ErrThreadClosed) {
		t.Errorf("append without the grant err = %v", err)
	}
}

func TestThreadClosesWhenTheRequestIsAnswered(t *testing.T) {
	h := newHarness(t)
	token, profile, session, request, requestCookie := h.threadHarness(t)
	if err := h.store.ApplyDecision(context.Background(), request.ID, RequestDeclined, time.Time{}, time.Time{}, h.clock.Now()); err != nil {
		t.Fatalf("decline: %v", err)
	}
	for _, author := range []string{AuthorVisitor, AuthorOwner} {
		if _, err := h.store.AppendMessage(context.Background(), profile, request.ID, author, "still there?", h.clock.Now()); !errors.Is(err, ErrThreadClosed) {
			t.Errorf("%s message after the answer err = %v", author, err)
		}
	}
	if page := h.statusPage(token, request.ID, session, requestCookie); strings.Contains(page, "Add a message") {
		t.Error("an answered request still offers a message form")
	}
}

func TestVisitorMessagesAreBoundedPerThreadPerDay(t *testing.T) {
	h := newHarness(t)
	_, profile, _, request, _ := h.threadHarness(t)
	for i := 0; i < MessagesPerThreadDay; i++ {
		if _, err := h.store.AppendMessage(context.Background(), profile, request.ID, AuthorVisitor, "again", h.clock.Now()); err != nil {
			t.Fatalf("message %d: %v", i, err)
		}
	}
	if _, err := h.store.AppendMessage(context.Background(), profile, request.ID, AuthorVisitor, "one more", h.clock.Now()); !errors.Is(err, ErrMessageLimit) {
		t.Errorf("message past the daily bound err = %v", err)
	}
	// The owner is not rate-limited in their own thread.
	if _, err := h.store.AppendMessage(context.Background(), profile, request.ID, AuthorOwner, "noted", h.clock.Now()); err != nil {
		t.Errorf("owner reply refused: %v", err)
	}
}

func TestMessageValidation(t *testing.T) {
	for name, body := range map[string]string{
		"empty":     "  \n\t ",
		"too long":  strings.Repeat("x", MaxMessageRunes+1),
		"only ctrl": "\x00\x01\x7f",
	} {
		if _, err := ValidateMessage(body); !errors.Is(err, ErrMessageInvalid) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	cleaned, err := ValidateMessage("line one\r\n\r\n\r\n\r\nline two\x00\tend")
	if err != nil || cleaned != "line one\n\nline two end" {
		t.Errorf("cleaned = %q, %v", cleaned, err)
	}
}

// Retention: a decided request's messages stay readable for fourteen days,
// then the maintenance job deletes them; the owner may erase them sooner.
func TestThreadsAreDeletedAfterRetentionOrOnErasure(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	_, profile, _, request, _ := h.threadHarness(t)
	if _, err := h.store.AppendMessage(ctx, profile, request.ID, AuthorVisitor, "hello", h.clock.Now()); err != nil {
		t.Fatal(err)
	}
	decidedAt := h.clock.Now()
	if err := h.store.ApplyDecision(ctx, request.ID, RequestDeclined, time.Time{}, time.Time{}, decidedAt); err != nil {
		t.Fatal(err)
	}
	if err := h.store.PurgeExpired(ctx, decidedAt.Add(ThreadRetention-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if messages, _ := h.store.ListMessages(ctx, profile.ID, request.ID); len(messages) != 1 {
		t.Fatalf("messages within retention = %d, want 1", len(messages))
	}
	if err := h.store.PurgeExpired(ctx, decidedAt.Add(ThreadRetention+time.Hour)); err != nil {
		t.Fatal(err)
	}
	if messages, _ := h.store.ListMessages(ctx, profile.ID, request.ID); len(messages) != 0 {
		t.Errorf("messages after retention = %d, want 0", len(messages))
	}

	_, profile2, _, request2, _ := h.threadHarness(t)
	if _, err := h.store.AppendMessage(ctx, profile2, request2.ID, AuthorVisitor, "hi", h.clock.Now()); err != nil {
		t.Fatal(err)
	}
	if err := h.store.EraseThread(ctx, profile2.ID, request2.ID); err != nil {
		t.Fatal(err)
	}
	if messages, _ := h.store.ListMessages(ctx, profile2.ID, request2.ID); len(messages) != 0 {
		t.Errorf("messages after erasure = %d, want 0", len(messages))
	}
}

// A message body sealed for one thread does not open as another's.
func TestMessageBodiesAreBoundToTheirThread(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	_, profile, _, request, _ := h.threadHarness(t)
	if _, err := h.store.AppendMessage(ctx, profile, request.ID, AuthorVisitor, "secret", h.clock.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.db.ExecContext(ctx, `UPDATE portal_messages SET author = ?`, AuthorOwner); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.ListMessages(ctx, profile.ID, request.ID); err == nil {
		t.Error("a message whose author was rewritten still decrypted")
	}
}
