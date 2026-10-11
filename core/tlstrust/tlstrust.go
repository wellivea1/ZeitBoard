// Package tlstrust is how a ZeitBoard client trusts the owner's own server
// (ADR-0055): the system's roots, plus one certificate the owner chose (the
// server's own certificate, or the authority that issued it). Verification is
// never turned off by it; the server must still present a valid certificate
// for the address the client was given.
package tlstrust

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

// MaxPEMBytes bounds a chosen certificate file. A chain of a few certificates
// is a few kilobytes; anything far larger is not one.
const MaxPEMBytes = 64 * 1024

const maxCertificates = 8

// Certificates reads the certificates in a PEM file. It refuses a file with
// no certificate, with anything but certificates in it (a private key above
// all), or with more than a short chain.
func Certificates(data []byte) ([]*x509.Certificate, error) {
	if len(data) > MaxPEMBytes {
		return nil, errors.New("the certificate file is too large to be a certificate")
	}
	var certificates []*x509.Certificate
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("the file holds a %s, not only certificates", strings.ToLower(block.Type))
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("a certificate in the file could not be read: %w", err)
		}
		certificates = append(certificates, certificate)
		if len(certificates) > maxCertificates {
			return nil, errors.New("the file holds more certificates than a server's chain")
		}
	}
	if len(certificates) == 0 {
		return nil, errors.New("the file holds no PEM certificate")
	}
	if strings.TrimSpace(string(rest)) != "" {
		return nil, errors.New("the file holds something other than PEM certificates")
	}
	return certificates, nil
}

// PEM reads a chosen certificate file, PEM or a single DER certificate as
// Windows often saves them, and returns its certificates as PEM.
func PEM(data []byte) (string, error) {
	certificates, err := Certificates(data)
	if err != nil {
		der, derErr := x509.ParseCertificate(data)
		if derErr != nil || len(data) > MaxPEMBytes {
			return "", err
		}
		certificates = []*x509.Certificate{der}
	}
	var encoded strings.Builder
	for _, certificate := range certificates {
		encoded.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw}))
	}
	return encoded.String(), nil
}

// Fingerprint is the SHA-256 of a certificate, as "AB:CD:…", the form servers
// and certificate tools print.
func Fingerprint(certificate *x509.Certificate) string {
	sum := sha256.Sum256(certificate.Raw)
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(parts, ":")
}

// Config is a client TLS configuration that trusts the system's roots and,
// when certificatePEM is not empty, the certificates in it as well.
func Config(certificatePEM string) (*tls.Config, error) {
	config := &tls.Config{MinVersion: tls.VersionTLS12}
	if strings.TrimSpace(certificatePEM) == "" {
		return config, nil
	}
	certificates, err := Certificates([]byte(certificatePEM))
	if err != nil {
		return nil, err
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	for _, certificate := range certificates {
		pool.AddCert(certificate)
	}
	config.RootCAs = pool
	return config, nil
}
