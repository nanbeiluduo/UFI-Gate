//go:build !windows

package main

import (
	"testing"
)

func TestUpdateLockExclusion(t *testing.T) {
	oldDir := dir
	dir = t.TempDir()
	defer func() { dir = oldDir }()
	first, e := acquireUpdateLock()
	if e != nil {
		t.Fatal(e)
	}
	if second, e := acquireUpdateLock(); e == nil {
		second.Close()
		first.Close()
		t.Fatal("concurrent update allowed")
	}
	first.Close()
	third, e := acquireUpdateLock()
	if e != nil {
		t.Fatal(e)
	}
	third.Close()
}
