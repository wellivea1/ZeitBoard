package tray

type Callbacks struct {
	Show func()
	Quit func()
}

type Controller interface {
	Start(Callbacks) error
	Stop() error
}

func New() Controller { return newPlatformController() }
