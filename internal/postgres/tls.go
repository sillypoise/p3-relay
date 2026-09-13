package postgres

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Two 16 KiB anchors allow an old/new trust overlap without an unbounded bundle.
const databaseCABytesMax = 32 * 1024
const databaseCACountMax = 2

type TLSOptions struct {
	CAPEM    string
	Required string
}

// ConfigureTLS changes only a not-yet-used connection configuration. Rejection leaves it unchanged.
// Empty optional trust preserves local configuration; deployed tasks explicitly require trust.
func ConfigureTLS(configuration *pgx.ConnConfig, options *TLSOptions) error {
	if configuration == nil {
		panic("database connection configuration required")
	}
	if options == nil {
		panic("database TLS options required")
	}
	switch options.Required {
	case "", "false", "true":
	default:
		return errors.New("invalid database CA requirement")
	}
	if options.CAPEM == "" {
		if options.Required == "true" {
			return errors.New("database CA required")
		}
		return nil
	}
	current := configuration.TLSConfig
	if current == nil || current.InsecureSkipVerify || current.ServerName == "" {
		return errors.New("database CA requires hostname-verified TLS")
	}
	if len(configuration.Fallbacks) != 0 {
		return errors.New("database TLS fallback targets are not supported")
	}
	minimumVersion := max(current.MinVersion, tls.VersionTLS12)
	if current.MaxVersion != 0 && current.MaxVersion < minimumVersion {
		return errors.New("database TLS maximum excludes required protocol versions")
	}
	roots, err := databaseCARoots(options.CAPEM, time.Now())
	if err != nil {
		return err
	}
	verified := current.Clone()
	verified.RootCAs = roots
	verified.MinVersion = minimumVersion
	configuration.TLSConfig = verified
	return nil
}

// Copy the timestamp intentionally so every anchor uses the same validation instant.
func databaseCARoots(content string, now time.Time) (*x509.CertPool, error) {
	if len(content) > databaseCABytesMax {
		return nil, errors.New("database CA bundle too large")
	}
	roots := x509.NewCertPool()
	remainder := strings.TrimSpace(content)
	var count uint32
	for count < databaseCACountMax && remainder != "" {
		if !strings.HasPrefix(remainder, "-----BEGIN CERTIFICATE-----") {
			return nil, errors.New("invalid database CA encoding")
		}
		block, rest := pem.Decode([]byte(remainder))
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			return nil, errors.New("invalid database CA encoding")
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, errors.New("invalid database CA certificate")
		}
		if !certificate.IsCA {
			return nil, errors.New("database trust anchor must be a CA")
		}
		if certificate.KeyUsage != 0 && certificate.KeyUsage&x509.KeyUsageCertSign == 0 {
			return nil, errors.New("database CA excludes certificate signing")
		}
		if now.Before(certificate.NotBefore) || !now.Before(certificate.NotAfter) {
			return nil, errors.New("database CA outside validity period")
		}
		roots.AddCert(certificate)
		count++
		remainder = strings.TrimSpace(string(rest))
	}
	if count == 0 || remainder != "" {
		return nil, errors.New("database CA bundle must contain one or two certificates")
	}
	return roots, nil
}
