// Package sandbox defines visitor credentials separately from operator credentials.
package sandbox

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	SessionLifetime = 30 * time.Minute
	cookieName      = "__Host-relay_sandbox"
	tokenLength     = 108 // 32 identifier characters, two separators, 10 seconds digits, 64 MAC characters.
)

var ErrSession = errors.New("invalid sandbox session")

// Issue creates a credential; callers must persist its ownership and quota before setting it.
func Issue(key []byte, now time.Time) (http.Cookie, string, error) {
	if len(key) != 32 {
		return http.Cookie{}, "", ErrSession
	}
	if now.Unix() < 1_000_000_000 || now.Unix() > 9_999_998_199 {
		return http.Cookie{}, "", ErrSession
	}
	var randomID [16]byte
	if _, err := rand.Read(randomID[:]); err != nil {
		return http.Cookie{}, "", err
	}
	id := hex.EncodeToString(randomID[:])
	expires := now.Add(SessionLifetime)
	payload := id + "." + strconv.FormatInt(expires.Unix(), 10)
	cookie := http.Cookie{
		Name: cookieName, Value: payload + "." + sessionMAC(key, payload),
		Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode,
		MaxAge: 1800, Expires: expires,
	}
	return cookie, id, nil
}

// Validate verifies integrity and expiry only; database authorization remains mandatory.
func Validate(key []byte, token string, now time.Time) (string, error) {
	if len(key) != 32 || len(token) != tokenLength {
		return "", ErrSession
	}
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return "", ErrSession
	}
	if len(parts[0]) != 32 || len(parts[1]) != 10 || len(parts[2]) != 64 {
		return "", ErrSession
	}
	if _, err := hex.DecodeString(parts[0]); err != nil {
		return "", ErrSession
	}
	expected := sessionMAC(key, parts[0]+"."+parts[1])
	if hmac.Equal([]byte(expected), []byte(parts[2])) == false {
		return "", ErrSession
	}
	expires, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", ErrSession
	}
	if expires <= now.Unix() || expires > now.Add(SessionLifetime).Unix() {
		return "", ErrSession
	}
	return parts[0], nil
}

func sessionMAC(key []byte, payload string) string {
	mac := hmac.New(sha256.New, key)
	// hash.Hash.Write is specified to return a nil error.
	_, err := mac.Write([]byte("relay-sandbox-v1:" + payload))
	if err != nil {
		panic("HMAC write invariant failed")
	}
	return hex.EncodeToString(mac.Sum(nil))
}
