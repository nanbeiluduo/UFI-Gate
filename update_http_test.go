package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type updateTransport func(*http.Request) (*http.Response, error)

func (f updateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestReleaseCheckSignedAndInvalid(t *testing.T) {
	g := testGate(t)
	raw, sig, key := signedFixture(t)
	old := updatePublicKey
	updatePublicKey = key
	defer func() { updatePublicKey = old }()
	broken := false
	g.httpClient = &http.Client{Transport: updateTransport(func(r *http.Request) (*http.Response, error) {
		var b []byte
		switch {
		case strings.HasSuffix(r.URL.Path, "/latest"):
			b = []byte(`{"tag_name":"v1.0.11","body":"notes"}`)
		case strings.HasSuffix(r.URL.Path, ".sig"):
			b = sig
			if broken {
				b = []byte("tampered")
			}
		default:
			b = raw
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b)), Header: make(http.Header)}, nil
	})}
	info := g.checkRelease()
	if info.Error != "" || info.Manifest == nil || info.Version != "1.0.11" {
		t.Fatalf("%+v", info)
	}
	broken = true
	info = g.checkRelease()
	if info.Error == "" || info.Manifest != nil {
		t.Fatal("bad signature accepted")
	}
}
func TestGenericPushUsesExistingChannels(t *testing.T) {
	g := testGate(t)
	for _, kind := range []string{"webhook", "wecom", "feishu"} {
		g.httpClient = &http.Client{Transport: updateTransport(func(r *http.Request) (*http.Response, error) {
			var payload map[string]any
			if e := json.NewDecoder(r.Body).Decode(&payload); e != nil {
				t.Fatal(e)
			}
			if kind == "webhook" && (payload["title"] != "test" || r.Header.Get("Authorization") != "Bearer token") {
				t.Fatal(payload)
			}
			if kind == "feishu" && (payload["msg_type"] != "text" || payload["sign"] == nil || payload["card"] != nil) {
				t.Fatal(payload)
			}
			if kind == "wecom" && payload["msgtype"] != "text" {
				t.Fatal(payload)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":0}`)), Header: make(http.Header)}, nil
		})}
		if e := g.sendPush(Push{Kind: kind, URL: "https://example.com/hook", Secret: "token"}, "test", "quota reached", "", "", nil); e != nil {
			t.Fatal(e)
		}
	}
}
func TestNotificationSettingsAndReadAPI(t *testing.T) {
	g := testGate(t)
	g.db.Config.AdminHash = "initialized"
	g.notify("error", "test", "logs", "e", false)
	w := call(g.adminHandler(), "/admin-api/notifications/settings", "POST", map[string]any{"Push": map[string]bool{"quota": true, "update_available": true}}, true, g)
	if w.Code != 200 || !g.pushEnabled("quota") || g.pushEnabled("error") {
		t.Fatal(w.Body.String())
	}
	w = call(g.adminHandler(), "/admin-api/notifications/read", "POST", map[string]any{"All": true}, true, g)
	if w.Code != 200 || !g.db.Notifications[0].Read {
		t.Fatal(w.Body.String())
	}
	w = call(g.adminHandler(), "/admin-api/notifications/settings", "POST", map[string]any{"Push": map[string]bool{"unknown": true}}, true, g)
	if w.Code == 200 {
		t.Fatal("unknown category accepted")
	}
}
