package main

import (
	"encoding/json"
	"errors"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func f50Network() NetworkInventory {
	return NetworkInventory{Complete: true, RouteKnown: true, Interfaces: []NetworkInterface{
		{Name: "wlan0", Up: true, Complete: true, Addresses: []string{}},
		{Name: "br0", Up: true, Complete: true, Addresses: []string{"192.168.0.1/24"}},
		{Name: "rmnet_data0", Up: true, Complete: true, Uplink: true, Addresses: []string{"10.1.2.3/30"}},
		{Name: "lo", Up: true, Loopback: true, Complete: true, Addresses: []string{"127.0.0.1/8"}},
	}}
}
func mockNetwork(t *testing.T, n NetworkInventory) {
	t.Helper()
	old := inspectNetwork
	inspectNetwork = func() NetworkInventory { return n }
	t.Cleanup(func() { inspectNetwork = old })
}
func TestNetworkValidation(t *testing.T) {
	n := f50Network()
	for _, c := range []struct{ lan, ip, status, code string }{{"wlan0", "192.168.0.1", "invalid", "gateway_mismatch"}, {"br0", "192.168.0.1", "valid", "ok"}, {"absent", "192.168.0.1", "invalid", "interface_missing"}, {"br0", "192.168.0.2", "invalid", "gateway_mismatch"}, {"br0;reboot", "192.168.0.1", "invalid", "interface_format"}, {"br0", "192.168.0.1$(id)", "invalid", "gateway_format"}, {"rmnet_data0", "10.1.2.3", "invalid", "interface_unsuitable"}} {
		v := validateNetwork(n, c.lan, c.ip)
		if v.Status != c.status || v.Code != c.code {
			t.Fatalf("%+v: %+v", c, v)
		}
	}
	if v := validateNetwork(NetworkInventory{}, "br0", "192.168.0.1"); v.Status != "unknown" {
		t.Fatal(v)
	}
	n.Interfaces[1].Up = false
	if v := validateNetwork(n, "br0", "192.168.0.1"); v.Code != "interface_down" {
		t.Fatal(v)
	}
	if !strings.Contains(validateNetwork(f50Network(), "wlan0", "192.168.0.1").Message, "192.168.0.1 当前配置在 br0，而不是 wlan0") {
		t.Fatal("F50 diagnosis missing")
	}
}
func TestNetworkAndroidTextFallback(t *testing.T) {
	oldI, oldR := networkInterfaces, realRun
	t.Cleanup(func() { networkInterfaces = oldI; realRun = oldR })
	networkInterfaces = func() ([]net.Interface, error) { return nil, errors.New("permission denied") }
	realRun = func(_ string, args ...string) (string, error) {
		if strings.Contains(strings.Join(args, " "), "addr") {
			return "1: lo: <LOOPBACK,UP> mtu 65536\n    inet 127.0.0.1/8 scope host lo\n3: br0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500\n    inet 192.168.0.1/24 brd 192.168.0.255 scope global br0\n4: upstream@parent: <UP> mtu 1500\n    inet 10.1.1.2/24 scope global upstream", nil
		}
		return "default via 10.1.1.1 dev upstream table 100", nil
	}
	n := systemNetwork()
	if !n.Complete || !n.RouteKnown || validateNetwork(n, "br0", "192.168.0.1").Status != "valid" {
		t.Fatal(n)
	}
	if validateNetwork(n, "upstream", "10.1.1.2").Code != "interface_unsuitable" {
		t.Fatal(n)
	}
	realRun = func(string, ...string) (string, error) { return "", errors.New("ip unavailable") }
	n = systemNetwork()
	if n.Complete || validateNetwork(n, "br0", "192.168.0.1").Status != "unknown" {
		t.Fatal(n)
	}
}
func TestNetworkReportConflictAndRedaction(t *testing.T) {
	g := testGate(t)
	g.db.Config.LAN = "wlan0"
	g.db.Config.Gateway = "192.168.0.1"
	g.db.Config.AdminHash = "SECRET_HASH"
	g.db.Config.RecoveryHash = "SECRET_RECOVERY"
	g.db.Config.SigningKey = "SECRET_SIGN"
	g.db.Config.Push.Secret = "SECRET_PUSH"
	g.save()
	os.WriteFile(filepath.Join(dir, "bootstrap.json"), []byte(`{"LAN":"br0","Gateway":"192.168.0.1"}`), 0600)
	mockNetwork(t, f50Network())
	r := networkReport("192.168.0.1", "wlan0", "192.168.0.1")
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "SECRET_") || r.Recommended == nil || r.Recommended.LAN != "br0" || r.SavedValidation.Code != "gateway_mismatch" || !strings.Contains(strings.Join(r.Warnings, " "), "不一致") {
		t.Fatal(string(b))
	}
	if networkReport("203.0.113.3", "", "").Recommended != nil {
		t.Fatal("unverified browser hint trusted")
	}
	n := f50Network()
	n.Interfaces = append(n.Interfaces, NetworkInterface{Name: "other", Up: true, Complete: true, Addresses: []string{"192.168.0.1/24"}})
	inspectNetwork = func() NetworkInventory { return n }
	if networkReport("192.168.0.1", "", "").Recommended != nil {
		t.Fatal("ambiguous recommendation")
	}
}
func TestNetworkIdentityFailureGuidance(t *testing.T) {
	g := testGate(t)
	g.db.Config.AdminHash = "HASH"
	demo = false
	g.db.Config.LAN = "wlan0"
	g.db.Config.Gateway = "192.168.0.1"
	mockNetwork(t, f50Network())
	realRun = func(string, ...string) (string, error) { return "", nil }
	r := httptest.NewRequest("GET", "http://192.168.0.1/", nil)
	r.RemoteAddr = "192.168.0.99:12345"
	_, e := g.peerMAC(r)
	if e == nil || e.Error() != networkRepairHint {
		t.Fatal(e)
	}
	g.db.Config.LAN = "br0"
	_, e = g.peerMAC(r)
	if e == nil || !strings.Contains(e.Error(), "未找到当前设备") {
		t.Fatal(e)
	}
	realRun = func(string, ...string) (string, error) { return "", errors.New("denied") }
	_, e = g.peerMAC(r)
	if e == nil || !strings.Contains(e.Error(), "权限不足") {
		t.Fatal(e)
	}
	if g.adminAddressAllowed(r.RemoteAddr) {
		t.Fatal("identity failure granted admin access")
	}
}

func TestNetworkMaintenanceBlocksBrowserWrites(t *testing.T) {
	g := testGate(t)
	os.WriteFile(filepath.Join(dir, "network-journal.json"), []byte(`{}`), 0600)
	w := call(g.adminHandler(), "/admin-api/toggle", "POST", map[string]bool{"Enabled": true}, true, g)
	if w.Code != 503 || g.db.Config.Enabled {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestNetworkIdentityNeverUsesOtherInterface(t *testing.T) {
	g := testGate(t)
	g.db.Config.LAN = "wlan0"
	g.db.Config.Gateway = "192.168.0.1"
	mockNetwork(t, f50Network())
	realRun = func(string, ...string) (string, error) {
		return "192.168.0.99 dev br0 lladdr 02:00:00:00:00:99 REACHABLE", nil
	}
	r := httptest.NewRequest("POST", "http://192.168.0.1:8081/admin-api/setup", strings.NewReader(`{"MAC":"02:00:00:00:00:99"}`))
	r.RemoteAddr = "192.168.0.99:12345"
	if mac, e := g.peerMAC(r); e == nil || mac != "" {
		t.Fatal("cross-interface identity accepted", mac, e)
	}
}
