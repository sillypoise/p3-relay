package postgres

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Optional empty trust preserves local behavior. Configured trust requires verify-full, rejects
// fallback and malformed input without mutation, and preserves stricter protocol requirements.
func TestConfigureTLS(t *testing.T) {
	certificate := testDatabaseCA(t, &x509.Certificate{IsCA: true})
	for _, mode := range []string{
		"disable", "allow", "prefer", "require", "verify-ca", "verify-full",
	} {
		t.Run(mode, func(t *testing.T) {
			configuration, err := pgx.ParseConfig("host=gateway.test sslmode=" + mode)
			if err != nil {
				t.Fatal("fixture config rejected")
			}
			original := configuration.TLSConfig
			if err := ConfigureTLS(configuration, &TLSOptions{}); err != nil {
				t.Fatal("optional local settings rejected")
			}
			if configuration.TLSConfig != original {
				t.Fatal("optional settings mutated")
			}
			err = ConfigureTLS(configuration, &TLSOptions{CAPEM: certificate, Required: "true"})
			if mode != "verify-full" {
				if err == nil {
					t.Fatal("insecure TLS mode accepted")
				}
				if configuration.TLSConfig != original {
					t.Fatal("rejection mutated configuration")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if configuration.TLSConfig == original {
				t.Fatal("TLS configuration was not cloned")
			}
			if configuration.TLSConfig.RootCAs == nil {
				t.Fatal("custom trust missing")
			}
			if configuration.TLSConfig.MinVersion != tls.VersionTLS12 {
				t.Fatal("TLS minimum missing")
			}
			configuration.TLSConfig.MinVersion = tls.VersionTLS13
			if err := ConfigureTLS(configuration, &TLSOptions{CAPEM: certificate}); err != nil {
				t.Fatal(err)
			}
			if configuration.TLSConfig.MinVersion != tls.VersionTLS13 {
				t.Fatal("TLS downgraded")
			}
		})
	}
}

// Failed validation must never partially replace trust or enable an alternate connection target.
func TestConfigureTLSFailures(t *testing.T) {
	certificate := testDatabaseCA(t, &x509.Certificate{IsCA: true})
	cases := []struct {
		name    string
		options TLSOptions
		change  func(*pgx.ConnConfig)
	}{
		{name: "missing required CA", options: TLSOptions{Required: "true"}},
		{name: "invalid requirement", options: TLSOptions{Required: "TRUE"}},
		{name: "malformed", options: TLSOptions{CAPEM: "sensitive-invalid-input"}},
		{name: "fallback", options: TLSOptions{CAPEM: certificate},
			change: func(c *pgx.ConnConfig) {
				c.Fallbacks = []*pgconn.FallbackConfig{{Host: "other.test"}}
			}},
		{name: "obsolete maximum", options: TLSOptions{CAPEM: certificate},
			change: func(c *pgx.ConnConfig) { c.TLSConfig.MaxVersion = tls.VersionTLS11 }},
	}
	for _, entry := range cases {
		t.Run(entry.name, func(t *testing.T) {
			configuration, err := pgx.ParseConfig("host=gateway.test sslmode=verify-full")
			if err != nil {
				t.Fatal(err)
			}
			if entry.change != nil {
				entry.change(configuration)
			}
			original := configuration.TLSConfig
			err = ConfigureTLS(configuration, &entry.options)
			if err == nil {
				t.Fatal("invalid configuration accepted")
			}
			if strings.Contains(err.Error(), "sensitive-invalid-input") {
				t.Fatal("input leaked")
			}
			if configuration.TLSConfig != original {
				t.Fatal("rejection changed TLS settings")
			}
		})
	}
}

// Check the exact bundle count/byte limits and malformed trailing content, not just a valid root.
func TestDatabaseCABundle(t *testing.T) {
	certificate := testDatabaseCA(t, &x509.Certificate{IsCA: true})
	other := testDatabaseCA(t, &x509.Certificate{IsCA: true})
	for _, content := range []string{
		certificate, certificate + other,
		certificate + strings.Repeat("\n", databaseCABytesMax-len(certificate)),
	} {
		if _, err := databaseCARoots(content, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	for _, content := range []string{
		"", " ", certificate + other + certificate,
		certificate + "garbage", "garbage" + certificate,
		certificate + strings.Repeat("\n", databaseCABytesMax-len(certificate)+1),
		"-----BEGIN PRIVATE KEY-----\nAA==\n-----END PRIVATE KEY-----",
	} {
		if _, err := databaseCARoots(content, time.Now()); err == nil {
			t.Fatal("invalid bundle accepted")
		}
	}
}

// Certificate boundaries and signing restrictions are independent from PEM framing.
func TestDatabaseCAValidity(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	template := &x509.Certificate{IsCA: true, NotBefore: now, NotAfter: now.Add(time.Hour)}
	certificate := testDatabaseCA(t, template)
	for _, instant := range []time.Time{now, template.NotAfter.Add(-time.Nanosecond)} {
		if _, err := databaseCARoots(certificate, instant); err != nil {
			t.Fatal(err)
		}
	}
	for _, instant := range []time.Time{now.Add(-time.Nanosecond), template.NotAfter} {
		if _, err := databaseCARoots(certificate, instant); err == nil {
			t.Fatal("invalid time accepted")
		}
	}
	for _, invalid := range []*x509.Certificate{
		{IsCA: false}, {IsCA: true, KeyUsage: x509.KeyUsageDigitalSignature},
	} {
		if _, err := databaseCARoots(testDatabaseCA(t, invalid), now); err == nil {
			t.Fatal("invalid CA authority accepted")
		}
	}
}

func testDatabaseCA(t *testing.T, template *x509.Certificate) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal("test key creation failed")
	}
	// Copy options so fixture defaulting cannot change a caller's boundary timestamps.
	certificate := *template
	certificate.BasicConstraintsValid = true
	certificate.SerialNumber = big.NewInt(1)
	if certificate.NotBefore.IsZero() {
		certificate.NotBefore = time.Now().Add(-time.Hour)
	}
	if certificate.NotAfter.IsZero() {
		certificate.NotAfter = time.Now().Add(time.Hour)
	}
	encoded, err := x509.CreateCertificate(
		rand.Reader, &certificate, &certificate, &key.PublicKey, key)
	if err != nil {
		t.Fatal("test certificate creation failed")
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: encoded}))
}
