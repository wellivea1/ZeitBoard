package chatgpt

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// Problem is why ChatGPT could not do what was asked, in terms the owner can
// act on. The window words each one; nothing ChatGPT writes is ever shown.
type Problem string

const (
	// UsageLimit is the plan's limit reached. ChatGPT's settings say when it
	// resets; ZeitBoard does not guess.
	UsageLimit Problem = "usage_limit"
	// Busy is ChatGPT, or the way to it, not answering. Trying again later may
	// work.
	Busy Problem = "busy"
	// SignInEnded is a sign-in that no longer works. Signing in again restores
	// it.
	SignInEnded Problem = "sign_in_ended"
	// PlanNotShared is a sign-in that did not let ZeitBoard use the plan.
	PlanNotShared Problem = "plan_not_shared"
	// NotEligible is an account that cannot share its plan with apps.
	NotEligible Problem = "not_eligible"
	// NotEnabled is plan usage OpenAI has not turned on for this sign-in.
	NotEnabled Problem = "not_enabled"
	// ModelUnavailable is a chosen model the plan no longer offers.
	ModelUnavailable Problem = "model_unavailable"
	// AccountMismatch is a sign-in to another account than the one the
	// assistant used. Disconnecting first lets any account sign in.
	AccountMismatch Problem = "account_mismatch"
	// SignInExpired is a browser's answer to a sign-in that is no longer
	// waiting: it took too long, or another began since.
	SignInExpired Problem = "sign_in_expired"
	// Refused is a request ChatGPT refused as such: ZeitBoard's to fix.
	Refused Problem = "refused"
)

// Error is a failure ChatGPT answered with, or one of reaching it. It carries
// the code ChatGPT gave and the parameter it named, when they read as
// identifiers, and never a message, a body or a token.
type Error struct {
	Problem Problem
	Code    string
	Param   string
	Status  int
}

func (e *Error) Error() string {
	detail := e.Code
	if detail == "" && e.Status != 0 {
		detail = fmt.Sprintf("HTTP %d", e.Status)
	}
	if e.Param != "" {
		detail += " (" + e.Param + ")"
	}
	if detail == "" {
		return "chatgpt: " + string(e.Problem)
	}
	return "chatgpt: " + string(e.Problem) + ": " + detail
}

// fail is an Error with a code of ZeitBoard's own.
func fail(problem Problem, code string) *Error {
	return &Error{Problem: problem, Code: code}
}

// answered reads an error response: the code its body names, and the problem
// that code, or failing that the status, means.
func answered(status int, body []byte) *Error {
	code, param := errorCode(body)
	return &Error{Problem: classify(status, code), Code: code, Param: param, Status: status}
}

// classify gives the problem a code means, and the status's for a code it
// does not know. The "subscription_sharing_v2_" codes mean what the codes
// without "v2" do.
func classify(status int, code string) Problem {
	switch strings.Replace(code, "subscription_sharing_v2_", "subscription_sharing_", 1) {
	case "subscription_sharing_usage_limit_exceeded", "insufficient_quota":
		return UsageLimit
	case "subscription_sharing_usage_unavailable", "subscription_sharing_user_unavailable",
		"server_error", "rate_limit_exceeded", "identity_verification_unavailable":
		return Busy
	case "subscription_sharing_user_not_eligible":
		return NotEligible
	case "subscription_sharing_client_not_enabled":
		return NotEnabled
	case "chatpass_v2_scope_not_authorized":
		return PlanNotShared
	case "model_not_found":
		return ModelUnavailable
	case "subscription_sharing_invalid_user", "chatpass_v2_invalid_authorization_context", "invalid_client":
		return SignInEnded
	}
	if endsSignIn(code) {
		return SignInEnded
	}
	// A status of 0 is no answer at all: the network, or a stream cut short.
	if status == http.StatusTooManyRequests || status >= 500 || (status == 0 && code == "") {
		return Busy
	}
	return Refused
}

// endsSignIn reports whether a token endpoint's code says the refresh token
// is spent: only signing in again restores the grant.
func endsSignIn(code string) bool {
	switch code {
	case "invalid_grant", "invalid_refresh_token", "token_expired", "refresh_token_expired",
		"refresh_token_invalidated", "refresh_token_reused":
		return true
	}
	return false
}

var identifierPattern = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.:\[\]-]*$`)

func identifier(value string, limit int) bool {
	return len(value) <= limit && identifierPattern.MatchString(value)
}

// errorCode reads the code an error body names, and the parameter: in the
// innermost object reached through "error" or "detail", at most four deep,
// the "error" string OAuth answers with, or else "code".
func errorCode(body []byte) (code, param string) {
	var object map[string]any
	if json.Unmarshal(body, &object) != nil {
		return "", ""
	}
	for range 4 {
		inner, ok := object["error"].(map[string]any)
		if !ok {
			inner, ok = object["detail"].(map[string]any)
		}
		if !ok {
			break
		}
		object = inner
	}
	if text, ok := object["error"].(string); ok {
		code = text
	} else if text, ok := object["code"].(string); ok {
		code = text
	}
	if !identifier(code, 100) {
		code = ""
	}
	if text, ok := object["param"].(string); ok && identifier(text, 160) {
		param = text
	}
	return code, param
}
