package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Pending credentials are memory-only and cannot change the account until the
// bound client has downloaded the replacement key and confirmed saving it.
type adminRecovery struct {
	Ticket, Key, MAC, IP, Mode, Username, PasswordHash string
	OldHash, OldRecovery, BootToken                    string
	Expires                                            int64
	Downloaded                                         bool
}

func recoveryDigest(key string) string {
	sum := sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(key))))
	return hex.EncodeToString(sum[:])
}
func recoveryKey() string {
	raw := strings.ToUpper(random(32))
	parts := []string{"UFIG"}
	for i := 0; i < len(raw); i += 8 {
		parts = append(parts, raw[i:i+8])
	}
	return strings.Join(parts, "-")
}
func (g *Gate) adminSession(w http.ResponseWriter, r *http.Request) {
	token := random(32)
	g.sessions[token] = Session{time.Now().Unix(), peer(r)}
	http.SetCookie(w, &http.Cookie{Name: "ug_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil, MaxAge: 1800})
}
func adminDestination(r *http.Request) string {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return "http://" + net.JoinHostPort(host, "8081") + "/"
}
func (g *Gate) prepareRecovery(w http.ResponseWriter, r *http.Request, mode, user, hash string) {
	mac, err := g.peerMAC(r)
	if err != nil {
		fail(w, err)
		return
	}
	if g.recoveries == nil {
		g.recoveries = map[string]*adminRecovery{}
	}
	now := time.Now().Unix()
	for id, p := range g.recoveries {
		if p.Expires < now || p.MAC == mac {
			delete(g.recoveries, id)
		}
	}
	if len(g.recoveries) >= 16 {
		reply(w, 429, map[string]string{"error": "恢复任务过多，请稍后重试"})
		return
	}
	p := &adminRecovery{Ticket: random(32), Key: recoveryKey(), MAC: mac, IP: peer(r), Mode: mode, Username: user, PasswordHash: hash, OldHash: g.db.Config.AdminHash, OldRecovery: g.db.Config.RecoveryHash, BootToken: g.bootToken, Expires: now + 600}
	g.recoveries[p.Ticket] = p
	reply(w, 200, map[string]any{"ticket": p.Ticket, "expires": p.Expires})
}

// Called while g.mu is held. The setup token remains valid until commit.
func (g *Gate) prepareAdminSetup(w http.ResponseWriter, r *http.Request) {
	var v struct{ Username, Password, Token string }
	if err := body(w, r, &v); err != nil {
		fail(w, err)
		return
	}
	if g.db.Config.AdminHash != "" || g.bootToken == "" || time.Now().Unix() > g.bootExpiry || subtle.ConstantTimeCompare([]byte(v.Token), []byte(g.bootToken)) != 1 {
		reply(w, 403, map[string]string{"error": "初始化凭证无效或已过期，请在插件中重新生成"})
		return
	}
	if !strong(v.Password) || strings.TrimSpace(v.Username) == "" || len(v.Username) > 64 {
		fail(w, errors.New("账号不可为空，密码须为 8–72 字节且包含字母和数字"))
		return
	}
	g.prepareRecovery(w, r, "setup", v.Username, passwordHash(v.Password))
}

func (g *Gate) portalAdminAPI(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.ctx.Err() != nil {
		reply(w, 503, map[string]string{"error": "服务正在停止"})
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	// JSON plus Origin checks prevent cross-site forms and credentialed fetches.
	if !sameOrigin(r) {
		reply(w, 403, map[string]string{"error": "跨站请求被拒绝"})
		return
	}
	var v struct {
		Username, Password, RecoveryKey, NewPassword, Ticket string
		Saved                                                bool
	}
	if err := body(w, r, &v); err != nil {
		fail(w, err)
		return
	}
	mac, err := g.peerMAC(r)
	if err != nil {
		fail(w, err)
		return
	}
	action := strings.TrimPrefix(r.URL.Path, "/api/admin-access/")
	if action == "download" || action == "commit" {
		p := g.recoveries[v.Ticket]
		if p == nil || p.MAC != mac || p.IP != peer(r) || p.Expires < time.Now().Unix() || p.OldHash != g.db.Config.AdminHash || p.OldRecovery != g.db.Config.RecoveryHash || p.BootToken != g.bootToken {
			reply(w, 403, map[string]string{"error": "恢复流程已失效，请重新开始"})
			return
		}
		if p.Mode == "setup" && (g.bootToken == "" || time.Now().Unix() > g.bootExpiry) {
			reply(w, 403, map[string]string{"error": "初始化凭证已过期，请重新生成"})
			return
		}
		if p.Mode == "enroll" && !g.isAdminDevice(mac) {
			reply(w, 403, map[string]string{"error": "此设备的管理员权限已被移除"})
			return
		}
		if action == "download" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="UFI-Gate-recovery-key.txt"`)
			_, err = fmt.Fprintf(w, "UFI-Gate 管理员恢复密钥\r\n\r\n%s\r\n\r\n管理员：%s\r\n网关：%s\r\n生成时间：%s\r\n\r\n请完成页面上的保存确认，此密钥才会生效。\r\n连接此热点，打开 http://%s:8090/，选择管理员登录 → 忘记密码。\r\n此密钥等同管理员恢复权限，请离线妥善保存，勿分享或上传。\r\n仅适用于本设备；成功重置密码后立即失效，请下载新密钥。\r\n", p.Key, p.Username, g.db.Config.Gateway, time.Now().In(zone).Format(time.RFC3339), g.db.Config.Gateway)
			if err == nil {
				p.Downloaded = true
			}
			return
		}
		if !p.Downloaded || !v.Saved {
			fail(w, errors.New("请先下载 TXT 恢复密钥并确认已保存"))
			return
		}
		old := g.db.Config
		g.db.Config.RecoveryHash = recoveryDigest(p.Key)
		if p.Mode == "setup" || p.Mode == "reset" {
			g.db.Config.AdminName = p.Username
			g.db.Config.AdminHash = p.PasswordHash
		}
		macs := append([]string{}, adminDeviceMACs(old)...)
		if p.Mode == "setup" {
			macs = []string{}
		}
		if !contains(macs, mac) {
			macs = append(macs, mac)
		}
		g.db.Config.AdminMACs = macs
		g.db.Config.AdminMAC = macs[0]
		// Preserve every existing device record, including during reinitialization.
		// Only a newly seen initial owner receives the historical initial trust.
		newOwner := p.Mode == "setup" && g.db.Devices[mac] == nil
		if newOwner {
			d := g.device(mac)
			d.IP, d.Trusted = peer(r), true
		}
		if err = g.saveAdminChange(old); err != nil {
			if newOwner {
				delete(g.db.Devices, mac)
				_ = g.save()
			}
			fail(w, err)
			return
		}
		if p.Mode == "setup" {
			g.bootToken = ""
			g.bootExpiry = 0
			_ = os.Remove(filepath.Join(dir, "setup.token"))
		}
		if p.Mode == "setup" || p.Mode == "reset" {
			g.sessions = map[string]Session{}
		}
		g.recoveries = nil
		g.adminSession(w, r)
		g.event("admin", "管理员恢复密钥已保存，设备权限已更新："+mac+"（"+p.Mode+"）")
		_ = g.save()
		reply(w, 200, map[string]any{"ok": true, "adminURL": adminDestination(r)})
		return
	}
	if action != "login" && action != "reset" {
		http.NotFound(w, r)
		return
	}
	// A separate global bucket cannot be bypassed by rotating MAC/IP or filling
	// the ordinary visitor attempt map. Bound bcrypt work before verification.
	now := time.Now().Unix()
	if now < g.recoveryBlocked {
		reply(w, 429, map[string]string{"error": "管理员验证过于频繁，请稍后重试"})
		return
	}
	if now-g.recoveryWindow >= 60 {
		g.recoveryWindow = now
		g.recoveryRequests = 0
	}
	g.recoveryRequests++
	if g.recoveryRequests > 12 {
		reply(w, 429, map[string]string{"error": "管理员验证过于频繁，请一分钟后重试"})
		return
	}
	c := g.db.Config
	ok := false
	if action == "login" {
		ok = c.AdminHash != "" && v.Username == c.AdminName && passwordOK(c.AdminHash, v.Password)
	} else {
		ok = c.AdminHash != "" && c.RecoveryHash != "" && subtle.ConstantTimeCompare([]byte(recoveryDigest(v.RecoveryKey)), []byte(c.RecoveryHash)) == 1
	}
	if !ok {
		g.recoveryFailures++
		if g.recoveryFailures >= 5 {
			g.recoveryBlocked = now + 600
			g.recoveryFailures = 0
			g.event("admin", "门户管理员验证连续失败，已限制 10 分钟")
		}
		reply(w, 401, map[string]string{"error": "管理员凭据无效，或尚未初始化"})
		return
	}
	g.recoveryFailures = 0
	if action == "reset" {
		if !strong(v.NewPassword) {
			fail(w, errors.New("新密码须为 8–72 字节且包含字母和数字"))
			return
		}
		g.prepareRecovery(w, r, "reset", c.AdminName, passwordHash(v.NewPassword))
		return
	}
	if err = g.setAdminDevice(mac, true); err != nil {
		fail(w, err)
		return
	}
	g.adminSession(w, r)
	if c.RecoveryHash == "" {
		g.prepareRecovery(w, r, "enroll", c.AdminName, c.AdminHash)
		return
	}
	reply(w, 200, map[string]any{"ok": true, "adminURL": adminDestination(r)})
}
