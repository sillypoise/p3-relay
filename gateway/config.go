package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"regexp"
	"strings"
	"time"
)

// Three bounded PEM inputs fit comfortably within the 128 MiB startup/container budget.
const pemBytesMax = 16 * 1024

var hostnamePattern = regexp.MustCompile(
	`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`,
)
var passwordPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type configuration struct {
	certificate       string
	privateKey        string
	backendCA         string
	hostname          string
	runtimePassword   string
	migrationPassword string
}

// The timestamp is intentionally copied to freeze one verification instant across both identities.
func (value *configuration) validate(now time.Time) error {
	if value == nil {
		panic("gateway configuration required")
	}
	if len(value.hostname) > 253 || !hostnamePattern.MatchString(value.hostname) {
		return errors.New("invalid gateway hostname")
	}
	if !passwordPattern.MatchString(value.runtimePassword) {
		return errors.New("invalid runtime password format")
	}
	if !passwordPattern.MatchString(value.migrationPassword) {
		return errors.New("invalid migration password format")
	}
	if value.runtimePassword == value.migrationPassword {
		return errors.New("database roles require distinct credentials")
	}
	for _, content := range [...]string{value.certificate, value.privateKey, value.backendCA} {
		if len(content) == 0 || len(content) > pemBytesMax {
			return errors.New("missing or oversized TLS material")
		}
	}
	if err := value.validateServer(now); err != nil {
		return err
	}
	certificate, err := parseCertificate(value.backendCA)
	if err != nil {
		return errors.New("invalid backend CA")
	}
	if !certificate.IsCA {
		return errors.New("backend certificate is not a CA")
	}
	// An omitted key-usage extension is unrestricted under X.509, not a signing prohibition.
	if certificate.KeyUsage != 0 && certificate.KeyUsage&x509.KeyUsageCertSign == 0 {
		return errors.New("backend CA excludes certificate signing")
	}
	if now.Before(certificate.NotBefore) || !now.Before(certificate.NotAfter) {
		return errors.New("backend CA outside validity period")
	}
	return nil
}

func (value *configuration) validateServer(now time.Time) error {
	certificate, err := parseCertificate(value.certificate)
	if err != nil {
		return errors.New("invalid gateway certificate")
	}
	block, rest := pem.Decode([]byte(value.privateKey))
	if block == nil || block.Type != "PRIVATE KEY" || strings.TrimSpace(string(rest)) != "" {
		return errors.New("one PKCS8 private key required")
	}
	if _, err := tls.X509KeyPair([]byte(value.certificate), []byte(value.privateKey)); err != nil {
		return errors.New("invalid gateway key pair")
	}
	// Startup uses a half-open validity interval, rejecting even the exact expiry instant.
	if now.Before(certificate.NotBefore) || !now.Before(certificate.NotAfter) {
		return errors.New("gateway certificate outside validity period")
	}
	// The initial gateway uses one self-signed trust anchor, distributed to its clients separately.
	if err := certificate.CheckSignatureFrom(certificate); err != nil {
		return errors.New("gateway certificate must be self-signed CA")
	}
	roots := x509.NewCertPool()
	roots.AddCert(certificate)
	if _, err := certificate.Verify(x509.VerifyOptions{
		Roots: roots, DNSName: value.hostname, CurrentTime: now,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}); err != nil {
		return errors.New("gateway certificate identity or validity rejected")
	}
	return nil
}

func parseCertificate(content string) (*x509.Certificate, error) {
	block, rest := pem.Decode([]byte(content))
	if block == nil || block.Type != "CERTIFICATE" || strings.TrimSpace(string(rest)) != "" {
		return nil, errors.New("one PEM certificate required")
	}
	return x509.ParseCertificate(block.Bytes)
}
