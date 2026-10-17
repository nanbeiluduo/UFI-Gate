package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Only a locally installed tunnel client can connect. Never expose the admin listener.
type RemoteApproval struct {
	Enabled   bool
	Port      int
	PublicURL string
}

func remotePort(c RemoteApproval) int {
	if c.Port == 0 {
		return 8091
	}
	return c.Port
}

func validateRemoteApproval(c RemoteApproval) error {
	port := remotePort(c)
	if port < 1024 || port > 65535 || port == 8081 || port == 8090 || port == 5354 {
		return errors.New("审批端口须为 1024–65535，且不能使用 8081、8090、5354")
	}
	if !c.Enabled && c.PublicURL == "" {
		return nil
	}
	u, err := url.Parse(c.PublicURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("远程审批地址须为 HTTPS 域名或地址，不含路径、账号、查询参数或片段")
	}
	if u.Port() != "" {
		n, err := strconv.Atoi(u.Port())
		if err != nil || n < 1 || n > 65535 {
			return errors.New("公网地址端口无效")
		}
	}
	return nil
}

// Called with g.mu held. Bind before saving so a port conflict never replaces working settings.
func (g *Gate) prepareRemoteApproval(c RemoteApproval) (net.Listener, error) {
	if err := validateRemoteApproval(c); err != nil {
		return nil, err
	}
	if !c.Enabled || (g.remoteServer != nil && remotePort(c) == remotePort(g.db.Config.RemoteApproval)) {
		return nil, nil
	}
	return net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", remotePort(c)))
}

// Called only after configuration has been saved, with g.mu held.
func (g *Gate) commitRemoteApproval(listener net.Listener) {
	if !g.db.Config.RemoteApproval.Enabled || listener != nil {
		if g.remoteServer != nil {
			_ = g.remoteServer.Close()
			g.remoteServer = nil
		}
	}
	if listener == nil {
		return
	}
	s := &http.Server{Handler: g.remoteApprovalHandler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 8192}
	g.remoteServer = s
	go func() {
		if err := s.Serve(listener); err != nil && err != http.ErrServerClosed {
			g.mu.Lock()
			g.event("error", "远程审批服务异常，请检查端口并重新保存设置")
			g.mu.Unlock()
		}
	}()
}

func (g *Gate) remoteApprovalHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		g.mu.Lock()
		defer g.mu.Unlock()
		if !g.db.Config.RemoteApproval.Enabled || g.db.Config.AdminHash == "" || g.ctx.Err() != nil {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/approve" && r.URL.Path != "/approve-test" && r.URL.Path != "/style.css" {
			http.NotFound(w, r)
			return
		}
		if r.Method != "GET" && r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Method == "POST" && networkPending() {
			http.Error(w, "网络配置维护中，请稍后重试", 503)
			return
		}
		// Global budget is deliberate: forwarded IP headers are not trusted.
		now := time.Now().Unix()
		if now-g.remoteWindow >= 60 {
			g.remoteWindow = now
			g.remoteRequests = 0
		}
		g.remoteRequests++
		if g.remoteRequests > 120 {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "请求过于频繁，请稍后重试", 429)
			return
		}
		if r.Method == "POST" {
			if origin := r.Header.Get("Origin"); origin != "" && !remoteOriginMatches(origin, g.db.Config.RemoteApproval.PublicURL) {
				http.Error(w, "跨站请求被拒绝", 403)
				return
			}
		}
		if r.URL.Path == "/style.css" {
			if r.Method != "GET" {
				w.WriteHeader(405)
				return
			}
			staticFile(w, r, "style.css")
			return
		}
		if r.URL.Path == "/approve-test" {
			g.remoteApprovalTestPage(w, r)
			return
		}
		g.approvalPageLocked(w, r)
	})
}

func remoteOriginMatches(origin, base string) bool {
	a, err := url.Parse(origin)
	if err != nil || a.Scheme != "https" || a.User != nil || a.Path != "" || a.RawQuery != "" || a.Fragment != "" {
		return false
	}
	b, err := url.Parse(base)
	if err != nil {
		return false
	}
	port := func(u *url.URL) string {
		if u.Port() == "" {
			return "443"
		}
		return u.Port()
	}
	return strings.EqualFold(a.Hostname(), b.Hostname()) && port(a) == port(b)
}

func (g *Gate) remoteTestToken() string {
	payload := fmt.Sprintf("remote-test|%d|%s", g.remoteTestExpires, g.remoteTestNonce)
	h := hmac.New(sha256.New, []byte(g.db.Config.SigningKey))
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}

func (g *Gate) remoteApprovalTestPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if r.Method == "POST" {
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "请求无效", 400)
			return
		}
		token = r.PostForm.Get("token")
	}
	if g.remoteTestNonce == "" || time.Now().Unix() >= g.remoteTestExpires || !hmac.Equal([]byte(token), []byte(g.remoteTestToken())) {
		http.Error(w, "测试链接无效、已过期或已使用", 400)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Method == "POST" {
		g.remoteTestNonce = ""
		g.event("admin", "远程审批连通性测试完成（未变更设备权限）")
		fmt.Fprint(w, "<!doctype html><meta charset=utf-8><meta name=viewport content='width=device-width'><h2>远程审批测试成功</h2><p>未给任何设备放行。请关闭手机 WiFi 后再次生成链接测试，以确认公网可达。</p>")
		return
	}
	fmt.Fprintf(w, `<!doctype html><meta charset=utf-8><meta name=viewport content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/style.css"><main class="auth-shell"><section class="card auth-card"><h1>远程审批连接测试</h1><p>请使用手机流量打开并确认。有效期 5 分钟，不会给任何设备放行。</p><form method="post" action="/approve-test"><input type="hidden" name="token" value="%s"><button class="primary">确认测试</button></form></section></main>`, htmlEscape(token))
}

// Called by the authenticated admin API with g.mu held. Test tokens are memory-only.
func (g *Gate) sendRemoteApprovalTest(w http.ResponseWriter) {
	c := g.db.Config.RemoteApproval
	if !c.Enabled || g.remoteServer == nil {
		fail(w, errors.New("请先启用并保存远程审批设置"))
		return
	}
	p := g.db.Config.Push
	if p.Kind == "" {
		fail(w, errors.New("请先配置并保存推送渠道"))
		return
	}
	if time.Now().Unix()-g.remoteTestSent < 30 {
		fail(w, errors.New("测试消息发送至少间隔 30 秒"))
		return
	}
	g.remoteTestSent = time.Now().Unix()
	g.remoteTestExpires = g.remoteTestSent + 300
	g.remoteTestNonce = random(24)
	link := strings.TrimRight(c.PublicURL, "/") + "/approve-test?token=" + url.QueryEscape(g.remoteTestToken())
	go func() {
		err := g.sendPush(p, "远程审批连接测试", "请关闭手机 WiFi，使用移动网络打开任一测试按钮并确认。5 分钟有效，不会给设备放行。", link, link, nil)
		g.mu.Lock()
		defer g.mu.Unlock()
		if err != nil {
			g.event("push", "远程审批测试发送失败: "+safeError(err.Error(), p))
		} else {
			g.event("push", "远程审批测试消息已发送")
		}
		_ = g.save()
	}()
	reply(w, 200, map[string]bool{"ok": true})
}
