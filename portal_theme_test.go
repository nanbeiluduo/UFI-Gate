package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func sampleTheme() *PortalTheme {
	t := &PortalTheme{Schema: 1, Accent: "#2563eb", Background: "#ffffff", Radius: 16, Pages: map[string]ThemePage{}}
	for _, k := range []string{"login", "waiting", "success", "restricted"} {
		t.Pages[k] = ThemePage{Before: []ThemeBlock{{Type: "text", Text: "Welcome"}}}
	}
	return t
}
func TestThemeValidation(t *testing.T) {
	if e := validateTheme(sampleTheme()); e != nil {
		t.Fatal(e)
	}
	cases := []func(*PortalTheme){func(x *PortalTheme) { x.Schema = 2 }, func(x *PortalTheme) { delete(x.Pages, "login") }, func(x *PortalTheme) { x.Accent = "red;display:none" }, func(x *PortalTheme) { x.Shared = []ThemeBlock{{Type: "script"}} }, func(x *PortalTheme) { x.Shared = []ThemeBlock{{Type: "button", URL: "javascript:alert(1)"}} }, func(x *PortalTheme) {
		x.Shared = []ThemeBlock{{Type: "image", Image: "data:image/svg+xml;base64,AA=="}}
	}, func(x *PortalTheme) {
		for i := 0; i < 81; i++ {
			x.Shared = append(x.Shared, ThemeBlock{Type: "text"})
		}
	}}
	for i, f := range cases {
		x := sampleTheme()
		f(x)
		if validateTheme(x) == nil {
			t.Fatalf("accepted invalid theme %d", i)
		}
	}
}
func TestThemeLifecycle(t *testing.T) {
	g := testGate(t)
	h := g.adminHandler()
	theme := sampleTheme()
	post := func(action string, rev int) int {
		return call(h, "/admin-api/portal-theme", "POST", map[string]any{"action": action, "revision": rev, "theme": theme}, true, g).Code
	}
	if w := call(h, "/admin-api/portal-theme", "GET", nil, false, g); w.Code != 401 {
		t.Fatalf("anonymous %d", w.Code)
	}
	if post("save", 0) != 200 || readThemes().Published != nil {
		t.Fatal("draft exposed")
	}
	if post("publish", 0) != 409 {
		t.Fatal("stale revision accepted")
	}
	if post("publish", 1) != 200 || readThemes().Published == nil {
		t.Fatal("publish failed")
	}
	theme.Accent = "#123456"
	if post("publish", 2) != 200 {
		t.Fatal("second publish")
	}
	if post("rollback", 3) != 200 || readThemes().Published.Accent != "#2563eb" {
		t.Fatal("rollback")
	}
	if post("reset", 4) != 200 || readThemes().Published != nil {
		t.Fatal("reset")
	}
	var s ThemeStore
	b, _ := os.ReadFile(filepath.Join(dir, "portal-theme.json"))
	if json.Unmarshal(b, &s) != nil || s.Draft == nil {
		t.Fatal("draft lost")
	}
	os.WriteFile(filepath.Join(dir, "portal-theme.json"), []byte("broken"), 0600)
	if readThemes().Published != nil {
		t.Fatal("corrupt fallback")
	}
}
