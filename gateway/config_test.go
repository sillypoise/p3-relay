package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Validate generated identities and reject malformed, mismatched, expired and oversized inputs.
// Copies isolate each mutation so rejected input cannot affect the valid fixture or its peers.
func TestConfiguration(t *testing.T) {
	base := testConfiguration(t)
	now := time.Now()
	if err := base.validate(now); err != nil {
		t.Fatal("valid fixture rejected", err)
	}
	other := testConfiguration(t)
	cases := []struct {
		name   string
		change func(*configuration)
	}{
		{"missing hostname", func(v *configuration) { v.hostname = "" }},
		{"invalid hostname", func(v *configuration) { v.hostname = "bad_name" }},
		{"wrong hostname", func(v *configuration) { v.hostname = "other.test" }},
		{"oversized hostname", func(v *configuration) { v.hostname = strings.Repeat("a", 254) }},
		{"short password", func(v *configuration) { v.runtimePassword = strings.Repeat("a", 63) }},
		{"long password", func(v *configuration) { v.runtimePassword = strings.Repeat("a", 65) }},
		{"password injection", func(v *configuration) { v.runtimePassword = "\"\nadmin" }},
		{"shared password", func(v *configuration) { v.migrationPassword = v.runtimePassword }},
		{"missing certificate", func(v *configuration) { v.certificate = "" }},
		{"bad certificate", func(v *configuration) { v.certificate = "not a certificate" }},
		{"oversized PEM", func(v *configuration) {
			v.backendCA = strings.Repeat("a", pemBytesMax+1)
		}},
		{"bad CA", func(v *configuration) { v.backendCA = "not a CA" }},
		{"extra certificate", func(v *configuration) { v.certificate += v.certificate }},
		{"wrong key", func(v *configuration) { v.privateKey = other.privateKey }},
		{"extra key", func(v *configuration) { v.privateKey += v.privateKey }},
		{"missing migration password", func(v *configuration) { v.migrationPassword = "" }},
	}
	for _, entry := range cases {
		t.Run(entry.name, func(t *testing.T) {
			value := *base
			entry.change(&value)
			if err := value.validate(now); err == nil {
				t.Fatal("invalid input accepted")
			}
		})
	}
	if err := base.validate(now.Add(2 * time.Hour)); err == nil {
		t.Fatal("expired certificate accepted")
	}
	if err := base.validate(now.Add(-2 * time.Hour)); err == nil {
		t.Fatal("not-yet-valid certificate accepted")
	}
	base.backendCA += strings.Repeat("\n", pemBytesMax-len(base.backendCA))
	if err := base.validate(now); err != nil {
		t.Fatal("maximum PEM size rejected")
	}
}

// Boundary instants use a single identity for both hops to avoid fixture-generation clock skew.
func TestCertificateValidityBounds(t *testing.T) {
	value := testConfiguration(t)
	value.backendCA = value.certificate
	certificate, err := parseCertificate(value.certificate)
	if err != nil {
		t.Fatal(err)
	}
	for _, instant := range []time.Time{
		certificate.NotBefore, certificate.NotAfter.Add(-time.Nanosecond),
	} {
		if err := value.validate(instant); err != nil {
			t.Fatal("valid boundary rejected", err)
		}
	}
	for _, instant := range []time.Time{
		certificate.NotBefore.Add(-time.Nanosecond), certificate.NotAfter,
	} {
		if err := value.validate(instant); err == nil {
			t.Fatal("excluded boundary accepted")
		}
	}
}

// Backend CA failures must be checked independently while the frontend identity remains valid.
func TestBackendCAConstraints(t *testing.T) {
	value := testConfiguration(t)
	now := time.Now()
	for _, kind := range []string{"expired", "future", "not CA", "cannot sign"} {
		t.Run(kind, func(t *testing.T) {
			certificate, err := parseCertificate(value.certificate)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "expired":
				certificate.NotBefore = now.Add(-3 * time.Hour)
				certificate.NotAfter = now.Add(-2 * time.Hour)
			case "future":
				certificate.NotBefore = now.Add(2 * time.Hour)
				certificate.NotAfter = now.Add(3 * time.Hour)
			case "not CA":
				certificate.IsCA = false
			case "cannot sign":
				certificate.KeyUsage = x509.KeyUsageDigitalSignature
			default:
				t.Fatal("unknown fixture case")
			}
			// The parsed template must not retain the previous signer's public key.
			certificate.PublicKey = nil
			configuration := *value
			configuration.backendCA, _ = signTestIdentity(t, certificate)
			if err := configuration.validate(now); err == nil {
				t.Fatal("invalid backend CA accepted")
			}
		})
	}
}

// X.509 permits a CA with no key-usage extension; absence is not an explicit prohibition.
func TestBackendCAUnrestrictedUsage(t *testing.T) {
	value := testConfiguration(t)
	certificate, err := parseCertificate(value.backendCA)
	if err != nil {
		t.Fatal(err)
	}
	certificate.PublicKey = nil
	certificate.KeyUsage = 0
	value.backendCA, _ = signTestIdentity(t, certificate)
	if err := value.validate(time.Now()); err != nil {
		t.Fatal("unrestricted CA rejected", err)
	}
}

// Private startup files are complete and owner-only; acquisition failure creates no partial state.
func TestPrepareFiles(t *testing.T) {
	base := testConfiguration(t)
	parent := t.TempDir()
	directory, err := prepareFiles(parent, base)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Fatal("incomplete runtime directory")
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatal("runtime file is not owner-only")
		}
	}
	users, err := os.ReadFile(filepath.Join(directory, "users.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(users), "\n") != 2 {
		t.Fatal("auth allowlist must have exactly two roles")
	}
	if _, err := prepareFiles(filepath.Join(parent, "absent", "directory"), base); err == nil {
		t.Fatal("missing parent accepted")
	}
	remaining, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 {
		t.Fatal("failed preparation left partial state")
	}
}

func testConfiguration(t *testing.T) *configuration {
	t.Helper()
	certificate, key := testIdentity(t, "gateway.test")
	backend, _ := testIdentity(t, "postgres.railway.internal")
	return &configuration{
		certificate: certificate, privateKey: key, backendCA: backend, hostname: "gateway.test",
		runtimePassword:   strings.Repeat("a", 64),
		migrationPassword: strings.Repeat("b", 64),
	}
}

func testIdentity(t *testing.T, hostname string) (string, string) {
	t.Helper()
	now := time.Now()
	certificate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: hostname},
		DNSNames: []string{hostname}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	return signTestIdentity(t, certificate)
}

func signTestIdentity(t *testing.T, certificate *x509.Certificate) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal("test key generation failed")
	}
	encoded, err := x509.CreateCertificate(
		rand.Reader, certificate, certificate, &key.PublicKey, key)
	if err != nil {
		t.Fatal("test certificate generation failed")
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal("test key encoding failed")
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: encoded})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}))
}
