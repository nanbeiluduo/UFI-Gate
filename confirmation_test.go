package main

import (
	"strings"
	"testing"
)

func TestEnableRequiresSavedAndCurrentConfirmation(t *testing.T) {
	for _, saved := range []bool{false, true} {
		for _, confirmed := range []bool{false, true} {
			g := testGate(t)
			g.db.Config.Verified = saved
			w := call(g.adminHandler(), "/admin-api/toggle", "POST", map[string]bool{"Enabled": true, "Confirmed": confirmed}, true, g)
			if saved && confirmed {
				if w.Code != 200 || !g.db.Config.Enabled {
					t.Fatal(w.Code, w.Body.String())
				}
			} else if w.Code != 400 || g.db.Config.Enabled || !strings.Contains(w.Body.String(), "confirmation_required") {
				t.Fatal(saved, confirmed, w.Code, w.Body.String())
			}
		}
	}
}

func TestHistoricalConfirmationCannotEnableWithoutRequestConfirmation(t *testing.T) {
	g := testGate(t)
	g.db.Config.Verified = true
	w := call(g.adminHandler(), "/admin-api/toggle", "POST", map[string]bool{"Enabled": true}, true, g)
	if w.Code != 400 || g.db.Config.Enabled {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestDisableDoesNotRequireConfirmation(t *testing.T) {
	g := testGate(t)
	g.db.Config.Verified = false
	g.db.Config.Enabled = true
	w := call(g.adminHandler(), "/admin-api/toggle", "POST", map[string]bool{"Enabled": false}, true, g)
	if w.Code != 200 || g.db.Config.Enabled {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestThirdPartyMethodDoesNotDiscardUncheckedConfirmation(t *testing.T) {
	g := testGate(t)
	g.db.Config.Enabled = true
	g.db.Config.Methods = []string{"thirdparty"}
	g.db.Config.CaptchaProvider = "turnstile"
	g.db.Config.CaptchaSite = "test-site"
	g.db.Config.CaptchaSecret = "test-secret"
	c := g.db.Config
	c.Verified = false
	w := call(g.adminHandler(), "/admin-api/config", "POST", c, true, g)
	if w.Code != 200 || g.db.Config.Verified {
		t.Fatal(w.Code, w.Body.String())
	}
}
