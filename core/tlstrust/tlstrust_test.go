package tlstrust

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newServer serves over a self-signed certificate of its own: every
// httptest server shares one built-in certificate, which would let any of
// them pass for another.
func newServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "synthetic owner server"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

func certificatePEM(server *httptest.Server) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
}

func get(t *testing.T, server *httptest.Server, trusted string) error {
	t.Helper()
	config, err := Config(trusted)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: config}}
	defer client.CloseIdleConnections()
	response, err := client.Get(server.URL)
	if err == nil {
		response.Body.Close()
	}
	return err
}

func TestTheOwnersCertificateIsTrustedAndNothingElseIs(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	owners := newServer(t, ok)
	another := newServer(t, ok)

	// The system's roots alone do not know a self-signed server.
	if err := get(t, owners, ""); err == nil {
		t.Fatal("a self-signed server was trusted without its certificate")
	}
	if err := get(t, owners, certificatePEM(owners)); err != nil {
		t.Fatalf("the owner's certificate was not trusted: %v", err)
	}
	// Trusting one server's certificate trusts no other.
	if err := get(t, another, certificatePEM(owners)); err == nil {
		t.Fatal("another server was trusted on the owner's certificate")
	}
}

func TestOnlyCertificatesAreAccepted(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	good := certificatePEM(server)
	if certificates, err := Certificates([]byte(good)); err != nil || len(certificates) != 1 {
		t.Fatalf("a certificate was refused: %v", err)
	}
	key := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("synthetic")}))
	for name, bad := range map[string]string{
		"empty":                 "",
		"not PEM":               "not a certificate",
		"a private key":         good + key,
		"text after the chain":  good + "trailing words",
		"an unreadable payload": string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("synthetic")})),
		"far too large":         strings.Repeat(good, 40),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Certificates([]byte(bad)); err == nil {
				t.Fatal("something other than a certificate was accepted")
			}
			if _, err := Config(bad); bad != "" && err == nil {
				t.Fatal("a configuration was made from it")
			}
		})
	}
}

func TestTheFingerprintIsTheCertificatesSHA256(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	certificate := server.Certificate()
	sum := sha256.Sum256(certificate.Raw)
	want := strings.ToUpper(strings.Join(strings.Split(fmt.Sprintf("% x", sum[:]), " "), ":"))
	if got := Fingerprint(certificate); got != want || len(got) != 95 {
		t.Fatalf("fingerprint = %s, want %s", got, want)
	}
}

func TestADERCertificateReadsAsPEM(t *testing.T) {
	server := newServer(t, http.NotFoundHandler())
	der := server.Certificate().Raw
	converted, err := PEM(der)
	if err != nil {
		t.Fatalf("a DER certificate was refused: %v", err)
	}
	if converted != certificatePEM(server) {
		t.Fatalf("converted = %q", converted)
	}
	// A PEM file reads back as itself, and junk is refused either way.
	if again, err := PEM([]byte(converted)); err != nil || again != converted {
		t.Fatalf("PEM round trip = %q, %v", again, err)
	}
	if _, err := PEM([]byte("neither PEM nor DER")); err == nil {
		t.Fatal("junk was accepted")
	}
}
