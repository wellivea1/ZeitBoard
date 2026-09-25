// Package autostart manages only ZeitBoard's per-user login registration.
package autostart

import "errors"

var ErrUnsupported = errors.New("login startup is unavailable on this platform")

type Registration struct {
	Available      bool
	Registered     bool
	MatchesCurrent bool
	StartHidden    bool
}

type Controller interface {
	Read() (Registration, error)
	Set(enabled, hidden bool) error
}

func New(executable string) Controller { return newPlatformController(executable) }
