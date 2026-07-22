//go:build !windows

package tray

import "fmt"

type unsupportedController struct{}

func newPlatformController() Controller             { return unsupportedController{} }
func (unsupportedController) Start(Callbacks) error { return nil }
func (unsupportedController) Notify(string, string) error {
	return fmt.Errorf("%w on this platform", ErrNotificationsUnavailable)
}
func (unsupportedController) Stop() error { return nil }
