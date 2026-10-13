//go:build !windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func detachUpdater(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
func checkUpdateSpace(stage string) error {
	var s syscall.Statfs_t
	if e := syscall.Statfs(dir, &s); e != nil {
		return e
	}
	var size int64
	entries, e := os.ReadDir(stage)
	if e != nil {
		return e
	}
	for _, f := range entries {
		i, e := f.Info()
		if e != nil {
			return e
		}
		size += i.Size()
	}
	if uint64(s.Bavail)*uint64(s.Bsize) < uint64(size*3+(8<<20)) {
		return errors.New("可用空间不足，需容纳备份与更新文件")
	}
	return nil
}
