package chatgpt

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"time"
)

// clockSkew is how far apart this server's clock and OpenAI's may be.
const clockSkew = 5 * time.Second

// identity is what a verified ID token says of the account signed in to.
type identity struct {
	Subject string
	Email   string
}

// verifyIDToken checks an ID token as OpenID Connect asks (Core §3.1.3.7):
// signed with RS256 by a key the issuer publishes, issued by it to this
// client, current, and, on a sign-in, answering this sign-in's nonce. It
// reads only the subject and email.
func (a *Account) verifyIDToken(ctx context.Context, d discovery, raw, clientID, nonce string, at time.Time) (identity, error) {
	invalid := fail(SignInEnded, "invalid_id_token")
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return identity{}, invalid
	}
	var header struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	if decodeSegment(parts[0], &header) != nil || header.Algorithm != "RS256" {
		return identity{}, invalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return identity{}, invalid
	}
	key, err := a.signingKey(ctx, d.JWKSURI, header.KeyID)
	if err != nil {
		return identity{}, err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature) != nil {
		return identity{}, invalid
	}

	var claims struct {
		Issuer    string          `json:"iss"`
		Audience  json.RawMessage `json:"aud"`
		Party     *string         `json:"azp"`
		Subject   string          `json:"sub"`
		Expires   *float64        `json:"exp"`
		IssuedAt  *float64        `json:"iat"`
		NotBefore *float64        `json:"nbf"`
		Nonce     string          `json:"nonce"`
		Email     string          `json:"email"`
	}
	if decodeSegment(parts[1], &claims) != nil {
		return identity{}, invalid
	}
	audiences, ok := audienceOf(claims.Audience)
	switch {
	case claims.Issuer != d.Issuer, !ok, !slices.Contains(audiences, clientID),
		strings.TrimSpace(claims.Subject) == "", claims.Expires == nil, claims.IssuedAt == nil:
		return identity{}, invalid
	case claims.Party != nil && *claims.Party != clientID, len(audiences) > 1 && claims.Party == nil:
		return identity{}, invalid
	case !at.Before(unixTime(*claims.Expires).Add(clockSkew)), at.Add(clockSkew).Before(unixTime(*claims.IssuedAt)):
		return identity{}, invalid
	case claims.NotBefore != nil && at.Add(clockSkew).Before(unixTime(*claims.NotBefore)):
		return identity{}, invalid
	case nonce != "" && claims.Nonce != nonce:
		return identity{}, invalid
	}
	email := strings.TrimSpace(claims.Email)
	if len(email) > 254 || strings.ContainsFunc(email, isControl) {
		email = ""
	}
	return identity{Subject: claims.Subject, Email: email}, nil
}

func decodeSegment(segment string, target any) error {
	data, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// audienceOf reads "aud", which is one string or a list of them.
func audienceOf(raw json.RawMessage) ([]string, bool) {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return []string{one}, true
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil && len(many) > 0 {
		return many, true
	}
	return nil, false
}

func unixTime(seconds float64) time.Time {
	return time.Unix(0, int64(seconds*float64(time.Second)))
}

// signingKey finds the issuer's key a token names, reading the issuer's key
// set again once when it does not hold it, as after the issuer rotates its
// keys. Not reaching the key set is Busy: it says nothing of the sign-in.
func (a *Account) signingKey(ctx context.Context, jwksURI, keyID string) (*rsa.PublicKey, error) {
	for attempt := range 2 {
		a.mu.Lock()
		keys := a.keys
		if a.keysFrom != jwksURI || attempt > 0 {
			keys = nil
		}
		a.mu.Unlock()
		if keys == nil {
			read, err := a.readKeySet(ctx, jwksURI)
			if err != nil {
				return nil, err
			}
			a.mu.Lock()
			a.keys, a.keysFrom = read, jwksURI
			a.mu.Unlock()
			keys = read
		}
		if key := pickKey(keys, keyID); key != nil {
			return key, nil
		}
	}
	return nil, fail(SignInEnded, "invalid_id_token")
}

// pickKey is the key named, or the only key when the token names none.
func pickKey(keys map[string]*rsa.PublicKey, keyID string) *rsa.PublicKey {
	if keyID != "" {
		return keys[keyID]
	}
	if len(keys) == 1 {
		for _, key := range keys {
			return key
		}
	}
	return nil
}

// readKeySet reads the issuer's RSA signing keys, by key id.
func (a *Account) readKeySet(ctx context.Context, jwksURI string) (map[string]*rsa.PublicKey, error) {
	unavailable := fail(Busy, "identity_verification_unavailable")
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURI, nil)
	if err != nil {
		return nil, unavailable
	}
	request.Header.Set("Accept", "application/json")
	status, body, err := a.send(request)
	if err != nil || status != http.StatusOK {
		return nil, unavailable
	}
	var set struct {
		Keys []struct {
			Type      string `json:"kty"`
			KeyID     string `json:"kid"`
			Use       string `json:"use"`
			Algorithm string `json:"alg"`
			Modulus   string `json:"n"`
			Exponent  string `json:"e"`
		} `json:"keys"`
	}
	if json.Unmarshal(body, &set) != nil || set.Keys == nil {
		return nil, unavailable
	}
	keys := map[string]*rsa.PublicKey{}
	for _, jwk := range set.Keys {
		if jwk.Type != "RSA" || (jwk.Use != "" && jwk.Use != "sig") || (jwk.Algorithm != "" && jwk.Algorithm != "RS256") {
			continue
		}
		key, err := rsaKey(jwk.Modulus, jwk.Exponent)
		if err != nil {
			continue
		}
		keys[jwk.KeyID] = key
	}
	if len(keys) == 0 {
		return nil, unavailable
	}
	return keys, nil
}

// rsaKey builds a public key from a JWK's modulus and exponent, refusing one
// shorter than 2048 bits.
func rsaKey(modulus, exponent string) (*rsa.PublicKey, error) {
	n, err := base64.RawURLEncoding.DecodeString(modulus)
	if err != nil {
		return nil, err
	}
	e, err := base64.RawURLEncoding.DecodeString(exponent)
	if err != nil || len(e) == 0 || len(e) > 4 {
		return nil, errors.New("invalid RSA exponent")
	}
	key := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	if key.N.BitLen() < 2048 || key.E < 3 || key.E%2 == 0 {
		return nil, errors.New("RSA key is too weak")
	}
	return key, nil
}
