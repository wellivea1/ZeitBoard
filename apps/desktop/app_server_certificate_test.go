package main

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"non24.app/core/tlstrust"
)

// chooseFile stubs the open dialog with one answer.
func chooseFile(t *testing.T, path string) {
	t.Helper()
	previous := openServerCertificateDialog
	openServerCertificateDialog = func(context.Context, runtime.OpenDialogOptions) (string, error) { return path, nil }
	t.Cleanup(func() { openServerCertificateDialog = previous })
}

func TestTheOwnerChoosesTheServersCertificateAsPEMOrDER(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	certificate := server.Certificate()
	wantPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw}))
	dir := t.TempDir()
	app := newTestApp(t)

	for name, contents := range map[string][]byte{
		"server.pem": []byte(wantPEM),
		"server.cer": certificate.Raw, // DER, as Windows often saves one
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatal(err)
		}
		chooseFile(t, path)
		chosen, err := app.ChooseServerCertificate()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if chosen.Canceled || chosen.PEM != wantPEM || chosen.Fingerprint != tlstrust.Fingerprint(certificate) || chosen.ExpiresAt == "" {
			t.Fatalf("%s chose %+v", name, chosen)
		}
	}

	chooseFile(t, "")
	if chosen, err := app.ChooseServerCertificate(); err != nil || !chosen.Canceled {
		t.Fatalf("canceling chose %+v, %v", chosen, err)
	}

	key := filepath.Join(dir, "server.key")
	if err := os.WriteFile(key, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("synthetic")}), 0o600); err != nil {
		t.Fatal(err)
	}
	chooseFile(t, key)
	if _, err := app.ChooseServerCertificate(); err == nil {
		t.Fatal("a private key was chosen as a certificate")
	}
}
