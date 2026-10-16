package auth_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"regexp"
	"testing"

	"non24.app/server/internal/auth"
)

// A device ID is "device_" and 96 random bits in hex, and no two are alike.
func TestDeviceIDsAreRandomAndShapedForTheContract(t *testing.T) {
	shape := regexp.MustCompile(`^device_[0-9a-f]{24}$`)
	seen := map[string]bool{}
	for range 64 {
		id, err := auth.NewDeviceID()
		if err != nil || !shape.MatchString(id) || seen[id] {
			t.Fatalf("device ID %q, %v, repeated %v", id, err, seen[id])
		}
		seen[id] = true
	}
}

// A device token is 256 random bits, URL-safe without padding, and no two
// are alike.
func TestDeviceTokensCarry256RandomBits(t *testing.T) {
	seen := map[string]bool{}
	for range 64 {
		token, err := auth.NewToken()
		if err != nil {
			t.Fatal(err)
		}
		raw, decodeErr := base64.RawURLEncoding.DecodeString(token)
		if len(token) != 43 || decodeErr != nil || len(raw) != 32 || seen[token] {
			t.Fatalf("a token of %d characters: %v, repeated %v", len(token), decodeErr, seen[token])
		}
		seen[token] = true
	}
}

// The server keeps a token's SHA-256, never the token: the same token always
// finds its device, and another never does.
func TestATokenIsKeptAsItsHash(t *testing.T) {
	kept := auth.HashToken("synthetic-token-one")
	want := sha256.Sum256([]byte("synthetic-token-one"))
	if !bytes.Equal(kept, want[:]) {
		t.Fatalf("kept %x, not the token's SHA-256", kept)
	}
	if !bytes.Equal(auth.HashToken("synthetic-token-one"), kept) {
		t.Fatal("the same token hashed differently")
	}
	if bytes.Equal(auth.HashToken("synthetic-token-two"), kept) {
		t.Fatal("two tokens share a hash")
	}
}

// Only the bearer scheme, exactly as the clients send it, gives a token, and
// one of only spaces is none.
func TestOnlyABearerTokenIsTaken(t *testing.T) {
	for header, want := range map[string]string{
		"Bearer synthetic-token":       "synthetic-token",
		"Bearer   synthetic-token  ":   "synthetic-token",
		"":                             "",
		"Bearer":                       "",
		"Bearer ":                      "",
		"Bearer    ":                   "",
		"Basic c3ludGhldGljOnNlY3JldA": "",
		"synthetic-token":              "",
		" Bearer synthetic-token":      "",
	} {
		token, ok := auth.BearerToken(header)
		if token != want || ok != (want != "") {
			t.Errorf("%q gave %q, %v", header, token, ok)
		}
	}
}

// The enrollment secret matches only itself, compared in constant time; an
// empty secret matches nothing, not even another empty one.
func TestTheEnrollmentSecretMatchesOnlyItself(t *testing.T) {
	const secret = "synthetic-enrollment-secret"
	for _, attempt := range []struct {
		got, want string
		matches   bool
	}{
		{secret, secret, true},
		{"synthetic-enrollment-secreT", secret, false},
		{secret + "-and-more", secret, false},
		{secret[:9], secret, false},
		{"", secret, false},
		{secret, "", false},
		{"", "", false},
	} {
		if auth.SecretMatches(attempt.got, attempt.want) != attempt.matches {
			t.Errorf("%q against %q matched %v", attempt.got, attempt.want, !attempt.matches)
		}
	}
}
