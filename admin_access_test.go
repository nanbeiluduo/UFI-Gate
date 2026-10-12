package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAdminConnectionAccess(t *testing.T) {
	g := testGate(t)
	g.db.Config.AdminHash = "initialized"
	g.db.Config.AdminMACs = []string{"02:00:00:00:00:33"}
	server := httptest.NewUnstartedServer(g.adminHandler())
	server.Listener = &adminListener{Listener: server.Listener, gate: g}
	server.Start()
	defer server.Close()
	client := &http.Client{Timeout: 2 * time.Second}
	defer client.CloseIdleConnections()
	for _, enabled := range []bool{false, true} {
		g.mu.Lock()
		g.db.Config.Enabled = enabled
		g.mu.Unlock()
		for _, path := range []string{"/", "/admin.js", "/style.css", "/site-logo", "/default-logo.png", "/admin-api/ready", "/approve"} {
			resp, err := client.Get(server.URL + path)
			if resp != nil {
				resp.Body.Close()
			}
			if err == nil {
				t.Fatalf("non-admin received HTTP response at %s (enabled=%v)", path, enabled)
			}
		}
	}
	// Reset opens the initialization page without changing the network switch.
	g.mu.Lock()
	_, err := g.resetAdmin()
	g.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal("initialization page unavailable")
	}
	// Initialization elsewhere must also close the existing keep-alive connection.
	g.mu.Lock()
	g.db.Config.AdminHash = "initialized"
	g.db.Config.AdminMACs = []string{"02:00:00:00:00:33"}
	g.mu.Unlock()
	resp, err = client.Get(server.URL + "/admin.js")
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil {
		t.Fatal("old keep-alive connection bypassed permissions")
	}
	// An administrator can still load the login page.
	g.mu.Lock()
	g.db.Config.AdminMACs = []string{"02:00:00:00:00:02"}
	g.mu.Unlock()
	resp, err = client.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal("administrator login unavailable")
	}
}
