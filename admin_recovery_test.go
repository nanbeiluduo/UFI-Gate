package main

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func recoveryCall(g *Gate, action, ip string, value any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(value)
	r := httptest.NewRequest("POST", "http://127.0.0.1:8090/api/admin-access/"+action, strings.NewReader(string(b)))
	r.RemoteAddr = ip + ":4567"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://127.0.0.1:8090")
	w := httptest.NewRecorder()
	g.portalHandler().ServeHTTP(w, r)
	return w
}
func pendingTicket(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var v struct{ Ticket string }
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	if w.Code != 200 || v.Ticket == "" {
		t.Fatalf("expected pending ticket: %d %s", w.Code, w.Body.String())
	}
	return v.Ticket
}
func finishRecovery(t *testing.T, g *Gate, w *httptest.ResponseRecorder, ip string) *httptest.ResponseRecorder {
	t.Helper()
	ticket := pendingTicket(t, w)
	d := recoveryCall(g, "download", ip, map[string]any{"Ticket": ticket})
	if d.Code != 200 {
		t.Fatal(d.Body.String())
	}
	result := recoveryCall(g, "commit", ip, map[string]any{"Ticket": ticket, "Saved": true})
	if result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	return result
}
func TestRecoverySetupRequiresDownloadAndBindsClient(t *testing.T) {
	g := testGate(t)
	w := call(g.adminHandler(), "/admin-api/setup", "POST", map[string]string{"Username": "admin", "Password": "AdminPass123", "Token": g.bootToken}, false, g)
	ticket := pendingTicket(t, w)
	key := g.recoveries[ticket].Key
	if g.db.Config.AdminHash != "" || len(g.sessions) != 0 {
		t.Fatal("setup committed before download")
	}
	for _, saved := range []bool{false, true} {
		if recoveryCall(g, "commit", "127.0.0.1", map[string]any{"Ticket": ticket, "Saved": saved}).Code != 400 {
			t.Fatal("download bypass")
		}
	}
	realRun = func(string, ...string) (string, error) {
		return "192.168.0.23 dev lo lladdr 02:00:00:00:00:23 REACHABLE", nil
	}
	if recoveryCall(g, "download", "192.168.0.23", map[string]string{"Ticket": ticket}).Code != 403 {
		t.Fatal("ticket usable by another device")
	}
	w = finishRecovery(t, g, w, "127.0.0.1")
	if !passwordOK(g.db.Config.AdminHash, "AdminPass123") || g.db.Config.RecoveryHash != recoveryDigest(key) || g.bootToken != "" {
		t.Fatal("setup did not persist credentials")
	}
	if len(w.Result().Cookies()) == 0 || !w.Result().Cookies()[0].HttpOnly {
		t.Fatal("missing HttpOnly session")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "state.json"))
	if strings.Contains(string(raw), key) {
		t.Fatal("plaintext recovery key persisted")
	}
	if recoveryCall(g, "commit", "127.0.0.1", map[string]any{"Ticket": ticket, "Saved": true}).Code != 403 {
		t.Fatal("commit replay accepted")
	}
	config := call(g.adminHandler(), "/admin-api/config", "GET", nil, true, g)
	if strings.Contains(config.Body.String(), g.db.Config.RecoveryHash) {
		t.Fatal("recovery digest exposed in config")
	}
}
func TestPortalAdminLoginPreservesNetworkAndProtectsBackends(t *testing.T) {
	g := testGate(t)
	g.db.Config.AdminName = "admin"
	g.db.Config.AdminHash = passwordHash("AdminPass123")
	g.db.Config.RecoveryHash = recoveryDigest("old-key")
	g.db.Config.Enabled = true
	d := g.device("02:00:00:00:00:23")
	d.IP = "192.168.0.23"
	d.Up = 1234
	d.Banned = true
	d.ManualRateKBps = 256
	before := *d
	realRun = func(string, ...string) (string, error) {
		return "192.168.0.23 dev lo lladdr 02:00:00:00:00:23 REACHABLE", nil
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1:8081/", nil)
	r.RemoteAddr = "192.168.0.23:5678"
	if g.adminAddressAllowed(r.RemoteAddr) {
		t.Fatal("unknown MAC can access backend")
	}
	w := recoveryCall(g, "login", "192.168.0.23", map[string]string{"Username": "admin", "Password": "wrong"})
	if w.Code != 401 || g.isAdminDevice(d.MAC) {
		t.Fatal("wrong password granted access")
	}
	demo = false
	g.fw.installed = true
	var commands []string
	g.fw.runner = func(bin string, args ...string) (string, error) {
		commands = append(commands, strings.Join(args, " "))
		return "", nil
	}
	w = recoveryCall(g, "login", d.IP, map[string]string{"Username": "admin", "Password": "AdminPass123"})
	if w.Code != 200 || !g.adminAddressAllowed(r.RemoteAddr) || !reflect.DeepEqual(before, *d) {
		t.Fatal("login changed network state or failed", w.Body.String())
	}
	joined := strings.Join(commands, "\n")
	if !strings.Contains(joined, "--mac-source "+d.MAC+" -j ACCEPT") || strings.Contains(joined, "UFIG_F") || strings.Contains(joined, "UFIG_N") {
		t.Fatal("incorrect backend firewall update", joined)
	}
	if !strings.Contains(joined, "-j DROP") {
		t.Fatal("visitor isolation lost")
	}
}
func TestRecoveryRotationPersistenceAndReset(t *testing.T) {
	g := testGate(t)
	g.db.Config.AdminName = "admin"
	g.db.Config.AdminHash = passwordHash("OldPass123")
	g.db.Config.RecoveryHash = recoveryDigest("old-key")
	d := g.device(g.db.Config.AdminMAC)
	d.Up = 98
	d.Trusted = true
	before := *d
	g.sessions["old"] = Session{time.Now().Unix(), "127.0.0.1"}
	w := recoveryCall(g, "reset", "127.0.0.1", map[string]string{"RecoveryKey": "old-key", "NewPassword": "NewPass123"})
	id := pendingTicket(t, w)
	newKey := g.recoveries[id].Key
	if !passwordOK(g.db.Config.AdminHash, "OldPass123") {
		t.Fatal("password changed before download")
	}
	finishRecovery(t, g, w, "127.0.0.1")
	if _, ok := g.sessions["old"]; ok {
		t.Fatal("old session survived")
	}
	if !reflect.DeepEqual(before, *d) || !passwordOK(g.db.Config.AdminHash, "NewPass123") {
		t.Fatal("data changed or password not changed")
	}
	if recoveryCall(g, "reset", "127.0.0.1", map[string]string{"RecoveryKey": "old-key", "NewPassword": "Another123"}).Code != 401 {
		t.Fatal("old key reused")
	}
	loaded, err := loadGate()
	if err != nil {
		t.Fatal(err)
	}
	w = recoveryCall(loaded, "reset", "127.0.0.1", map[string]string{"RecoveryKey": newKey, "NewPassword": "Another123"})
	pendingTicket(t, w)
	if _, err = loaded.resetAdmin(); err != nil {
		t.Fatal(err)
	}
	if loaded.db.Config.RecoveryHash != "" || len(loaded.recoveries) != 0 || !reflect.DeepEqual(before, *loaded.db.Devices[d.MAC]) {
		t.Fatal("reset did not invalidate keys or preserve data")
	}
}
func TestRecoveryFailureExpiryCSRFAndGlobalLimit(t *testing.T) {
	g := testGate(t)
	g.db.Config.AdminName = "admin"
	g.db.Config.AdminHash = passwordHash("OldPass123")
	g.db.Config.RecoveryHash = recoveryDigest("old-key")
	makePending := func() (*httptest.ResponseRecorder, string) {
		w := recoveryCall(g, "reset", "127.0.0.1", map[string]string{"RecoveryKey": "old-key", "NewPassword": "NewPass123"})
		return w, pendingTicket(t, w)
	}
	_, id := makePending()
	g.recoveries[id].Expires = time.Now().Unix() - 1
	if recoveryCall(g, "download", "127.0.0.1", map[string]string{"Ticket": id}).Code != 403 {
		t.Fatal("expired ticket accepted")
	}
	_, id = makePending()
	recoveryCall(g, "download", "127.0.0.1", map[string]string{"Ticket": id})
	old := g.db.Config
	g.db.Config.Enabled = true
	old.Enabled = true
	demo = false
	g.fw.installed = true
	realRun = func(string, ...string) (string, error) {
		return "127.0.0.1 dev lo lladdr 02:00:00:00:00:02 REACHABLE", nil
	}
	g.fw.runner = func(string, ...string) (string, error) { return "", errors.New("firewall failed") }
	if recoveryCall(g, "commit", "127.0.0.1", map[string]any{"Ticket": id, "Saved": true}).Code != 400 || !reflect.DeepEqual(old, g.db.Config) {
		t.Fatal("failed update did not roll back")
	}
	r := httptest.NewRequest("POST", "http://127.0.0.1:8090/api/admin-access/login", strings.NewReader(`{}`))
	r.RemoteAddr = "127.0.0.1:5000"
	r.Header.Set("Origin", "http://evil.example")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.portalHandler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("CSRF accepted")
	}
	demo = true
	for i := 0; i < 5; i++ {
		recoveryCall(g, "login", "127.0.0.1", map[string]string{"Username": "wrong", "Password": "wrong"})
	}
	g.attempts = map[string]Attempt{} // Clearing visitor throttles cannot clear administrator throttling.
	if recoveryCall(g, "login", "127.0.0.1", map[string]string{"Username": "admin", "Password": "OldPass123"}).Code != 429 {
		t.Fatal("global throttle bypass")
	}
}

func TestLegacyRecoveryEnrollmentAndConfigProtection(t *testing.T) {
	g := testGate(t)
	g.db.Config.AdminName = "admin"
	g.db.Config.AdminHash = passwordHash("AdminPass123")
	before := g.db.Config
	w := call(g.adminHandler(), "/admin-api/recovery-prepare", "POST", map[string]any{}, false, g)
	if w.Code != 401 {
		t.Fatal("unauthenticated enrollment")
	}
	w = call(g.adminHandler(), "/admin-api/recovery-prepare", "POST", map[string]any{}, true, g)
	finishRecovery(t, g, w, "127.0.0.1")
	if g.db.Config.AdminHash != before.AdminHash || g.db.Config.AdminName != before.AdminName || g.db.Config.RecoveryHash == "" {
		t.Fatal("legacy enrollment changed credentials")
	}
	hash := g.db.Config.RecoveryHash
	c := g.db.Config
	c.RecoveryHash = "attacker-controlled"
	w = call(g.adminHandler(), "/admin-api/config", "POST", c, true, g)
	if w.Code != 200 || g.db.Config.RecoveryHash != hash {
		t.Fatal("config replaced recovery key", w.Body.String())
	}
	if call(g.adminHandler(), "/admin-api/recovery-prepare", "POST", map[string]any{}, true, g).Code != 400 {
		t.Fatal("silent replacement allowed")
	}
	w = recoveryCall(g, "login", "127.0.0.1", map[string]string{"Username": "admin", "Password": "AdminPass123", "MAC": "02:00:00:00:00:99"})
	if w.Code != 400 || g.isAdminDevice("02:00:00:00:00:99") {
		t.Fatal("browser-selected identity accepted")
	}
}
