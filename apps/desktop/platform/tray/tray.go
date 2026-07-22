package tray

import "errors"

var ErrNotificationsUnavailable = errors.New("desktop notifications are unavailable")

type Callbacks struct {
	Show func()
	Quit func()
}

type Controller interface {
	Start(Callbacks) error
	Notify(title, message string) error
	Stop() error
}

func New() Controller { return newPlatformController() }
