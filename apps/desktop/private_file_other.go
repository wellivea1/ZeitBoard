//go:build !windows

package main

import "os"

func replaceFileAtomic(from, to string) error {
	return os.Rename(from, to)
}
