package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func adminDeviceMACs(c Config) []string {
	if c.AdminMACs == nil && validMAC(c.AdminMAC) {
		return []string{c.AdminMAC}
	}
	return c.AdminMACs
}

func (g *Gate) isAdminDevice(mac string) bool { return contains(adminDeviceMACs(g.db.Config), mac) }

// Manual rate limits do not grant access or reset quota usage. The tighter limit wins.
func (g *Gate) effectiveRate(d *Device) int {
	if !g.allowed(d) {
		return 0
	}
	rate := d.ManualRateKBps
	quotaLimited := d.LimitReason != "" && d.Quota.Action == "slow" || d.Quota.SlowPercent > 0 && g.ratio(d) >= float64(d.Quota.SlowPercent)/100
	if quotaLimited && (rate == 0 || d.Quota.RateKBps < rate) {
		rate = d.Quota.RateKBps
	}
	return rate
}

func (g *Gate) adminInputRules() [][]string {
	rules := [][]string{{"-p", "udp", "--dport", "67", "-j", "ACCEPT"}, {"-p", "udp", "--dport", "5354", "-j", "ACCEPT"}, {"-p", "tcp", "--dport", "5354", "-j", "ACCEPT"}, {"-p", "tcp", "--dport", "8090", "-j", "ACCEPT"}}
	if g.db.Config.AdminHash == "" {
		// Only expose the setup port; other host services remain protected.
		rules = append(rules, []string{"-p", "tcp", "--dport", "8081", "-j", "ACCEPT"})
	} else {
		for _, mac := range adminDeviceMACs(g.db.Config) {
			rules = append(rules, []string{"-m", "mac", "--mac-source", mac, "-j", "ACCEPT"})
		}
	}
	return append(rules, []string{"-j", "DROP"})
}

// Update only INPUT. FORWARD, NAT, counters and active network sessions are untouched.
func (g *Gate) refreshAdminAccess() error {
	if demo || !g.db.Config.Enabled || !g.fw.installed {
		return nil
	}
	next := 1 - g.fw.inputSide
	chain := "UFIG_IA"
	if next == 1 {
		chain = "UFIG_IB"
	}
	if err := g.fw.ip("filter", "-F", chain); err != nil {
		return err
	}
	for _, rule := range g.adminInputRules() {
		if err := g.fw.ip("filter", append([]string{"-A", chain}, rule...)...); err != nil {
			return err
		}
	}
	if err := g.fw.ip("filter", "-R", "UFIG_I", "1", "-j", chain); err != nil {
		return err
	}
	g.fw.inputSide = next
	return nil
}

func (g *Gate) saveAdminChange(old Config) error {
	if err := g.save(); err != nil {
		g.db.Config = old
		return err
	}
	if err := g.refreshAdminAccess(); err != nil {
		g.db.Config = old
		if rollback := g.save(); rollback != nil {
			return fmt.Errorf("更新管理权限失败: %v；保存回滚失败: %w", err, rollback)
		}
		return err
	}
	return nil
}

func (g *Gate) setAdminDevice(mac string, enabled bool) error {
	old := g.db.Config
	macs := append([]string{}, adminDeviceMACs(old)...)
	if enabled {
		if !contains(macs, mac) {
			macs = append(macs, mac)
		}
	} else {
		filtered := []string{}
		for _, m := range macs {
			if m != mac {
				filtered = append(filtered, m)
			}
		}
		macs = filtered
		if len(macs) == 0 {
			return errors.New("至少保留一台管理员设备；如需全部清除，请在插件中重置初始化凭证")
		}
	}
	g.db.Config.AdminMACs = macs
	g.db.Config.AdminMAC = macs[0]
	if err := g.saveAdminChange(old); err != nil {
		return err
	}
	// Revoke sessions for the removed device; API access also checks the current MAC.
	if !enabled {
		if d := g.db.Devices[mac]; d != nil {
			for token, s := range g.sessions {
				if s.IP == d.IP {
					delete(g.sessions, token)
				}
			}
		}
	}
	g.event("admin", fmt.Sprintf("管理员设备 %s 权限=%v", mac, enabled))
	return g.save()
}

// Called with g.mu held, exclusively through the root-only local control socket.
func (g *Gate) resetAdmin() (string, error) {
	old := g.db.Config
	token := random(24)
	if err := atomicWrite(filepath.Join(dir, "setup.token"), []byte(token)); err != nil {
		return "", err
	}
	g.db.Config.AdminName = ""
	g.db.Config.AdminHash = ""
	g.db.Config.AdminMAC = ""
	g.db.Config.AdminMACs = []string{}
	if err := g.saveAdminChange(old); err != nil {
		if g.bootToken != "" {
			_ = atomicWrite(filepath.Join(dir, "setup.token"), []byte(g.bootToken))
		} else {
			_ = os.Remove(filepath.Join(dir, "setup.token"))
		}
		return "", err
	}
	g.sessions = map[string]Session{}
	g.attempts = map[string]Attempt{}
	g.bootToken = token
	g.bootExpiry = time.Now().Add(20 * time.Minute).Unix()
	g.event("admin", "管理员信息已重置，联网授权及流量记录保留")
	return token, nil
}

func (g *Gate) startControl() (net.Listener, error) {
	path := filepath.Join(dir, "control.sock")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(path, 0600); err != nil {
		l.Close()
		return nil, err
	}
	s := &http.Server{ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 25 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/health" {
			g.mu.Lock()
			defer g.mu.Unlock()
			if !g.ready || g.ctx.Err() != nil {
				http.Error(w, "not ready", 503)
				return
			}
			reply(w, 200, VersionInfo{Version: currentVersion(), Arch: buildArch(), Protocol: updateProtocol})
			return
		}
		if r.Method != "POST" || r.URL.Path != "/reset-admin" {
			http.NotFound(w, r)
			return
		}
		g.mu.Lock()
		defer g.mu.Unlock()
		if g.ctx.Err() != nil {
			http.Error(w, "服务正在停止", 503)
			return
		}
		token, err := g.resetAdmin()
		if err != nil {
			reply(w, 500, map[string]string{"error": err.Error()})
			return
		}
		reply(w, 200, map[string]string{"token": token})
	})}
	go s.Serve(l)
	return l, nil
}

func requestAdminReset() error {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(dir, "control.sock"))
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 25 * time.Second}
	r, err := client.Post("http://local/reset-admin", "application/json", nil)
	if err != nil {
		return fmt.Errorf("无法连接运行中的服务，请先启动服务后重试: %w", err)
	}
	defer r.Body.Close()
	var v struct{ Token, Error string }
	if err = json.NewDecoder(r.Body).Decode(&v); err != nil {
		return err
	}
	if r.StatusCode != 200 {
		return errors.New(v.Error)
	}
	fmt.Println("管理员信息已清除，已获准联网的设备继续正常使用。\n初始化凭证（20 分钟有效）：" + v.Token + "\n请使用连接此热点的设备访问后台，凭此凭证重新初始化。")
	return nil
}
