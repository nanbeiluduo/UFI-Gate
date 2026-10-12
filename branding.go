package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"strings"
)

// Inline images keep branding available before the visitor has Internet access.
func defaultLogo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "方法不支持", 405)
		return
	}
	raw, err := web.ReadFile("web/default-logo.png")
	if err != nil {
		http.Error(w, "默认图标不可用", 500)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodHead {
		_, _ = w.Write(raw)
	}
}

func decodeLogo(value string) ([]byte, string, error) {
	if value == "" {
		return nil, "", nil
	}
	invalid := errors.New("Logo 须为不超过 32 KiB、1024 × 1024 的 PNG、JPG 或 GIF 图片")
	if len(value) > 44000 {
		return nil, "", invalid
	}
	header, encoded, ok := strings.Cut(value, ",")
	formats := map[string]string{"data:image/png;base64": "png", "data:image/jpeg;base64": "jpeg", "data:image/gif;base64": "gif"}
	format := formats[header]
	if !ok || format == "" {
		return nil, "", invalid
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(raw) > 32768 {
		return nil, "", invalid
	}
	cfg, actual, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || actual != format || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 1024 || cfg.Height > 1024 {
		return nil, "", invalid
	}
	if _, _, err = image.Decode(bytes.NewReader(raw)); err != nil {
		return nil, "", invalid
	}
	return raw, "image/" + format, nil
}

func (g *Gate) siteLogo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "方法不支持", 405)
		return
	}
	g.mu.Lock()
	value := g.db.Config.SiteLogo
	g.mu.Unlock()
	raw, mime, err := decodeLogo(value)
	if err != nil || len(raw) == 0 {
		mime = "image/png"
		raw, err = web.ReadFile("web/default-logo.png")
		if err != nil {
			http.Error(w, "默认图标不可用", 500)
			return
		}
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodHead {
		_, _ = w.Write(raw)
	}
}
