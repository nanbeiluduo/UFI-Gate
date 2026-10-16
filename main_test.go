package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func testGate(t *testing.T) *Gate {
	t.Helper()
	oldDir, oldDemo, oldRun := dir, demo, realRun
	dir = t.TempDir()
	demo = true
	t.Cleanup(func() { dir = oldDir; demo = oldDemo; realRun = oldRun })
	g, e := loadGate()
	if e != nil {
		t.Fatal(e)
	}
	g.db.Config.LAN = "lo"
	g.db.Config.Gateway = "127.0.0.1"
	g.db.Config.AdminMAC = "02:00:00:00:00:02"
	g.db.Config.Verified = true
	g.device(g.db.Config.AdminMAC).IP = "127.0.0.1"
	return g
}
func call(h http.Handler, path, method string, v any, cookie bool, g *Gate) *httptest.ResponseRecorder {
	var b []byte
	if v != nil {
		b, _ = json.Marshal(v)
	}
	r := httptest.NewRequest(method, "http://127.0.0.1:8081"+path, bytes.NewReader(b))
	r.RemoteAddr = "127.0.0.1:42300"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://127.0.0.1:8081")
	if cookie {
		g.sessions["test-session"] = Session{time.Now().Unix(), "127.0.0.1"}
		r.AddCookie(&http.Cookie{Name: "ug_session", Value: "test-session"})
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestPasswordBcrypt(t *testing.T) {
	hash := passwordHash("SecurePass123")
	if !strings.HasPrefix(hash, "$2a$12$") || !passwordOK(hash, "SecurePass123") || passwordOK(hash, "wrong") {
		t.Fatal("bcrypt validation failed")
	}
	if strong("123456789") || strong("abcdefgh") || !strong("abcdefgh9") {
		t.Fatal("strong password validation")
	}
}
func TestAdminAuthenticationAndSecrets(t *testing.T) {
	g := testGate(t)
	g.db.Config.AdminHash = passwordHash("AdminPass123")
	g.db.Config.CaptchaSecret = "TOPSECRET"
	g.db.Config.Push.Secret = "PUSHSECRET"
	w := call(g.adminHandler(), "/admin-api/config", "GET", nil, false, g)
	if w.Code != 401 {
		t.Fatalf("unauth %d", w.Code)
	}
	w = call(g.adminHandler(), "/admin-api/config", "GET", nil, true, g)
	if w.Code != 200 || strings.Contains(w.Body.String(), "TOPSECRET") || strings.Contains(w.Body.String(), "PUSHSECRET") || strings.Contains(w.Body.String(), g.db.Config.AdminHash) {
		t.Fatal("secret leaked")
	}
}
func TestSetupTokenAndIdentity(t *testing.T) {
	g := testGate(t)
	v := map[string]string{"Username": "admin", "Password": "AdminPass123", "Token": "bad"}
	w := call(g.adminHandler(), "/admin-api/setup", "POST", v, false, g)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	v["Token"] = g.bootToken
	w = call(g.adminHandler(), "/admin-api/setup", "POST", v, false, g)
	w = finishRecovery(t, g, w, "127.0.0.1")
	if w.Code != 200 || g.db.Config.AdminMAC != "02:00:00:00:00:02" {
		t.Fatal(w.Body.String())
	}
	if len(w.Result().Cookies()) == 0 || !w.Result().Cookies()[0].HttpOnly {
		t.Fatal("session cookie must be HttpOnly")
	}
	w = call(g.adminHandler(), "/admin-api/setup", "POST", v, false, g)
	if w.Code != 403 {
		t.Fatal("setup replay accepted")
	}
}
func TestSetupExpiry(t *testing.T) {
	g := testGate(t)
	g.bootExpiry = time.Now().Unix() - 1
	w := call(g.adminHandler(), "/admin-api/setup", "POST", map[string]string{"Username": "admin", "Password": "AdminPass123", "Token": g.bootToken}, false, g)
	if w.Code != 403 {
		t.Fatal("expired setup accepted")
	}
}
func TestCSRFProtection(t *testing.T) {
	g := testGate(t)
	r := httptest.NewRequest("POST", "http://127.0.0.1:8081/admin-api/toggle", strings.NewReader(`{"Enabled":true}`))
	r.Header.Set("Origin", "https://attacker.example")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.adminHandler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-site mutation accepted")
	}
}
func TestIdleSessionExpires(t *testing.T) {
	g := testGate(t)
	g.sessions["expired"] = Session{time.Now().Unix() - 1801, "127.0.0.1"}
	r := httptest.NewRequest("GET", "http://127.0.0.1:8081/admin-api/dashboard", nil)
	r.RemoteAddr = "127.0.0.1:888"
	r.AddCookie(&http.Cookie{Name: "ug_session", Value: "expired"})
	w := httptest.NewRecorder()
	g.adminHandler().ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("idle token accepted")
	}
}

func TestSessionCookieRenewalExcludesPolling(t *testing.T) {
	g := testGate(t)
	w := call(g.adminHandler(), "/admin-api/dashboard", "GET", nil, true, g)
	if w.Code != 200 || len(w.Result().Cookies()) != 0 {
		t.Fatal("polling must not renew idle sessions")
	}
	w = call(g.adminHandler(), "/admin-api/config", "GET", nil, true, g)
	cookies := w.Result().Cookies()
	if w.Code != 200 || len(cookies) != 1 || cookies[0].MaxAge != 1800 || !cookies[0].HttpOnly {
		t.Fatal("user activity must renew session cookie")
	}
}
func TestLoginLocksAfterFiveFailures(t *testing.T) {
	g := testGate(t)
	g.db.Config.AdminName = "admin"
	g.db.Config.AdminHash = passwordHash("AdminPass123")
	for i := 0; i < 5; i++ {
		w := call(g.adminHandler(), "/admin-api/login", "POST", map[string]string{"Username": "wrong", "Password": "wrong"}, false, g)
		if w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	w := call(g.adminHandler(), "/admin-api/login", "POST", map[string]string{"Username": "admin", "Password": "AdminPass123"}, false, g)
	if w.Code != 429 {
		t.Fatal("lockout not enforced")
	}
}
func TestCaptchaSingleUseAndCannotForgeMAC(t *testing.T) {
	g := testGate(t)
	g.db.Config.Enabled = true
	mac := g.db.Config.AdminMAC
	g.challenges["one"] = Challenge{mac, "ABCDE", time.Now().Unix() + 60}
	payload := map[string]string{"Method": "captcha", "ChallengeID": "one", "Answer": "ABCDE"}
	w := call(g.portalHandler(), "/api/auth", "POST", payload, false, g)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = call(g.portalHandler(), "/api/auth", "POST", payload, false, g)
	if w.Code == 200 {
		t.Fatal("challenge reused")
	}
	payload["MAC"] = "02:00:00:00:00:99"
	w = call(g.portalHandler(), "/api/auth", "POST", payload, false, g)
	if w.Code == 200 {
		t.Fatal("forged identity accepted")
	}
}
func TestApprovalSignatureExpiryAndReplay(t *testing.T) {
	g := testGate(t)
	a := &Request{ID: "req1", MAC: g.db.Config.AdminMAC, Status: "pending", Nonce: random(16), Expires: time.Now().Unix() + 60}
	g.db.Requests[a.ID] = a
	token := g.approvalToken(*a, "allow")
	id, action, e := g.parseApproval(token)
	if e != nil || id != "req1" || action != "allow" {
		t.Fatal(e)
	}
	if _, _, e = g.parseApproval(token + "x"); e == nil {
		t.Fatal("tampered signature accepted")
	}
	a.Expires = time.Now().Unix() - 1
	if _, _, e = g.parseApproval(token); e == nil {
		t.Fatal("expired request accepted")
	}
	a.Expires = time.Now().Unix() + 60
	token = g.approvalToken(*a, "allow")
	if e = g.decide(a.ID, "allow", ""); e != nil {
		t.Fatal(e)
	}
	if _, _, e = g.parseApproval(token); e == nil {
		t.Fatal("token replay accepted")
	}
}
func TestApprovalGetNeverMutates(t *testing.T) {
	g := testGate(t)
	a := &Request{ID: "req", MAC: g.db.Config.AdminMAC, Status: "pending", Nonce: random(16), Expires: time.Now().Unix() + 60}
	g.db.Requests[a.ID] = a
	w := call(g.adminHandler(), "/approve?token="+g.approvalToken(*a, "allow"), "GET", nil, false, g)
	if w.Code != 200 || a.Status != "pending" || g.device(a.MAC).Until != 0 {
		t.Fatal("GET must only show confirmation")
	}
}
func TestQuotasLatchAndReleasePreservesHistory(t *testing.T) {
	g := testGate(t)
	d := g.device(g.db.Config.AdminMAC)
	d.Quota.Daily = 1024
	d.DayUp = 800
	d.DayDown = 300
	d.Up = 800
	d.Down = 300
	if !g.enforceQuota(d) || g.state(d) != "quota" {
		t.Fatal("quota not enforced")
	}
	d.Trusted = true
	if g.state(d) != "quota" {
		t.Fatal("trusted bypassed quota")
	}
	w := call(g.adminHandler(), "/admin-api/device", "POST", map[string]string{"MAC": d.MAC, "Action": "release"}, true, g)
	if w.Code != 200 || d.Up+d.Down != 1100 || d.DailyBase != 1100 || g.ratio(d) != 0 || g.state(d) != "online" {
		t.Fatal("release must preserve historical counters")
	}
	d.Banned = true
	if g.state(d) != "blocked" {
		t.Fatal("ban priority")
	}
}
func TestDailyResetAndSessionCannotBeResetByReauth(t *testing.T) {
	g := testGate(t)
	d := g.device(g.db.Config.AdminMAC)
	d.Day = "1999-01-01"
	d.DayUp = 100
	d.Up = 1000
	g.roll(d)
	if d.DayUp != 0 || d.Up != 1000 {
		t.Fatal("day rollover damaged total")
	}
	d.Until = time.Now().Unix() + 3600
	d.SessionBytes = 900
	if e := g.grant(d); e != nil || d.SessionBytes != 900 {
		t.Fatal("reauth reset quota")
	}
}

func TestAccountSessionCannotOutliveAccount(t *testing.T) {
	g := testGate(t)
	d := g.device(g.db.Config.AdminMAC)
	expires := time.Now().Unix() + 60
	if e := g.grantUntil(d, expires); e != nil || d.Until != expires {
		t.Fatal("account expiry must cap session duration")
	}
	if e := g.grantUntil(d, time.Now().Unix()-1); e == nil {
		t.Fatal("expired account cannot grant")
	}
}
func dnsQuery(name string, typ uint16) []byte {
	q := make([]byte, 12)
	q[0] = 12
	q[1] = 34
	q[2] = 1
	q[5] = 1
	for _, s := range strings.Split(name, ".") {
		q = append(q, byte(len(s)))
		q = append(q, []byte(s)...)
	}
	q = append(q, 0, byte(typ>>8), byte(typ), 0, 1)
	return q
}
func TestDNSUnauthenticatedAndBlacklisted(t *testing.T) {
	g := testGate(t)
	g.db.Config.Enabled = true
	g.db.Config.Black = []string{"*.blocked.example"}
	q := dnsQuery("example.com", 1)
	a, e := g.dnsReply(q, "127.0.0.1")
	if e != nil {
		t.Fatal(e)
	}
	ips, e := answerIPs(a)
	if e != nil || len(ips) != 1 || ips["127.0.0.1"] == 0 {
		t.Fatal("unauth DNS must resolve to portal")
	}
	g.device(g.db.Config.AdminMAC).Trusted = true
	a, e = g.dnsReply(dnsQuery("sub.blocked.example", 1), "127.0.0.1")
	if e != nil {
		t.Fatal(e)
	}
	ips, _ = answerIPs(a)
	if ips["127.0.0.1"] == 0 {
		t.Fatal("blacklist failed")
	}
	a, e = g.dnsReply(dnsQuery("example.com", 28), "127.0.0.1")
	if e != nil || binary.BigEndian.Uint16(a[6:8]) != 0 {
		t.Fatal("AAAA should be NODATA")
	}
}
func TestDNSMalformedAndSuffixBoundaries(t *testing.T) {
	for _, q := range [][]byte{{}, make([]byte, 12), append(dnsQuery("x.com", 1)[:12], 0xc0, 0x0c, 0, 1, 0, 1)} {
		if _, _, _, e := question(q); e == nil {
			t.Fatal("malformed DNS accepted")
		}
	}
	if matches("evil-example.com", []string{"*.example.com"}) || !matches("a.example.com", []string{"*.example.com"}) {
		t.Fatal("domain suffix boundary")
	}
}
func TestHTTPRedirectAndBlackWarning(t *testing.T) {
	g := testGate(t)
	g.db.Config.Black = []string{"*.blocked.example"}
	r := httptest.NewRequest("GET", "http://a.blocked.example/path", nil)
	w := httptest.NewRecorder()
	g.portalHandler().ServeHTTP(w, r)
	if w.Code != 302 || !strings.Contains(w.Header().Get("Location"), "reason=black") {
		t.Fatal("black warning redirect missing")
	}
}
func TestPersistenceAndPermissions(t *testing.T) {
	g := testGate(t)
	g.db.Config.AdminHash = passwordHash("AdminPass123")
	g.device(g.db.Config.AdminMAC).Up = 42
	if e := g.save(); e != nil {
		t.Fatal(e)
	}
	st, e := os.Stat(filepath.Join(dir, "state.json"))
	if e != nil || (runtime.GOOS != "windows" && st.Mode().Perm() != 0600) {
		t.Fatal("state must be 0600")
	}
	other, e := loadGate()
	if e != nil || other.db.Devices[g.db.Config.AdminMAC].Up != 42 {
		t.Fatal("persistence failed")
	}
}
func TestValidationRejectsShellAndUnsafePush(t *testing.T) {
	g := testGate(t)
	c := g.db.Config
	if e := validateConfig(c); e != nil {
		t.Fatal(e)
	}
	c.LAN = "wlan0;reboot"
	if validateConfig(c) == nil {
		t.Fatal("shell interface accepted")
	}
	c = g.db.Config
	c.Black = []string{"example.com;reboot"}
	if validateConfig(c) == nil {
		t.Fatal("invalid domain accepted")
	}
	c = g.db.Config
	c.Push.Kind = "webhook"
	c.Push.URL = "http://example.com"
	if validateConfig(c) == nil {
		t.Fatal("insecure push accepted")
	}
}
func TestFirewallPoliciesAndScopedCleanup(t *testing.T) {
	g := testGate(t)
	demo = false
	g.db.Config.LAN = "wlan0"
	g.db.Config.Gateway = "192.168.0.1"
	g.db.Config.Enabled = true
	g.db.Config.OffloadOriginal = "1"
	var commands []string
	runner := func(bin string, args ...string) (string, error) {
		s := bin + " " + strings.Join(args, " ")
		commands = append(commands, s)
		if bin == "settings" && contains(args, "get") {
			return "1", nil
		}
		if contains(args, "-S") {
			return "-N UFIG_F\n-N UFIG_FA\n-A FORWARD -i wlan0 -j UFIG_F\n-N OTHER_PLUGIN\n-A FORWARD -j OTHER_PLUGIN\n", nil
		}
		return "", nil
	}
	realRun = runner
	g.fw.runner = runner
	d := g.device(g.db.Config.AdminMAC)
	d.IP = "192.168.0.2"
	d.Until = time.Now().Unix() + 3600
	d.Quota.Action = "slow"
	d.LimitReason = "quota"
	if e := g.reconcile(); e != nil {
		t.Fatal(e)
	}
	joined := strings.Join(commands, "\n")
	for _, required := range []string{"-R UFIG_F 1 -j UFIG_FB", "-R UFIG_I 1 -j UFIG_IB", "-A UFIG_FB -m comment --comment ug:blocked -j DROP", "ip6tables -w 3 -A UFIG_6 -j DROP", "--dport 53 -j REDIRECT --to-ports 5354", "iptables -w 3 -t mangle -I POSTROUTING 1 -j UFIG_C"} {
		if !strings.Contains(joined, required) {
			t.Errorf("missing rule %s", required)
		}
	}
	if strings.Contains(joined, "--dport 443 -j REDIRECT") {
		t.Fatal("must not intercept TLS with HTTP")
	}
	if !strings.Contains(joined, "-R UFIG_F 2 -j RETURN") || strings.Contains(joined, "-A UFIG_FB -i wlan0 -s 192.168.0.2 -m mac --mac-source 02:00:00:00:00:02 -j ACCEPT") {
		t.Fatal("approved traffic must respect native FORWARD restrictions")
	}
	if !strings.Contains(joined, "--hashlimit-name ugBu0") {
		t.Fatal("rate table must use inactive-side name")
	}
	commands = nil
	d.Quota.RateKBps = 64
	if e := g.reconcile(); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(strings.Join(commands, "\n"), "--hashlimit-name ugAu0") {
		t.Fatal("rate update must not reuse active-side table")
	}
	commands = nil
	g.fw.cleanup()
	joined = strings.Join(commands, "\n")
	if strings.Contains(joined, "-F FORWARD") || strings.Contains(joined, "-D FORWARD -j OTHER_PLUGIN") || strings.Contains(joined, "-X OTHER_PLUGIN") {
		t.Fatal("cleanup touched another plugin")
	}
}
func TestCounterSnapshotsDoNotDoubleCount(t *testing.T) {
	g := testGate(t)
	demo = false
	g.fw.installed = true
	d := g.device(g.db.Config.AdminMAC)
	out := fmt.Sprintf("0 1000 all -- wlan0 * 0/0 0/0 /* ug:%s:u */\n0 2000 all -- * wlan0 0/0 0/0 /* ug:%s:d */", d.MAC, d.MAC)
	realRun = func(bin string, args ...string) (string, error) {
		if contains(args, "UFIG_C") {
			return out, nil
		}
		return "", nil
	}
	g.sample()
	if d.Up != 1000 || d.Down != 2000 {
		t.Fatalf("counter parse: %d/%d", d.Up, d.Down)
	}
	g.sample()
	if d.Up != 1000 || d.Down != 2000 {
		t.Fatal("same snapshot counted twice")
	}
	out = strings.Replace(out, "1000", "1500", 1)
	g.sample()
	if d.Up != 1500 {
		t.Fatal("incremental counter lost")
	}
}
