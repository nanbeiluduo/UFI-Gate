package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBranding(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 16, 16))); err != nil {
		t.Fatal(err)
	}
	logo := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
	if _, mime, err := decodeLogo(logo); err != nil || mime != "image/png" {
		t.Fatalf("valid PNG: %s %v", mime, err)
	}
	for _, bad := range []string{"https://example.com/logo.png", "data:image/svg+xml;base64,PHN2Zy8+", strings.Replace(logo, "image/png", "image/jpeg", 1), "data:image/png;base64,AAAA", strings.Repeat("x", 44001)} {
		if _, _, err := decodeLogo(bad); err == nil {
			t.Fatal("accepted invalid logo")
		}
	}
	g := testGate(t)
	g.db.Config.SiteLogo = logo
	g.db.Config.PortalLogo = logo
	if err := g.save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := loadGate()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.db.Config.SiteLogo != logo || reloaded.db.Config.PortalLogo != logo {
		t.Fatal("logos did not persist")
	}
	w := httptest.NewRecorder()
	g.siteLogo(w, httptest.NewRequest("GET", "/site-logo", nil))
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), buf.Bytes()) {
		t.Fatal("logo response differs")
	}
	g.db.Config.SiteLogo = ""
	w = httptest.NewRecorder()
	g.siteLogo(w, httptest.NewRequest("GET", "/site-logo", nil))
	if w.Header().Get("Content-Type") != "image/svg+xml" {
		t.Fatal("default logo missing")
	}
}
