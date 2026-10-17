package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func repairFixture(t *testing.T, running bool) (*bool, *int) {
	t.Helper()
	g := testGate(t)
	g.db.Config.LAN = "wlan0"
	g.db.Config.Gateway = "192.168.0.1"
	g.db.Config.Enabled = true
	g.db.Config.AdminHash = "HASH"
	g.db.Config.RecoveryHash = "RECOVERY"
	g.db.Config.SigningKey = "SIGN"
	g.db.Config.Push.Secret = "PUSH"
	d := g.device("02:00:00:00:00:23")
	d.Up = 123
	d.Trusted = true
	d.Banned = true
	d.ManualRateKBps = 7
	d.Until = 9999999999
	g.save()
	b, _ := os.ReadFile(filepath.Join(dir, "state.json"))
	var raw map[string]json.RawMessage
	json.Unmarshal(b, &raw)
	raw["FutureBusiness"] = json.RawMessage(`{"preserve":[1,2,3]}`)
	b, _ = json.Marshal(raw)
	os.WriteFile(filepath.Join(dir, "state.json"), b, 0600)
	os.WriteFile(filepath.Join(dir, "bootstrap.json"), []byte(`{"LAN":"wlan0","Gateway":"192.168.0.1","future":"keep"}`), 0600)
	os.WriteFile(filepath.Join(dir, "setup.token"), []byte("KEEP_TOKEN"), 0600)
	mockNetwork(t, f50Network())
	oldS, oldH, oldW, oldSleep := networkService, networkHealth, networkWrite, networkSleep
	t.Cleanup(func() { networkService = oldS; networkHealth = oldH; networkWrite = oldW; networkSleep = oldSleep })
	live := running
	starts := 0
	networkSleep = func(time.Duration) {}
	networkHealth = func() (*NetworkConfig, error) {
		if !live {
			return nil, errors.New("stopped")
		}
		return readNetworkConfig("state.json", true)
	}
	networkService = func(_ string, action string) error {
		if action == "stop-network" {
			if live {
				b, e := networkState()
				if e != nil {
					return e
				}
				var r map[string]json.RawMessage
				json.Unmarshal(b, &r)
				r["FinalSample"] = json.RawMessage(`456`)
				b, _ = json.Marshal(r)
				os.WriteFile(filepath.Join(dir, "state.json"), b, 0600)
			}
			live = false
			return nil
		}
		if action == "start-network" {
			starts++
			live = true
			return nil
		}
		return errors.New("unexpected service command")
	}
	return &live, &starts
}
func normalizedBusiness(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var r map[string]any
	if e := json.Unmarshal(b, &r); e != nil {
		t.Fatal(e)
	}
	c := r["Config"].(map[string]any)
	delete(c, "LAN")
	delete(c, "Gateway")
	delete(c, "Enabled")
	delete(r, "FinalSample")
	return r
}
func TestNetworkRepairPreservesDataAndOrder(t *testing.T) {
	for _, running := range []bool{false, true} {
		t.Run(map[bool]string{true: "running", false: "stopped"}[running], func(t *testing.T) {
			live, _ := repairFixture(t, running)
			before, _ := os.ReadFile(filepath.Join(dir, "state.json"))
			originalWrite := networkWrite
			networkWrite = func(p string, b []byte) error {
				if filepath.Base(p) == "state.json" {
					if *live {
						t.Fatal("state replaced before worker stopped")
					}
				}
				return originalWrite(p, b)
			}
			r, e := runNetworkRepair("br0", "192.168.0.1", "test", false)
			if e != nil || r.Phase != "success" || !r.ServiceRunning || !r.AuthenticationDisabled {
				t.Fatal(r, e)
			}
			after, _ := os.ReadFile(filepath.Join(dir, "state.json"))
			if !reflect.DeepEqual(normalizedBusiness(t, before), normalizedBusiness(t, after)) {
				t.Fatal("business fields changed")
			}
			backup, _ := os.ReadFile(filepath.Join(r.Backup, "state.json"))
			if running && !strings.Contains(string(backup), "FinalSample") {
				t.Fatal("last sample missing")
			}
			c, _ := readNetworkConfig("state.json", true)
			if c.LAN != "br0" || c.Enabled {
				t.Fatal(c)
			}
			b, _ := os.ReadFile(filepath.Join(dir, "bootstrap.json"))
			if !strings.Contains(string(b), `"future": "keep"`) {
				t.Fatal(string(b))
			}
			token, _ := os.ReadFile(filepath.Join(dir, "setup.token"))
			if string(token) != "KEEP_TOKEN" {
				t.Fatal("setup token changed")
			}
			if networkPending() {
				t.Fatal("journal not closed")
			}
			if runtime.GOOS != "windows" {
				for _, name := range []string{"state.json", "bootstrap.json", "network-result.json"} {
					s, _ := os.Stat(filepath.Join(dir, name))
					if s.Mode().Perm() != 0600 {
						t.Fatal(name, s.Mode())
					}
				}
			}
		})
	}
}
func TestNetworkRepairFailuresRollback(t *testing.T) {
	for _, failure := range []string{"state-write", "bootstrap-write", "start", "health"} {
		t.Run(failure, func(t *testing.T) {
			_, starts := repairFixture(t, true)
			before, _ := os.ReadFile(filepath.Join(dir, "state.json"))
			originalWrite, originalService, originalHealth := networkWrite, networkService, networkHealth
			failed := false
			networkWrite = func(p string, b []byte) error {
				target := map[string]string{"state-write": "state.json", "bootstrap-write": "bootstrap.json"}[failure]
				if !failed && target != "" && p == filepath.Join(dir, target) {
					failed = true
					return errors.New("injected write failure")
				}
				return originalWrite(p, b)
			}
			networkService = func(p, a string) error {
				if failure == "start" && a == "start-network" && !failed {
					failed = true
					return errors.New("injected start failure")
				}
				return originalService(p, a)
			}
			networkHealth = func() (*NetworkConfig, error) {
				if failure == "health" && *starts == 1 {
					return nil, errors.New("injected health failure")
				}
				return originalHealth()
			}
			r, e := runNetworkRepair("br0", "192.168.0.1", "test", false)
			if e == nil || !r.ConfigRestored || !r.ServiceRunning || !r.AuthenticationDisabled || r.Phase != "rolled_back" {
				t.Fatal(r, e)
			}
			c, _ := readNetworkConfig("state.json", true)
			if c.LAN != "wlan0" || c.Enabled {
				t.Fatal(c)
			}
			after, _ := os.ReadFile(filepath.Join(dir, "state.json"))
			if !reflect.DeepEqual(normalizedBusiness(t, before), normalizedBusiness(t, after)) {
				t.Fatal("rollback lost business data")
			}
		})
	}
}
func TestNetworkRepairInterruptedRecovery(t *testing.T) {
	for _, phase := range []string{"stopping", "backed_up"} {
		t.Run(phase, func(t *testing.T) {
			repairFixture(t, false)
			j := NetworkJournal{ID: "interrupted", Phase: "stopping", Backup: filepath.Join(dir, "network-backup-interrupted"), LAN: "br0", Gateway: "192.168.0.1", HadBootstrap: true}
			os.Mkdir(j.Backup, 0700)
			saveNetworkJournal(j)
			if phase == "backed_up" {
				if e := backupNetwork(&j); e != nil {
					t.Fatal(e)
				}
				b, _ := os.ReadFile(filepath.Join(dir, "state.json"))
				b, _ = patchNetwork(b, true, "br0", "192.168.0.1")
				networkWrite(filepath.Join(dir, "state.json"), b)
			}
			r, e := runNetworkRepair("", "", "", true)
			if e == nil || !r.ConfigRestored || !r.AuthenticationDisabled || networkPending() {
				t.Fatal(r, e)
			}
			c, _ := readNetworkConfig("state.json", true)
			if c.LAN != "wlan0" {
				t.Fatal(c)
			}
		})
	}
}
func TestNetworkRepairRefusesInvalidStateAndPendingUpdate(t *testing.T) {
	repairFixture(t, false)
	original, _ := os.ReadFile(filepath.Join(dir, "state.json"))
	os.WriteFile(filepath.Join(dir, "update-journal.json"), []byte(`{}`), 0600)
	if _, e := runNetworkRepair("br0", "192.168.0.1", "test", false); e == nil {
		t.Fatal("concurrent update accepted")
	}
	os.Remove(filepath.Join(dir, "update-journal.json"))
	if _, e := runNetworkRepair("br0;id", "192.168.0.1", "test", false); e == nil {
		t.Fatal("shell syntax accepted")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "state.json"))
	if string(after) != string(original) {
		t.Fatal("preflight changed state")
	}
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"Config":`), 0600)
	if _, e := runNetworkRepair("br0", "192.168.0.1", "test", false); e == nil {
		t.Fatal("corrupt state accepted")
	}
	if networkPending() {
		t.Fatal("journal created for invalid state")
	}
}

func TestNetworkRepairRecoveryFailureAndRetry(t *testing.T) {
	repairFixture(t, false)
	original := networkService
	networkService = func(p, a string) error {
		if a == "start-network" {
			return errors.New("start unavailable")
		}
		return original(p, a)
	}
	r, e := runNetworkRepair("br0", "192.168.0.1", "failed-start", false)
	if e == nil || !r.ConfigRestored || r.ServiceRunning || !r.AuthenticationDisabled || !networkPending() {
		t.Fatal(r, e)
	}
	networkService = original
	r, e = runNetworkRepair("", "", "", true)
	if e == nil || !r.ConfigRestored || !r.ServiceRunning || networkPending() {
		t.Fatal(r, e)
	}
}

func TestNetworkRepairDoesNotReplaceWhenStopFails(t *testing.T) {
	repairFixture(t, true)
	before, _ := os.ReadFile(filepath.Join(dir, "state.json"))
	networkService = func(string, string) error { return errors.New("supervisor did not stop") }
	r, e := runNetworkRepair("br0", "192.168.0.1", "stop-failed", false)
	after, _ := os.ReadFile(filepath.Join(dir, "state.json"))
	if e == nil || r.ConfigRestored || string(before) != string(after) || !networkPending() {
		t.Fatal(r, e)
	}
}
