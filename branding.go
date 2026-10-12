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
		mime = "image/svg+xml"
		raw = []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48"><rect width="48" height="48" rx="12" fill="#0067c0"/><g fill="none" stroke="white" stroke-width="3" stroke-linecap="round"><path d="M10 18a23 23 0 0 1 28 0M15 25a15 15 0 0 1 18 0M20 32a7 7 0 0 1 8 0"/><path d="M24 38h0"/></g></svg>`)
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodHead {
		_, _ = w.Write(raw)
	}
}
