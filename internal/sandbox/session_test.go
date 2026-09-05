package sandbox

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// Exercise integrity and exact expiry boundaries without depending on wall-clock time.
func TestSessionLifecycle(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	now := time.Unix(1_800_000_000, 0)
	cookie, id, err := Issue(key, now)
	if err != nil {
		t.Fatal(err)
	}
	if cookie.Name != "__Host-relay_sandbox" || cookie.Path != "/" || cookie.Domain != "" {
		t.Fatal("cookie scope must be host-only")
	}
	if cookie.Secure == false || cookie.HttpOnly == false || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("cookie security attributes missing")
	}
	if cookie.MaxAge != 1800 || cookie.Expires.Equal(now.Add(SessionLifetime)) == false {
		t.Fatal("cookie lifetime differs from credential lifetime")
	}
	for _, offset := range []time.Duration{0, SessionLifetime - time.Second} {
		actual, err := Validate(key, cookie.Value, now.Add(offset))
		if err != nil || actual != id {
			t.Fatalf("valid session rejected: %v", err)
		}
	}
	for _, offset := range []time.Duration{-time.Second, SessionLifetime, SessionLifetime + time.Second} {
		if _, err := Validate(key, cookie.Value, now.Add(offset)); err == nil {
			t.Fatalf("invalid time boundary accepted: %s", offset)
		}
	}
}

// Malformed or forged credentials must never produce an authorized visitor identifier.
func TestSessionRejectsForgery(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	now := time.Unix(1_800_000_000, 0)
	cookie, _, err := Issue(key, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{
		"", "Bearer operator-token", strings.Repeat("x", tokenLength),
		cookie.Value + "x", cookie.Value[:107], "z" + cookie.Value[1:],
		cookie.Value[:44] + strings.Repeat("0", 64),
	} {
		id, err := Validate(key, token, now)
		if err == nil || id != "" {
			t.Fatal("malformed credential accepted")
		}
	}
	if _, err := Validate([]byte(strings.Repeat("b", 32)), cookie.Value, now); err == nil {
		t.Fatal("wrong signing key accepted")
	}
	if _, _, err := Issue(nil, now); err == nil {
		t.Fatal("missing signing key accepted")
	}
}
