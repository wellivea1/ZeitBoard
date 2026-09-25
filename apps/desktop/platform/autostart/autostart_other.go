//go:build !windows

package autostart

type unsupportedController struct{}

func newPlatformController(string) Controller             { return unsupportedController{} }
func (unsupportedController) Read() (Registration, error) { return Registration{}, nil }
func (unsupportedController) Set(bool, bool) error        { return ErrUnsupported }
