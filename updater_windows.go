//go:build windows

package main

import (
	"errors"
	"os/exec"
)

func detachUpdater(c *exec.Cmd)           {}
func checkUpdateSpace(stage string) error { return errors.New("Windows 不支持安装设备服务") }
