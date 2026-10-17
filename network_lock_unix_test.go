//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNetworkMaintenanceSharesUpdateAndServiceLocks(t *testing.T) {
	repairFixture(t, false)
	lock, e := acquireUpdateLock()
	if e != nil {
		t.Fatal(e)
	}
	if _, e = runNetworkRepair("br0", "192.168.0.1", "locked", false); e == nil {
		t.Fatal("repair bypassed update lock")
	}
	lock.Close()
	lock, e = acquireServiceFile()
	if e != nil {
		t.Fatal(e)
	}
	if second, e := acquireServiceFile(); e == nil {
		second.Close()
		t.Fatal("service exclusion missing")
	}
	lock.Close()
	if _, e := os.Stat(filepath.Join(dir, "network-journal.json")); !os.IsNotExist(e) {
		t.Fatal("busy lock created transaction")
	}
}
