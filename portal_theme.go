package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
)

// Themes never contain executable markup or authentication controls.
type ThemeBlock struct {
	Size     int          `json:"size,omitempty"`
	Color    string       `json:"color,omitempty"`
	Type     string       `json:"type"`
	Text     string       `json:"text,omitempty"`
	Image    string       `json:"image,omitempty"`
	URL      string       `json:"url,omitempty"`
	Align    string       `json:"align,omitempty"`
	Children []ThemeBlock `json:"children,omitempty"`
}
type ThemePage struct {
	Before []ThemeBlock `json:"before"`
	After  []ThemeBlock `json:"after"`
}
type PortalTheme struct {
	Schema     int                  `json:"schema"`
	Accent     string               `json:"accent"`
	Background string               `json:"background"`
	Radius     int                  `json:"radius"`
	Pages      map[string]ThemePage `json:"pages"`
	Shared     []ThemeBlock         `json:"shared"`
}
type ThemeStore struct {
	Revision  int          `json:"revision"`
	Draft     *PortalTheme `json:"draft"`
	Published *PortalTheme `json:"published"`
	Previous  *PortalTheme `json:"previous"`
}

var themeColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func validateTheme(t *PortalTheme) error {
	if t == nil {
		return nil
	}
	bad := errors.New("主题格式无效：请使用设计器导出的版本 1 主题，图片最大 32 KiB，最多 80 个组件")
	if t.Schema != 1 || !themeColor.MatchString(t.Accent) || !themeColor.MatchString(t.Background) || t.Radius < 0 || t.Radius > 32 || len(t.Pages) != 4 {
		return bad
	}
	count := 0
	var blocks func([]ThemeBlock, int) error
	blocks = func(items []ThemeBlock, depth int) error {
		if depth > 2 {
			return bad
		}
		for _, b := range items {
			count++
			if depth > 0 && b.Type == "container" {
				return bad
			}
			if (b.Size != 0 && (b.Size < 14 || b.Size > 40)) || (b.Color != "" && !themeColor.MatchString(b.Color)) {
				return bad
			}
			if count > 80 || len(b.Text) > 2000 || (b.Align != "" && b.Align != "left" && b.Align != "center" && b.Align != "right") {
				return bad
			}
			switch b.Type {
			case "text", "notice", "button", "divider", "space", "image", "container":
			default:
				return bad
			}
			if b.Image != "" {
				if b.Type != "image" {
					return bad
				}
				if _, _, e := decodeLogo(b.Image); e != nil {
					return e
				}
			}
			if b.URL != "" {
				u, e := url.Parse(b.URL)
				if b.Type != "button" || e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || len(b.URL) > 2048 {
					return bad
				}
			}
			if len(b.Children) > 0 && b.Type != "container" {
				return bad
			}
			if len(b.Children) > 0 {
				if e := blocks(b.Children, depth+1); e != nil {
					return e
				}
			}
		}
		return nil
	}
	for _, k := range []string{"login", "waiting", "success", "restricted"} {
		p, ok := t.Pages[k]
		if !ok {
			return bad
		}
		if e := blocks(p.Before, 0); e != nil {
			return e
		}
		if e := blocks(p.After, 0); e != nil {
			return e
		}
	}
	return blocks(t.Shared, 0)
}
func readThemes() ThemeStore {
	var s ThemeStore
	b, e := os.ReadFile(filepath.Join(dir, "portal-theme.json"))
	if e != nil || len(b) > 4<<20 || json.Unmarshal(b, &s) != nil {
		return ThemeStore{}
	}
	if validateTheme(s.Published) != nil {
		s.Published = nil
	}
	if validateTheme(s.Draft) != nil {
		s.Draft = nil
	}
	if validateTheme(s.Previous) != nil {
		s.Previous = nil
	}
	return s
}
func (g *Gate) publicTheme(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "方法不支持", 405)
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	reply(w, 200, readThemes().Published)
}

// Called only after the existing administrator device, session and origin checks.
func (g *Gate) themeAPI(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/admin-api/portal-theme" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	s := readThemes()
	if r.Method == "GET" {
		reply(w, 200, s)
		return true
	}
	if r.Method != "POST" {
		http.Error(w, "方法不支持", 405)
		return true
	}
	var v struct {
		Action   string       `json:"action"`
		Revision int          `json:"revision"`
		Theme    *PortalTheme `json:"theme"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if e := d.Decode(&v); e != nil {
		fail(w, e)
		return true
	}
	if d.Decode(new(any)) != io.EOF {
		fail(w, errors.New("无效 JSON"))
		return true
	}
	if v.Revision != s.Revision {
		reply(w, 409, map[string]string{"error": "主题已在其他窗口修改，请重新加载后编辑"})
		return true
	}
	switch v.Action {
	case "save", "publish":
		if v.Theme == nil {
			fail(w, errors.New("缺少主题"))
			return true
		}
		if e := validateTheme(v.Theme); e != nil {
			fail(w, e)
			return true
		}
		s.Draft = v.Theme
		if v.Action == "publish" {
			s.Previous = s.Published
			s.Published = v.Theme
		}
	case "rollback":
		s.Published, s.Previous = s.Previous, s.Published
	case "reset":
		s.Previous = s.Published
		s.Published = nil
	default:
		fail(w, errors.New("未知主题操作"))
		return true
	}
	s.Revision++
	b, e := json.Marshal(s)
	if e == nil {
		e = atomicWrite(filepath.Join(dir, "portal-theme.json"), b)
	}
	if e != nil {
		fail(w, e)
	} else {
		reply(w, 200, s)
	}
	return true
}
