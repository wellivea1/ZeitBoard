//go:build !windows

package efs

func platformCheck(string) (Status, error) { return Unsupported, nil }

func platformEncrypt(string) error { return ErrUnsupported }

func platformDecrypt(string) error { return ErrUnsupported }
