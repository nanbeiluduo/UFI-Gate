package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func remoteCall(g *Gate, path, method, token, origin string) *httptest.ResponseRecorder {
	form := url.Values{"token": {token}, "reason": {"remote test"}}
	if method == "GET" {
		path += "?token=" + url.QueryEscape(token)
	}
	r := httptest.NewRequest(method, "http://127.0.0.1:8091"+path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	g.remoteApprovalHandler().ServeHTTP(w, r)
	return w
}

func remoteGate(t *testing.T) *Gate {
	g := testGate(t)
	g.db.Config.AdminHash = "initialized"
	g.db.Config.RemoteApproval = RemoteApproval{Enabled: true, PublicURL: "https://approve.example.com"}
	t.Cleanup(func() {
		if g.remoteServer != nil {
			g.remoteServer.Close()
		}
		g.cancel()
	})
	return g
}

func TestRemoteApprovalValidation(t *testing.T) {
	if !remoteOriginMatches("https://approve.example.com", "https://APPROVE.example.com:443/") || remoteOriginMatches("https://approve.example.com:8443", "https://approve.example.com") {
		t.Fatal("HTTPS origin normalization")
	}
	for _, s := range []string{"http://approve.example.com", "https://a.example/path", "https://a.example?x=1", "https://a.example?", "https://a.example/#token", "https://u:p@a.example", "https://a.example:99999", ""} {
		if validateRemoteApproval(RemoteApproval{Enabled: true, PublicURL: s}) == nil {
			t.Fatalf("accepted %q", s)
		}
	}
	for _, port := range []int{1, 8081, 8090, 5354, 65536} {
		if validateRemoteApproval(RemoteApproval{Port: port}) == nil {
			t.Fatalf("accepted port %d", port)
		}
	}
	if err := validateRemoteApproval(RemoteApproval{}); err != nil {
		t.Fatal(err)
	}
	if err := validateRemoteApproval(RemoteApproval{Enabled: true, Port: 18091, PublicURL: "https://approve.example.com:8443/"}); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteApprovalIsolationAndSingleUse(t *testing.T) {
	g := remoteGate(t)
	a := &Request{ID: "remote", MAC: "02:00:00:00:00:44", Name: "Visitor", Status: "pending", Nonce: random(16), Expires: time.Now().Unix() + 1800}
	g.db.Requests[a.ID] = a
	token := g.approvalToken(*a, "allow")
	for _, path := range []string{"/", "/admin-api/config", "/admin-api/setup", "/admin.js", "/api/admin-access/reset", "/approve/"} {
		if w := remoteCall(g, path, "GET", token, ""); w.Code != 404 {
			t.Fatalf("exposed %s: %d", path, w.Code)
		}
	}
	for _, bad := range []string{"", token + "x"} {
		if w := remoteCall(g, "/approve", "GET", bad, ""); w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	w := remoteCall(g, "/approve", "GET", token, "")
	if w.Code != 200 || a.Status != "pending" || w.Header().Get("Referrer-Policy") != "no-referrer" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, a.Status)
	}
	if w = remoteCall(g, "/approve", "POST", token, "https://evil.example"); w.Code != 403 || a.Status != "pending" {
		t.Fatal("cross-origin executed")
	}
	if w = remoteCall(g, "/approve", "POST", token, "https://approve.example.com"); w.Code != 200 || a.Status != "allow" || !g.allowed(g.device(a.MAC)) {
		t.Fatal(w.Code, w.Body.String(), a.Status)
	}
	if w = remoteCall(g, "/approve", "POST", token, ""); w.Code != 400 {
		t.Fatal("replay accepted")
	}
	// Reject a valid signed but expired request.
	a.Status = "pending"
	a.Nonce = random(16)
	a.Expires = time.Now().Unix() - 1
	if w = remoteCall(g, "/approve", "GET", g.approvalToken(*a, "deny"), ""); w.Code != 400 {
		t.Fatal("expired accepted")
	}
	g.db.Config.RemoteApproval.Enabled = false
	if w = remoteCall(g, "/approve", "GET", token, ""); w.Code != 404 {
		t.Fatal("disabled exposed")
	}
	g.db.Config.RemoteApproval.Enabled = true
	g.db.Config.AdminHash = ""
	if w = remoteCall(g, "/approve", "GET", token, ""); w.Code != 404 {
		t.Fatal("reset exposed")
	}
}

func TestRemoteApprovalTestTokenAndRateLimit(t *testing.T) {
	g := remoteGate(t)
	g.remoteTestNonce = random(24)
	g.remoteTestExpires = time.Now().Unix() + 300
	token := g.remoteTestToken()
	before := len(g.db.Devices)
	if w := remoteCall(g, "/approve", "GET", token, ""); w.Code != 400 {
		t.Fatal("test token accepted as approval")
	}
	if w := remoteCall(g, "/approve-test", "GET", token, ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := remoteCall(g, "/approve-test", "POST", token, ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if len(g.db.Devices) != before || len(g.db.Requests) != 0 {
		t.Fatal("test changed devices/requests")
	}
	if w := remoteCall(g, "/approve-test", "POST", token, ""); w.Code != 400 {
		t.Fatal("test replay accepted")
	}
	g.remoteRequests = 120
	g.remoteWindow = time.Now().Unix()
	if w := remoteCall(g, "/approve", "GET", "invalid", ""); w.Code != 429 {
		t.Fatal("rate limit bypassed")
	}
}

func freeRemotePort(t *testing.T) int {
	t.Helper()
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestRemoteApprovalConfigLifecycle(t *testing.T) {
	g := remoteGate(t)
	g.db.Config.RemoteApproval = RemoteApproval{}
	c := g.db.Config
	c.RemoteApproval = RemoteApproval{Enabled: true, Port: freeRemotePort(t), PublicURL: "https://approve.example.com"}
	w := call(g.adminHandler(), "/admin-api/config", "POST", c, true, g)
	if w.Code != 200 || g.remoteServer == nil {
		t.Fatal(w.Code, w.Body.String())
	}
	probe := func(port int) error {
		r, e := http.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/style.css")
		if e == nil {
			r.Body.Close()
			if r.StatusCode != 200 {
				t.Fatalf("probe %d", r.StatusCode)
			}
		}
		return e
	}
	if e := probe(c.RemoteApproval.Port); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(filepath.Join(dir, "state.json"))
	if e != nil {
		t.Fatal(e)
	}
	var db DB
	if json.Unmarshal(raw, &db) != nil || db.Config.RemoteApproval != c.RemoteApproval {
		t.Fatal("not persisted")
	}
	occupied, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer occupied.Close()
	bad := c
	bad.RemoteApproval.Port = occupied.Addr().(*net.TCPAddr).Port
	if w = call(g.adminHandler(), "/admin-api/config", "POST", bad, true, g); w.Code != 400 || g.db.Config.RemoteApproval != c.RemoteApproval {
		t.Fatal("port conflict changed config")
	}
	if e := probe(c.RemoteApproval.Port); e != nil {
		t.Fatal("old listener lost", e)
	}
	oldPort := c.RemoteApproval.Port
	c.RemoteApproval.Port = freeRemotePort(t)
	if w = call(g.adminHandler(), "/admin-api/config", "POST", c, true, g); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if e := probe(c.RemoteApproval.Port); e != nil {
		t.Fatal(e)
	}
	if conn, e := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(oldPort), time.Second); e == nil {
		conn.Close()
		t.Fatal("old port still open")
	}
	c.RemoteApproval.Enabled = false
	if w = call(g.adminHandler(), "/admin-api/config", "POST", c, true, g); w.Code != 200 || g.remoteServer != nil {
		t.Fatal("disable failed")
	}
}

func TestRemoteApprovalPushURL(t *testing.T) {
	g := remoteGate(t)
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &payload)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()
	p := Push{Kind: "webhook", URL: server.URL, PublicBase: "http://192.168.0.1:8081"}
	a := Request{ID: "test", Expires: time.Now().Unix() + 1800, Nonce: random(8)}
	if e := g.sendRequest(p, a); e != nil {
		t.Fatal(e)
	}
	if !strings.HasPrefix(payload["allow_url"].(string), "https://approve.example.com/approve?token=") {
		t.Fatal(payload)
	}
	g.db.Config.RemoteApproval.Enabled = false
	if e := g.sendRequest(p, a); e != nil {
		t.Fatal(e)
	}
	if !strings.HasPrefix(payload["allow_url"].(string), p.PublicBase+"/approve?") {
		t.Fatal("legacy URL changed")
	}
}

func TestRemoteApprovalTestMessageEndToEnd(t *testing.T) {
	g := remoteGate(t)
	messages := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		messages <- payload
		w.Write([]byte(`{}`))
	}))
	defer server.Close()
	g.db.Config.Push = Push{Kind: "webhook", URL: server.URL}
	g.db.Config.RemoteApproval.Port = freeRemotePort(t)
	listener, err := g.prepareRemoteApproval(g.db.Config.RemoteApproval)
	if err != nil {
		t.Fatal(err)
	}
	if listener.Addr().(*net.TCPAddr).IP.String() != "127.0.0.1" {
		t.Fatal("non-loopback listener")
	}
	g.commitRemoteApproval(listener)
	if w := call(g.adminHandler(), "/admin-api/remote-approval-test", "POST", nil, false, g); w.Code != 401 {
		t.Fatal("unauthenticated send")
	}
	if w := call(g.adminHandler(), "/admin-api/remote-approval-test", "POST", nil, true, g); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var payload map[string]any
	select {
	case payload = <-messages:
	case <-time.After(5 * time.Second):
		t.Fatal("test message missing")
	}
	link, err := url.Parse(payload["allow_url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if link.Host != "approve.example.com" || link.Path != "/approve-test" {
		t.Fatal(link)
	}
	if w := remoteCall(g, link.Path, "POST", link.Query().Get("token"), "https://approve.example.com"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := call(g.adminHandler(), "/admin-api/remote-approval-test", "POST", nil, true, g); w.Code != 400 {
		t.Fatal("send cooldown missing")
	}
	// Wait for the asynchronous sender to finish persistence before test globals are restored.
	deadline := time.Now().Add(5 * time.Second)
	for {
		g.mu.Lock()
		done := false
		for _, e := range g.db.Logs {
			if e.Message == "远程审批测试消息已发送" {
				done = true
			}
		}
		g.mu.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("sender did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRemoteApprovalSaveFailureAndRestart(t *testing.T) {
	g := remoteGate(t)
	g.db.Config.RemoteApproval = RemoteApproval{}
	c := g.db.Config
	c.RemoteApproval = RemoteApproval{Enabled: true, Port: freeRemotePort(t), PublicURL: "https://approve.example.com"}
	// A directory at the state filename forces atomic persistence to fail.
	if err := os.Mkdir(filepath.Join(dir, "state.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if w := call(g.adminHandler(), "/admin-api/config", "POST", c, true, g); w.Code != 400 || g.remoteServer != nil || g.db.Config.RemoteApproval.Enabled {
		t.Fatal("failed save activated listener")
	}
	if err := os.Remove(filepath.Join(dir, "state.json")); err != nil {
		t.Fatal(err)
	}
	if w := call(g.adminHandler(), "/admin-api/config", "POST", c, true, g); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	g.remoteServer.Close()
	g.remoteServer = nil
	reloaded, err := loadGate()
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.cancel()
	listener, err := reloaded.prepareRemoteApproval(reloaded.db.Config.RemoteApproval)
	if err != nil {
		t.Fatal(err)
	}
	reloaded.commitRemoteApproval(listener)
	defer reloaded.remoteServer.Close()
	if reloaded.db.Config.RemoteApproval != c.RemoteApproval {
		t.Fatal("restart lost settings")
	}
}
