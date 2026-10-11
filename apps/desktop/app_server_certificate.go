package main

import (
	"errors"
	"io"
	"os"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"non24.app/core/tlstrust"
)

// The certificate the owner chooses for their own server (ADR-0055). The
// desktop trusts it beside the system's roots, for that server only, so a
// server with its own certificate or authority is reached with verification
// on rather than skipped.

var openServerCertificateDialog = runtime.OpenFileDialog

// ServerCertificateDTO is a chosen certificate, PEM, with what a person checks
// it by. A certificate is public, so a status carries it too, and enrolling
// again keeps it unless the owner removes it.
type ServerCertificateDTO struct {
	Canceled    bool   `json:"canceled,omitempty"`
	PEM         string `json:"pem,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Subject     string `json:"subject,omitempty"`
	ExpiresAt   string `json:"expiresAt,omitempty"`
}

// ChooseServerCertificate opens a certificate file, PEM or DER, and says what
// it holds.
func (a *App) ChooseServerCertificate() (ServerCertificateDTO, error) {
	path, err := openServerCertificateDialog(a.applicationContext(), runtime.OpenDialogOptions{
		Title: "Choose your server's certificate",
		Filters: []runtime.FileFilter{
			{DisplayName: "Certificates (*.pem, *.crt, *.cer)", Pattern: "*.pem;*.crt;*.cer"},
		},
	})
	if err != nil {
		return ServerCertificateDTO{}, err
	}
	if path == "" {
		return ServerCertificateDTO{Canceled: true}, nil
	}
	data, err := readCertificateFile(path)
	if err != nil {
		return ServerCertificateDTO{}, err
	}
	certificate, err := tlstrust.PEM(data)
	if err != nil {
		return ServerCertificateDTO{}, err
	}
	return *describedServerCertificate(certificate), nil
}

func readCertificateFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("The certificate file could not be opened.")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, tlstrust.MaxPEMBytes+1))
	if err != nil {
		return nil, errors.New("The certificate file could not be read.")
	}
	if len(data) > tlstrust.MaxPEMBytes {
		return nil, errors.New("The file is too large to be a certificate.")
	}
	return data, nil
}

// describedServerCertificate is what a status says of a trusted certificate:
// the first one in the file, the server's own or its authority. Nil for none.
func describedServerCertificate(certificatePEM string) *ServerCertificateDTO {
	if certificatePEM == "" {
		return nil
	}
	certificates, err := tlstrust.Certificates([]byte(certificatePEM))
	if err != nil {
		return nil
	}
	first := certificates[0]
	subject := first.Subject.CommonName
	if subject == "" {
		subject = first.Subject.String()
	}
	return &ServerCertificateDTO{
		PEM:         certificatePEM,
		Fingerprint: tlstrust.Fingerprint(first),
		Subject:     subject,
		ExpiresAt:   first.NotAfter.UTC().Format(time.RFC3339),
	}
}
