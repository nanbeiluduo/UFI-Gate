package main

import (
	"errors"
	"os"
)

// Demo/testing only. Production install/update is restricted to Linux ARM.
func lockFile(f *os.File) error {
	if demo {
		return nil
	}
	return errors.New("Windows 仅支持演示模式")
}
