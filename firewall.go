package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Firewall struct {
	runner    func(string, ...string) (string, error)
	installed bool
	side      int
	binding   string
	policy    string
}

func (g *Gate) networkPreflight() error {
	if demo {
		return nil
	}
	if err := g.fw.preflight(g.db.Config.LAN); err != nil {
		return err
	}
	out, err := realRun("ip", "-4", "-o", "addr", "show", "dev", g.db.Config.LAN)
	if err != nil || !strings.Contains(out, "inet "+g.db.Config.Gateway+"/") {
		return errors.New("网关地址不属于配置的热点接口，拒绝启用")
	}
	if g.db.Config.LAN == "lo" || strings.HasPrefix(g.db.Config.LAN, "rmnet") {
		return errors.New("请选择热点接口，不能使用回环或蜂窝接口")
	}
	return nil
}

func (f *Firewall) ip(table string, args ...string) error {
	_, e := f.runner("iptables", append([]string{"-w", "3", "-t", table}, args...)...)
	return e
}
func (f *Firewall) ip6(args ...string) error {
	_, e := f.runner("ip6tables", append([]string{"-w", "3"}, args...)...)
	return e
}
func (f *Firewall) preflight(lan string) error {
	if demo {
		return nil
	}
	if os.Geteuid() != 0 {
		return errors.New("Root 权限不可用")
	}
	if !ifaceRE.MatchString(lan) {
		return errors.New("热点接口无效")
	}
	if _, e := os.Stat("/sys/class/net/" + lan); e != nil {
		return errors.New("热点接口不存在，请开启热点并检查接口名")
	}
	if e := f.ip("filter", "-N", "UFIG_TEST"); e != nil {
		return e
	}
	defer f.ip("filter", "-X", "UFIG_TEST")
	defer f.ip("filter", "-F", "UFIG_TEST")
	tests := [][]string{{"-m", "mac", "--mac-source", "02:00:00:00:00:01", "-j", "RETURN"}, {"-m", "comment", "--comment", "ug:test", "-j", "RETURN"}, {"-p", "tcp", "-m", "hashlimit", "--hashlimit-above", "128kb/s", "--hashlimit-burst", "256kb", "--hashlimit-mode", "srcip", "--hashlimit-name", "ugtest", "-j", "DROP"}}
	for _, a := range tests {
		if e := f.ip("filter", append([]string{"-A", "UFIG_TEST"}, a...)...); e != nil {
			return e
		}
	}
	if e := f.ip("nat", "-N", "UFIG_TEST"); e != nil {
		return e
	}
	defer f.ip("nat", "-X", "UFIG_TEST")
	defer f.ip("nat", "-F", "UFIG_TEST")
	if e := f.ip("nat", "-A", "UFIG_TEST", "-p", "tcp", "--dport", "80", "-j", "REDIRECT", "--to-ports", "8080"); e != nil {
		return e
	}
	if e := f.ip("mangle", "-N", "UFIG_TEST"); e != nil {
		return e
	}
	defer f.ip("mangle", "-X", "UFIG_TEST")
	defer f.ip("mangle", "-F", "UFIG_TEST")
	if e := f.ip("mangle", "-A", "UFIG_TEST", "-s", "127.0.0.2", "-m", "comment", "--comment", "ug:counter-probe"); e != nil {
		return e
	}
	if e := f.ip6("-N", "UFIG_TEST6"); e != nil {
		return errors.New("ip6tables 不可用，拒绝启用以防 IPv6 绕过: " + e.Error())
	}
	f.ip6("-F", "UFIG_TEST6")
	if e := f.ip6("-X", "UFIG_TEST6"); e != nil {
		return e
	}
	return nil
}
func (f *Firewall) newChain(table, name string) error {
	if e := f.ip(table, "-N", name); e != nil {
		return e
	}
	return nil
}
func (f *Firewall) init(lan string) error {
	// Never flush Android's built-in chains. Own all chains/hooks under UFIG_ prefix.
	for _, p := range [][2]string{{"filter", "UFIG_F"}, {"filter", "UFIG_I"}, {"filter", "UFIG_FA"}, {"filter", "UFIG_FB"}, {"filter", "UFIG_IA"}, {"filter", "UFIG_IB"}, {"nat", "UFIG_N"}, {"nat", "UFIG_NA"}, {"nat", "UFIG_NB"}, {"mangle", "UFIG_C"}} {
		if e := f.newChain(p[0], p[1]); e != nil {
			f.cleanup()
			return e
		}
	}
	// Default-drop roots protect the short policy-build interval.
	for _, c := range []string{"UFIG_F", "UFIG_I"} {
		if e := f.ip("filter", "-A", c, "-j", "DROP"); e != nil {
			return e
		}
		if e := f.ip("filter", "-A", c, "-j", "DROP"); e != nil {
			return e
		}
	}
	if e := f.ip("nat", "-A", "UFIG_N", "-j", "RETURN"); e != nil {
		return e
	}
	for _, a := range [][]string{{"-I", "FORWARD", "1", "-i", lan, "-j", "UFIG_F"}, {"-I", "FORWARD", "1", "-o", lan, "-j", "UFIG_F"}, {"-I", "INPUT", "1", "-i", lan, "-j", "UFIG_I"}} {
		if e := f.ip("filter", a...); e != nil {
			return e
		}
	}
	if e := f.ip("nat", "-I", "PREROUTING", "1", "-i", lan, "-j", "UFIG_N"); e != nil {
		return e
	}
	// POSTROUTING sees packets after every FORWARD filter, before SNAT. Rejected/limited packets never reach our counters.
	if e := f.ip("mangle", "-I", "POSTROUTING", "1", "-j", "UFIG_C"); e != nil {
		return e
	}
	if e := f.ip6("-N", "UFIG_6"); e != nil {
		return e
	}
	if e := f.ip6("-A", "UFIG_6", "-j", "DROP"); e != nil {
		return e
	}
	for _, a := range [][]string{{"-I", "FORWARD", "1", "-i", lan, "-j", "UFIG_6"}, {"-I", "FORWARD", "1", "-o", lan, "-j", "UFIG_6"}, {"-I", "INPUT", "1", "-i", lan, "-j", "UFIG_6"}} {
		if e := f.ip6(a...); e != nil {
			return e
		}
	}
	f.installed = true
	return nil
}
func sortedDevices(g *Gate) []*Device {
	a := []*Device{}
	for _, d := range g.db.Devices {
		if ipv4(d.IP) && validMAC(d.MAC) {
			a = append(a, d)
		}
	}
	sort.Slice(a, func(i, j int) bool { return a[i].MAC < a[j].MAC })
	return a
}
func (g *Gate) reconcile() (result error) {
	if demo {
		return g.save()
	}
	if !g.db.Config.Enabled {
		return g.save()
	}
	if g.ctx.Err() != nil {
		return errors.New("服务正在停止")
	}
	defer func() {
		if result != nil {
			g.db.Config.Enabled = false
			if err := g.fw.cleanup(); err != nil {
				g.event("error", "紧急规则清理失败: "+err.Error())
			}
			g.restoreOffload()
			g.event("error", "网络策略更新失败，认证已停用: "+result.Error())
			g.save()
		}
	}()
	c := &g.db.Config
	if c.OffloadOriginal == "" {
		out, e := realRun("settings", "get", "global", "tether_offload_disabled")
		if e != nil {
			return errors.New("无法读取 tether offload 状态: " + e.Error())
		}
		c.OffloadOriginal = strings.TrimSpace(out)
		if c.OffloadOriginal == "" {
			c.OffloadOriginal = "null"
		}
		if e = g.save(); e != nil {
			return e
		}
	}
	if _, e := realRun("settings", "put", "global", "tether_offload_disabled", "1"); e != nil {
		return e
	}
	out, e := realRun("settings", "get", "global", "tether_offload_disabled")
	if e != nil || strings.TrimSpace(out) != "1" {
		return errors.New("无法关闭 Android tether offload")
	}
	f := g.fw
	if !f.installed {
		if e = f.init(c.LAN); e != nil {
			f.cleanup()
			g.restoreOffload()
			return e
		}
	}
	g.sampleBlocks()
	g.blockCounters = map[string]uint64{}
	// Build an inactive complete chain then swap the root's first rule. Existing policies never become empty.
	next := 1 - f.side
	suffix := "A"
	if next == 1 {
		suffix = "B"
	}
	fc, ic, nc := "UFIG_F"+suffix, "UFIG_I"+suffix, "UFIG_N"+suffix
	for _, p := range [][2]string{{"filter", fc}, {"filter", ic}, {"nat", nc}} {
		if e = f.ip(p[0], "-F", p[1]); e != nil {
			return e
		}
	}
	add := func(table, chain string, a ...string) error {
		return f.ip(table, append([]string{"-A", chain}, a...)...)
	}
	accept := func(args ...string) error {
		// Permit through our gate, then return to Android's original FORWARD chain so native/other-plugin restrictions still apply.
		return add("filter", fc, append(append([]string{}, args...), "-j", "RETURN")...)
	}
	devices := sortedDevices(g)
	// Quota and bans have priority over white lists and established connections.
	for _, d := range devices {
		if d.Banned || g.state(d) == "quota" {
			if e = add("nat", nc, "-s", d.IP, "-m", "mac", "--mac-source", d.MAC, "-p", "tcp", "--dport", "80", "-j", "REDIRECT", "--to-ports", "8080"); e != nil {
				return e
			}
			if e = add("filter", fc, "-i", c.LAN, "-s", d.IP, "-m", "mac", "--mac-source", d.MAC, "-j", "DROP"); e != nil {
				return e
			}
			if e = add("filter", fc, "-o", c.LAN, "-d", d.IP, "-j", "DROP"); e != nil {
				return e
			}
		}
	}
	// Blacklisted names resolve to the gateway; catch their HTTP requests before authorized-device exemptions.
	if e = add("nat", nc, "-d", c.Gateway, "-p", "tcp", "--dport", "80", "-j", "REDIRECT", "--to-ports", "8080"); e != nil {
		return e
	}
	black := g.destinationIPs(false)
	white := g.destinationIPs(true)
	for _, ip := range black {
		if e = add("filter", fc, "-i", c.LAN, "-d", ip, "-m", "comment", "--comment", "ug:black", "-j", "DROP"); e != nil {
			return e
		}
		if e = add("filter", fc, "-o", c.LAN, "-s", ip, "-j", "DROP"); e != nil {
			return e
		}
		if e = add("nat", nc, "-d", ip, "-p", "tcp", "--dport", "80", "-j", "REDIRECT", "--to-ports", "8080"); e != nil {
			return e
		}
	}
	for i, d := range devices {
		if !g.allowed(d) {
			continue
		}
		if g.limited(d) {
			for _, direction := range []string{"u", "d"} {
				args := []string{"-i", c.LAN, "-s", d.IP, "-m", "mac", "--mac-source", d.MAC}
				mode := "srcip"
				if direction == "d" {
					args = []string{"-o", c.LAN, "-d", d.IP}
					mode = "dstip"
				}
				args = append(args, "-m", "hashlimit", "--hashlimit-above", strconv.Itoa(d.Quota.RateKBps)+"kb/s", "--hashlimit-burst", strconv.Itoa(d.Quota.RateKBps*2)+"kb", "--hashlimit-mode", mode, "--hashlimit-name", fmt.Sprintf("ug%s%s%d", suffix, direction, i), "-j", "DROP")
				if e = add("filter", fc, args...); e != nil {
					return e
				}
			}
		}
	}
	for _, ip := range white {
		if contains(black, ip) {
			continue
		}
		if e = accept("-i", c.LAN, "-d", ip, "-p", "tcp", "-m", "multiport", "--dports", "80,443"); e != nil {
			return e
		}
		if e = accept("-o", c.LAN, "-s", ip, "-m", "conntrack", "--ctstate", "ESTABLISHED"); e != nil {
			return e
		}
	}
	// Prevent routing into adjacent private networks. Whitelisted private servers remain reachable via the explicit rules above.
	for _, cidr := range []string{"0.0.0.0/8", "10.0.0.0/8", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.168.0.0/16", "224.0.0.0/4", "240.0.0.0/4"} {
		if e = add("filter", fc, "-i", c.LAN, "-d", cidr, "-j", "DROP"); e != nil {
			return e
		}
	}
	for _, d := range devices {
		if !g.allowed(d) {
			continue
		}

		if e = accept("-i", c.LAN, "-s", d.IP, "-m", "mac", "--mac-source", d.MAC); e != nil {
			return e
		}
		if e = accept("-o", c.LAN, "-d", d.IP, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED"); e != nil {
			return e
		}
	}
	if e = add("filter", fc, "-m", "comment", "--comment", "ug:blocked", "-j", "DROP"); e != nil {
		return e
	}
	// Portal and DNS are local. Only the installation device can reach admin and UFI's original control plane through hotspot INPUT.
	for _, a := range [][]string{{"-p", "udp", "--dport", "67", "-j", "ACCEPT"}, {"-p", "udp", "--dport", "5354", "-j", "ACCEPT"}, {"-p", "tcp", "--dport", "5354", "-j", "ACCEPT"}, {"-p", "tcp", "--dport", "8080", "-j", "ACCEPT"}, {"-m", "mac", "--mac-source", c.AdminMAC, "-j", "ACCEPT"}, {"-j", "DROP"}} {
		if e = add("filter", ic, a...); e != nil {
			return e
		}
	}
	// DNS is always intercepted; authenticated devices get real answers except for blacklisted names.
	for _, proto := range []string{"udp", "tcp"} {
		if e = add("nat", nc, "-p", proto, "--dport", "53", "-j", "REDIRECT", "--to-ports", "5354"); e != nil {
			return e
		}
	}
	for _, d := range devices {
		if g.allowed(d) {
			if e = add("nat", nc, "-s", d.IP, "-m", "mac", "--mac-source", d.MAC, "-j", "RETURN"); e != nil {
				return e
			}
		}
	}
	for _, ip := range white {
		if e = add("nat", nc, "-d", ip, "-p", "tcp", "-m", "multiport", "--dports", "80,443", "-j", "RETURN"); e != nil {
			return e
		}
	}
	if e = add("nat", nc, "-p", "tcp", "--dport", "80", "-j", "REDIRECT", "--to-ports", "8080"); e != nil {
		return e
	}
	// 443 is dropped by FORWARD. Redirecting TLS to HTTP would produce certificate errors, not a valid captive portal.
	if e = f.ip("filter", "-R", "UFIG_F", "1", "-j", fc); e != nil {
		return e
	}
	if e = f.ip("filter", "-R", "UFIG_F", "2", "-j", "RETURN"); e != nil {
		return e
	}
	if e = f.ip("filter", "-R", "UFIG_I", "1", "-j", ic); e != nil {
		return e
	}
	if e = f.ip("nat", "-R", "UFIG_N", "1", "-j", nc); e != nil {
		return e
	}
	f.side = next
	bindings := []string{}
	for _, d := range devices {
		bindings = append(bindings, d.MAC+"="+d.IP)
	}
	binding := strings.Join(bindings, ";")
	if binding != f.binding {
		g.sample()
		if e = f.ip("mangle", "-F", "UFIG_C"); e != nil {
			return e
		}
		g.counters = map[string]uint64{}
		for _, d := range devices {
			if e = add("mangle", "UFIG_C", "-s", d.IP, "-m", "comment", "--comment", "ug:"+d.MAC+":u"); e != nil {
				return e
			}
			if e = add("mangle", "UFIG_C", "-o", c.LAN, "-d", d.IP, "!", "-s", c.Gateway, "-m", "comment", "--comment", "ug:"+d.MAC+":d"); e != nil {
				return e
			}
		}
		f.binding = binding
	}
	return g.save()
}
func (g *Gate) destinationIPs(white bool) []string {
	m := g.resolvedBlack
	list := g.db.Config.Black
	if white {
		m = g.resolvedWhite
		list = g.db.Config.White
	}
	result := map[string]bool{}
	now := time.Now().Unix()
	for ip, expires := range m {
		if expires > now {
			result[ip] = true
		}
	}
	for _, s := range list {
		if ipv4(s) {
			result[s] = true
		}
	}
	a := []string{}
	for ip := range result {
		a = append(a, ip)
	}
	sort.Strings(a)
	return a
}
func (f *Firewall) cleanup() error {
	if demo {
		return nil
	}
	var failures []string
	// Enumerate only our jumps; interface name may have changed since last boot.
	for _, table := range []string{"filter", "nat", "mangle"} {
		out, e := f.runner("iptables", "-w", "3", "-t", table, "-S")
		if e != nil {
			failures = append(failures, e.Error())
			continue
		}
		for _, line := range strings.Split(out, "\n") {
			parts := strings.Fields(line)
			if len(parts) < 4 || parts[0] != "-A" || strings.HasPrefix(parts[1], "UFIG_") {
				continue
			}
			own := false
			for i, p := range parts {
				if p == "-j" && i+1 < len(parts) && strings.HasPrefix(parts[i+1], "UFIG_") {
					own = true
				}
			}
			if own {
				parts[0] = "-D"
				if e = f.ip(table, parts...); e != nil {
					failures = append(failures, e.Error())
				}
			}
		}
		for _, line := range strings.Split(out, "\n") {
			parts := strings.Fields(line)
			if len(parts) == 2 && parts[0] == "-N" && strings.HasPrefix(parts[1], "UFIG_") {
				f.ip(table, "-F", parts[1])
			}
		}
		for _, line := range strings.Split(out, "\n") {
			parts := strings.Fields(line)
			if len(parts) == 2 && parts[0] == "-N" && strings.HasPrefix(parts[1], "UFIG_") {
				if e = f.ip(table, "-X", parts[1]); e != nil {
					failures = append(failures, e.Error())
				}
			}
		}
	}
	out, e := f.runner("ip6tables", "-w", "3", "-S")
	if e == nil {
		for _, line := range strings.Split(out, "\n") {
			p := strings.Fields(line)
			if len(p) > 3 && p[0] == "-A" && !strings.HasPrefix(p[1], "UFIG_") && contains(p, "UFIG_6") {
				p[0] = "-D"
				if e = f.ip6(p...); e != nil {
					failures = append(failures, e.Error())
				}
			}
		}
		f.ip6("-F", "UFIG_6")
		f.ip6("-X", "UFIG_6")
	}
	f.installed = false
	f.binding = ""
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}
func (g *Gate) sample() bool {
	if demo || !g.fw.installed {
		return false
	}
	out, e := realRun("iptables", "-w", "3", "-t", "mangle", "-L", "UFIG_C", "-n", "-v", "-x")
	if e != nil {
		g.event("error", "流量计数器读取失败")
		return false
	}
	delta := time.Since(g.lastSample).Seconds()
	if delta < 0.1 {
		delta = 1
	}
	g.lastSample = time.Now()
	for _, d := range g.db.Devices {
		d.RateUp = 0
		d.RateDown = 0
	}
	var allUp, allDown float64
	changed := false
	for _, line := range strings.Split(out, "\n") {
		idx := strings.Index(line, "ug:")
		if idx < 0 {
			continue
		}
		tag := strings.TrimSpace(strings.Split(line[idx:], "*/")[0])
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		n, e := strconv.ParseUint(fields[1], 10, 64)
		if e != nil {
			continue
		}
		p := strings.Split(tag, ":")
		if len(p) != 8 {
			continue
		}
		mac := strings.Join(p[1:7], ":")
		d := g.db.Devices[mac]
		if d == nil {
			continue
		}
		g.roll(d)
		prev := g.counters[tag]
		g.counters[tag] = n
		var inc uint64
		if n >= prev {
			inc = n - prev
		} else {
			inc = n
		}
		if p[7] == "u" {
			d.Up += inc
			d.DayUp += inc
			d.RateUp = float64(inc) / delta
			allUp += d.RateUp
		} else {
			d.Down += inc
			d.DayDown += inc
			d.RateDown = float64(inc) / delta
			allDown += d.RateDown
		}
		d.SessionBytes += inc
		d.PeriodBytes += inc
		d.MonthBytes += inc
		if g.enforceQuota(d) {
			changed = true
		}
	}
	g.rates = append(g.rates, RatePoint{time.Now().Unix(), allUp, allDown})
	if len(g.rates) > 60 {
		g.rates = g.rates[len(g.rates)-60:]
	}
	g.sampleBlocks()
	return changed
}
func (g *Gate) sampleBlocks() {
	out, e := realRun("iptables", "-w", "3", "-L", "UFIG_F"+[]string{"A", "B"}[g.fw.side], "-n", "-v", "-x")
	if e != nil {
		return
	}
	day := time.Now().In(zone).Format("2006-01-02")
	if day != g.countersDay {
		g.blockedPackets = 0
		g.blackPackets = 0
		g.countersDay = day
	}
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		n, _ := strconv.ParseUint(f[0], 10, 64)
		if strings.Contains(l, "ug:blocked") {
			key := "blocked:" + strings.Join(f[2:], " ")
			prev := g.blockCounters[key]
			g.blockCounters[key] = n
			if n >= prev {
				g.blockedPackets += n - prev
			} else {
				g.blockedPackets += n
			}
		} else if strings.Contains(l, "ug:black") {
			key := "black:" + strings.Join(f[2:], " ")
			prev := g.blockCounters[key]
			g.blockCounters[key] = n
			if n >= prev {
				g.blackPackets += n - prev
			} else {
				g.blackPackets += n
			}
		}
	}
}
func (g *Gate) discover() bool {
	if demo {
		d := g.device("02:00:00:00:00:02")
		d.IP = "127.0.0.1"
		d.Name = "本机演示设备"
		d.LastSeen = time.Now().Unix()
		return false
	}
	out, e := realRun("ip", "-4", "neigh", "show", "dev", g.db.Config.LAN)
	if e != nil {
		return false
	}
	seen := map[string]string{}
	changed := false
	for _, p := range []string{"/data/misc/dhcp/dnsmasq.leases", "/data/misc/wifi/dnsmasq.leases", "/data/vendor/dhcp/dnsmasq.leases", "/data/misc/wifi/softapd.leases"} {
		if b, err := os.ReadFile(p); err == nil && len(b) < 256*1024 {
			for _, line := range strings.Split(string(b), "\n") {
				f := strings.Fields(line)
				if len(f) >= 4 && validMAC(f[1]) && f[3] != "*" {
					d := g.device(strings.ToLower(f[1]))
					if len(f[3]) <= 100 {
						d.Name = f[3]
					}
				}
			}
		}
	}
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) < 5 || !ipv4(f[0]) || strings.Contains(l, "FAILED") || strings.Contains(l, "INCOMPLETE") {
			continue
		}
		for i, p := range f {
			if p == "lladdr" && i+1 < len(f) && validMAC(f[i+1]) {
				mac := strings.ToLower(f[i+1])
				if _, exists := seen[f[0]]; exists {
					continue
				}
				seen[f[0]] = mac
				d := g.device(mac)
				if d.IP != f[0] {
					d.IP = f[0]
					changed = true
				}
				if strings.Contains(l, "REACHABLE") || strings.Contains(l, "DELAY") {
					d.LastSeen = time.Now().Unix()
				}
			}
		}
	}
	// Revoke stale IP rules immediately if DHCP moved that address to a different MAC.
	for _, d := range g.db.Devices {
		if owner, ok := seen[d.IP]; ok && owner != d.MAC {
			d.IP = ""
			changed = true
		}
	}
	return changed
}
func (g *Gate) loop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	lastPolicy := ""
	for {
		select {
		case <-g.ctx.Done():
			return
		case <-ticker.C:
		}
		g.mu.Lock()
		g.cleanupEphemeral()
		changed := g.discover()
		changed = g.sample() || changed
		for _, d := range g.db.Devices {
			g.roll(d)
		}
		parts := []string{}
		for _, d := range sortedDevices(g) {
			parts = append(parts, d.MAC+":"+g.state(d)+":"+strconv.FormatBool(g.limited(d)))
		}
		policy := strings.Join(parts, ";")
		if policy != lastPolicy {
			changed = true
			lastPolicy = policy
		}
		for _, m := range []map[string]int64{g.resolvedWhite, g.resolvedBlack} {
			for ip, t := range m {
				if t <= time.Now().Unix() {
					delete(m, ip)
					changed = true
				}
			}
		}
		for _, a := range g.db.Requests {
			if a.Status == "pending" && a.Expires < time.Now().Unix() {
				a.Status = "expired"
				a.Nonce = ""
			}
		}
		if g.db.Config.Enabled {
			if changed {
				if e := g.reconcile(); e != nil {
					g.event("error", "规则更新失败，停用并清理: "+e.Error())
					g.db.Config.Enabled = false
					g.fw.cleanup()
					g.restoreOffload()
				}
			}
			if !demo {
				for _, h := range [][]string{{"-C", "FORWARD", "-i", g.db.Config.LAN, "-j", "UFIG_F"}, {"-C", "FORWARD", "-o", g.db.Config.LAN, "-j", "UFIG_F"}, {"-C", "INPUT", "-i", g.db.Config.LAN, "-j", "UFIG_I"}} {
					if e := g.fw.ip("filter", h...); e != nil {
						g.event("error", "检测到规则丢失，停用服务，请检查其他插件冲突")
						g.db.Config.Enabled = false
						g.fw.cleanup()
						g.restoreOffload()
						break
					}
				}
			}
		}
		g.save()
		g.mu.Unlock()
	}
}
func (g *Gate) dnsDevice(ip string) *Device {
	for _, d := range g.db.Devices {
		if d.IP == ip {
			return d
		}
	}
	if demo && net.ParseIP(ip).IsLoopback() {
		return g.device("02:00:00:00:00:02")
	}
	return nil
}

func (g *Gate) watchHost() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	client := &http.Client{Timeout: 5 * time.Second}
	re := regexp.MustCompile(`(?s)<!--\s*\[KANO_PLUGIN_START\].*?-->(.*?)<!--\s*\[KANO_PLUGIN_END\].*?-->`)
	for {
		select {
		case <-g.ctx.Done():
			return
		case <-ticker.C:
		}
		resp, err := client.Get("http://127.0.0.1:2333/api/get_custom_head")
		if err != nil {
			continue
		}
		var v struct{ Text string }
		err = json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 8*1024*1024)).Decode(&v)
		resp.Body.Close()
		if err != nil || resp.StatusCode != 200 {
			continue
		}
		found := false
		enabled := false
		for _, m := range re.FindAllStringSubmatch(v.Text, -1) {
			if strings.Contains(m[1], "UFI_GATE_PLUGIN_ID_v1") {
				found = true
				enabled = enabled || !strings.Contains(m[1], "[kano_disabled]")
			}
		}
		// Older hosts may use raw script text without wrappers.
		if !found && strings.Contains(v.Text, "UFI_GATE_PLUGIN_ID_v1") {
			enabled = !strings.Contains(v.Text, "[kano_disabled]")
		}
		if !enabled {
			g.mu.Lock()
			g.db.Config.Enabled = false
			g.event("service", "宿主插件已停用或删除，恢复网络并停止服务")
			g.save()
			atomicWrite(filepath.Join(dir, "suspended"), []byte("host-disabled"))
			g.mu.Unlock()
			g.cancel()
			return
		}
	}
}
