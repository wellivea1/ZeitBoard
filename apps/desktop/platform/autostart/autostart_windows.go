//go:build windows

package autostart

import (
	"errors"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

type registryController struct {
	path       string
	name       string
	executable string
}

func newPlatformController(executable string) Controller {
	return registryController{path: runKey, name: "ZeitBoard", executable: executable}
}

func startupCommand(executable string, hidden bool) (string, error) {
	if !filepath.IsAbs(executable) || strings.ContainsAny(executable, "\"\r\n\x00") || !strings.EqualFold(filepath.Ext(executable), ".exe") {
		return "", errors.New("login startup requires an absolute Windows executable path")
	}
	command := `"` + filepath.Clean(executable) + `"`
	if hidden {
		command += " --background"
	}
	// Windows Run values document a maximum command length of 260 characters.
	if len(utf16.Encode([]rune(command))) > 260 {
		return "", errors.New("the executable path is too long for Windows login startup")
	}
	return command, nil
}

func (c registryController) Read() (Registration, error) {
	result := Registration{Available: true}
	key, err := registry.OpenKey(registry.CURRENT_USER, c.path, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	defer key.Close()
	value, _, err := key.GetStringValue(c.name)
	if errors.Is(err, registry.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	result.Registered = true
	for _, hidden := range []bool{false, true} {
		command, err := startupCommand(c.executable, hidden)
		if err == nil && value == command {
			result.MatchesCurrent = true
			result.StartHidden = hidden
			break
		}
	}
	return result, nil
}

func (c registryController) Set(enabled, hidden bool) error {
	if !enabled {
		key, err := registry.OpenKey(registry.CURRENT_USER, c.path, registry.SET_VALUE)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer key.Close()
		err = key.DeleteValue(c.name)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	command, err := startupCommand(c.executable, hidden)
	if err != nil {
		return err
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, c.path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(c.name, command)
}
