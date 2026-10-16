package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAdminResetPreservesNetworkAndReinitializes(t *testing.T) {
	g := testGate(t)
	g.db.Config.AdminHash = "old-hash"
	g.db.Config.AdminName = "old-admin"
	g.db.Config.Enabled = true
	d := g.device(g.db.Config.AdminMAC)
	d.Trusted, d.Up, d.ManualRateKBps = true, 900, 256
	d.Until = time.Now().Add(time.Hour).Unix()
	before := *d
	g.sessions["old-session"] = Session{time.Now().Unix(), d.IP}
	oldToken := g.bootToken
	demo = false
	g.fw.installed = true
	var commands []string
	g.fw.runner = func(bin string, args ...string) (string, error) {
		commands = append(commands, bin+" "+strings.Join(args, " "))
		return "", nil
	}
	token, err := g.resetAdmin()
	if err != nil {
		t.Fatal(err)
	}
	if token == oldToken || token == "" || g.db.Config.AdminHash != "" || g.db.Config.AdminName != "" || len(adminDeviceMACs(g.db.Config)) != 0 || len(g.sessions) != 0 {
		t.Fatal("reset left admin credentials or sessions")
	}
	if !g.db.Config.Enabled || !reflect.DeepEqual(before, *d) || !g.allowed(d) {
		t.Fatal("reset changed network authorization, quotas or counters")
	}
	for _, cmd := range commands {
		if strings.Contains(cmd, "UFIG_F") || strings.Contains(cmd, "UFIG_N") || strings.Contains(cmd, "UFIG_C") || strings.Contains(cmd, "-t nat") {
			t.Fatal("reset touched network forwarding: " + cmd)
		}
	}
	if !strings.Contains(strings.Join(commands, "\n"), "--dport 8081 -j ACCEPT") {
		t.Fatal("setup port not opened")
	}
	reloaded, err := loadGate()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.db.Config.AdminHash != "" || !reflect.DeepEqual(*reloaded.db.Devices[d.MAC], before) {
		t.Fatal("reset not persisted correctly")
	}
	// A different hotspot device can initialize; old token is rejected.
	realRun = func(string, ...string) (string, error) {
		return "192.168.0.23 dev lo lladdr 02:00:00:00:00:23 REACHABLE", nil
	}
	setup := func(tok string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://127.0.0.1:8081/admin-api/setup", strings.NewReader(`{"Username":"new-admin","Password":"AdminPass123","Token":"`+tok+`"}`))
		r.RemoteAddr = "192.168.0.23:5000"
		r.Header.Set("Origin", "http://127.0.0.1:8081")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		g.adminHandler().ServeHTTP(w, r)
		if w.Code == 200 {
			w = finishRecovery(t, g, w, "192.168.0.23")
		}
		return w
	}
	if w := setup(oldToken); w.Code != 403 {
		t.Fatal("old token accepted", w.Code)
	}
	if w := setup(token); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if !g.isAdminDevice("02:00:00:00:00:23") || g.isAdminDevice(d.MAC) || !reflect.DeepEqual(before, *d) {
		t.Fatal("new owner or original network device incorrect")
	}
	if w := setup(token); w.Code != 403 {
		t.Fatal("token replay accepted")
	}
}

func TestAdminResetFailureRollsBack(t *testing.T) {
	g := testGate(t)
	g.db.Config.AdminHash = "old"
	g.db.Config.Enabled = true
	before := g.db.Config
	oldToken := g.bootToken
	demo = false
	g.fw.installed = true
	g.fw.runner = func(string, ...string) (string, error) { return "", errors.New("iptables failed") }
	if _, err := g.resetAdmin(); err == nil {
		t.Fatal("failed firewall operation reported success")
	}
	if !reflect.DeepEqual(before, g.db.Config) || g.bootToken != oldToken {
		t.Fatal("reset did not roll back")
	}
	data, err := os.ReadFile(filepath.Join(dir, "setup.token"))
	if err != nil || string(data) != oldToken {
		t.Fatal("token rollback failed")
	}
}

func TestAdminDevicePermissions(t *testing.T) {
	g := testGate(t)
	g.db.Config.AdminHash = "hash"
	owner := g.db.Config.AdminMAC
	other := "02:00:00:00:00:33"
	d := g.device(other)
	d.IP = "192.168.0.33"
	d.Trusted = true
	before := *d
	if err := g.setAdminDevice(other, true); err != nil {
		t.Fatal(err)
	}
	if !g.isAdminDevice(owner) || !g.isAdminDevice(other) {
		t.Fatal("multiple admins missing")
	}
	g.sessions["other"] = Session{time.Now().Unix(), d.IP}
	if err := g.setAdminDevice(other, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := g.sessions["other"]; ok {
		t.Fatal("removed device session survived")
	}
	if !reflect.DeepEqual(before, *d) {
		t.Fatal("admin permissions changed Internet authorization")
	}
	if err := g.setAdminDevice(owner, false); err == nil {
		t.Fatal("last admin removed")
	}
	// Even with authentication disabled, a valid old cookie cannot bypass the device list.
	demo = false
	realRun = func(string, ...string) (string, error) {
		return "192.168.0.33 dev lo lladdr " + other + " REACHABLE", nil
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1:8081/admin-api/config", nil)
	r.RemoteAddr = "192.168.0.33:5000"
	g.sessions["stolen"] = Session{time.Now().Unix(), d.IP}
	r.AddCookie(&http.Cookie{Name: "ug_session", Value: "stolen"})
	w := httptest.NewRecorder()
	g.adminAPI(w, r)
	if w.Code != 403 {
		t.Fatal("removed admin accessed API", w.Code)
	}
}

func TestIndependentRateLimit(t *testing.T) {
	g := testGate(t)
	d := g.device(g.db.Config.AdminMAC)
	d.Trusted = true
	d.Up = 100
	quota := d.Quota
	set := func(rate int) int {
		return call(g.adminHandler(), "/admin-api/device", "POST", map[string]any{"MAC": d.MAC, "Action": "rate", "RateKBps": rate}, true, g).Code
	}
	if set(256) != 200 || g.effectiveRate(d) != 256 || g.state(d) != "limited" {
		t.Fatal("manual rate not applied")
	}
	if d.Quota != quota || d.Up != 100 || !d.Trusted {
		t.Fatal("rate changed quota or authorization")
	}
	if set(-1) == 200 || set(102401) == 200 || d.ManualRateKBps != 256 {
		t.Fatal("invalid rate accepted")
	}
	d.Quota.Action = "slow"
	d.Quota.RateKBps = 128
	d.LimitReason = "total"
	if g.effectiveRate(d) != 128 {
		t.Fatal("stricter quota limit not used")
	}
	if set(64) != 200 || g.effectiveRate(d) != 64 {
		t.Fatal("stricter manual limit not used")
	}
	if set(0) != 200 || g.effectiveRate(d) != 128 || d.LimitReason != "total" {
		t.Fatal("cancel manual rate cleared quota limit")
	}
	d.ManualRateKBps = 256
	d.Quota.Action = "block"
	if g.allowed(d) || g.effectiveRate(d) != 0 {
		t.Fatal("manual rate bypassed quota block")
	}
	d.LimitReason = ""
	d.Trusted = false
	d.Until = 0
	if g.allowed(d) || g.effectiveRate(d) != 0 {
		t.Fatal("manual rate granted Internet access")
	}
}

func TestManagementFirewallAndRateRules(t *testing.T) {
	g := testGate(t)
	demo = false
	g.db.Config.Enabled = true
	g.db.Config.AdminHash = "hash"
	g.db.Config.OffloadOriginal = "1"
	var commands []string
	runner := func(bin string, args ...string) (string, error) {
		commands = append(commands, bin+" "+strings.Join(args, " "))
		if bin == "settings" && contains(args, "get") {
			return "1", nil
		}
		return "", nil
	}
	realRun = runner
	g.fw.runner = runner
	d := g.device(g.db.Config.AdminMAC)
	d.Trusted = true
	d.ManualRateKBps = 256
	if err := g.reconcile(); err != nil {
		t.Fatal(err)
	}
	if strings.Count(strings.Join(commands, "\n"), "--hashlimit-above 256kb/s") != 2 {
		t.Fatal("manual rate missing in upload/download rules")
	}
	if err := g.setAdminDevice("02:00:00:00:00:33", true); err != nil {
		t.Fatal(err)
	}
	active := "UFIG_IA"
	if g.fw.inputSide == 1 {
		active = "UFIG_IB"
	}
	commands = nil
	if err := g.reconcile(); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(commands, "\n")
	if strings.Contains(joined, "-F "+active) {
		t.Fatal("full reconcile flushed active INPUT after admin-only update")
	}
	if !strings.Contains(joined, "--mac-source 02:00:00:00:00:33 -j ACCEPT") {
		t.Fatal("additional admin missing from firewall")
	}
}
