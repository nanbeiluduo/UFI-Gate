package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNotificationDedupPersistenceAndPreferences(t *testing.T) {
	g := testGate(t)
	g.db.Config.Push = Push{Kind: "webhook"}
	n := g.notify("quota", "limit", "devices", "quota:device", true)
	id := n.ID
	n.Read = true
	n.Attempts = 2
	n.NextTry = time.Now().Unix() + 30
	g.notify("quota", "limit", "devices", "quota:device", true)
	if len(g.db.Notifications) != 1 {
		t.Fatal("duplicate")
	}
	raw, e := json.Marshal(g.db.Notifications)
	if e != nil {
		t.Fatal(e)
	}
	var restored []Notification
	if e = json.Unmarshal(raw, &restored); e != nil {
		t.Fatal(e)
	}
	if len(restored) != 1 || !restored[0].Read || restored[0].Key != "quota:device" || restored[0].Attempts != 2 || restored[0].ID != id {
		t.Fatalf("persistence: %s", raw)
	}
	g.db.Config.NotifyPush = map[string]bool{}
	if g.notify("error", "x", "logs", "error:x", true).PushState != "off" {
		t.Fatal("disabled push queued")
	}
	g.db.Config.NotifyPush = map[string]bool{"update_available": true}
	if g.notify("update_available", "new", "updates", "release:1.0.11", true).PushState != "pending" {
		t.Fatal("enabled push missing")
	}
	if g.notify("push_failed", "failed", "push", "failure", false).PushState != "off" {
		t.Fatal("recursive push")
	}
	for i := 0; i < 400; i++ {
		g.notify("admin", "x", "system", random(8), false)
	}
	if len(g.db.Notifications) != 300 {
		t.Fatal("unbounded history")
	}
}
func TestUpdateAndNotificationAPIsRequireLogin(t *testing.T) {
	g := testGate(t)
	for _, p := range []string{"/admin-api/update", "/admin-api/notifications"} {
		w := call(g.adminHandler(), p, "GET", nil, false, g)
		if w.Code != 401 {
			t.Fatalf("%s %d", p, w.Code)
		}
	}
}
