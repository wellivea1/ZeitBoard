//go:build !windows

package tray

type unsupportedController struct{}

func newPlatformController() Controller             { return unsupportedController{} }
func (unsupportedController) Start(Callbacks) error { return nil }
func (unsupportedController) Stop() error           { return nil }
