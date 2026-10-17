// UFI-Gate: standalone IPv4 captive portal for rooted UFI-TOOLS devices.
// No third-party runtime dependencies. See README for protocol/firmware limits.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"
)

//go:embed web/*
var web embed.FS

const version = "1.2.2"

var dir string
var demo bool
var zone = time.FixedZone("Asia/Shanghai", 8*3600)
var macRE = regexp.MustCompile(`(?i)^(?:[0-9a-f]{2}:){5}[0-9a-f]{2}$`)
var ifaceRE = regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,32}$`)
var domainRE = regexp.MustCompile(`^(?:\*\.)?[a-z0-9](?:[a-z0-9.-]{0,251}[a-z0-9])?$`)

type Quota struct {
	Daily, Total, Session, Period uint64
	PeriodDays                    int
	Action                        string
	RateKBps                      int
	SlowPercent                   int
}
type Device struct {
	QuotaWarned                                                     bool
	ManualRateKBps                                                  int
	MAC, IP, Name, Note                                             string
	FirstSeen, LastSeen, Until                                      int64
	Trusted, Banned                                                 bool
	LimitReason                                                     string
	Quota                                                           Quota
	Up, Down, DayUp, DayDown, SessionBytes, PeriodBytes, MonthBytes uint64
	Day, Month, PeriodKey                                           string
	TotalBase, DailyBase, SessionBase, PeriodBase                   uint64
	RateUp, RateDown                                                float64
}
type Account struct {
	Name, Hash string
	Expires    int64
	Quota      *Quota
}
type Push struct {
	Kind, URL, Secret, PublicBase, SMTPHost, SMTPUser, SMTPPassword, SMTPFrom, SMTPTo string
	SMTPPort                                                                          int
}
type Config struct {
	RemoteApproval                                                            RemoteApproval
	RecoveryHash                                                              string
	NotifyPush                                                                map[string]bool
	AutoUpdateCheck                                                           bool
	AdminMACs                                                                 []string
	SiteLogo, PortalLogo                                                      string
	PortalHost                                                                string
	Enabled                                                                   bool
	LAN, Gateway, AdminMAC                                                    string
	SessionHours                                                              int
	Methods                                                                   []string
	DefaultQuota                                                              Quota
	Black, White                                                              []string
	Title, Subtitle, Contact, Warning, BlockWarning, Accent                   string
	CaptchaProvider, CaptchaSite, CaptchaSecret, CaptchaVerify, CaptchaScript string
	Push                                                                      Push
	AdminName, AdminHash, SigningKey                                          string
	OffloadOriginal                                                           string
	Verified                                                                  bool
}
type Request struct {
	ID, MAC, Name, Reason, Status, DecisionReason, Nonce string
	Created, Expires, Decided                            int64
}
type DB struct {
	LastUpdateNotice, LastReleaseNotice string
	Notifications                       []Notification
	Config                              Config
	Devices                             map[string]*Device
	Accounts                            map[string]Account
	Requests                            map[string]*Request
	Logs                                []Event
}
type Event struct {
	Time          int64
	Kind, Message string
}
type Session struct {
	Last int64
	IP   string
}
type Attempt struct {
	Failures int
	Until    int64
}
type Challenge struct {
	MAC, Answer string
	Expires     int64
}
type Gate struct {
	remoteServer                                    *http.Server
	remoteWindow, remoteTestExpires, remoteTestSent int64
	remoteRequests                                  int
	remoteTestNonce                                 string
	recoveries                                      map[string]*adminRecovery
	recoveryWindow, recoveryBlocked                 int64
	recoveryRequests, recoveryFailures              int
	ready                                           bool
	release                                         ReleaseInfo
	updateChecking, updatePreparing                 bool
	mu                                              sync.Mutex
	db                                              DB
	sessions                                        map[string]Session
	attempts                                        map[string]Attempt
	challenges                                      map[string]Challenge
	counters                                        map[string]uint64
	resolvedWhite, resolvedBlack                    map[string]int64
	bootToken                                       string
	bootExpiry                                      int64
	fw                                              *Firewall
	started                                         time.Time
	rates                                           []RatePoint
	lastSample                                      time.Time
	ctx                                             context.Context
	cancel                                          context.CancelFunc
	httpClient                                      *http.Client
	dnsSlots                                        chan struct{}
	countersDay                                     string
	blockedPackets, blackPackets                    uint64
	blockCounters                                   map[string]uint64
}
type RatePoint struct {
	Time     int64
	Up, Down float64
}

func random(n int) string {
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func passwordHash(p string) string {
	k, e := bcrypt.GenerateFromPassword([]byte(p), 12)
	if e != nil {
		panic(e)
	}
	return string(k)
}
func passwordOK(hash, p string) bool {
	return len(p) <= 72 && bcrypt.CompareHashAndPassword([]byte(hash), []byte(p)) == nil
}
func strong(p string) bool {
	return len(p) >= 8 && len(p) <= 72 && regexp.MustCompile(`[A-Za-z]`).MatchString(p) && regexp.MustCompile(`[0-9]`).MatchString(p)
}
func atomicWrite(p string, b []byte) error {
	f, e := os.OpenFile(p+".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(p+".tmp", p)
}
func (g *Gate) save() error {
	b, e := json.MarshalIndent(g.db, "", "  ")
	if e != nil {
		return e
	}
	return atomicWrite(filepath.Join(dir, "state.json"), b)
}
func (g *Gate) event(k, m string) {
	g.notificationEvent(k, m)
	g.db.Logs = append(g.db.Logs, Event{time.Now().Unix(), k, m})
	if len(g.db.Logs) > 500 {
		g.db.Logs = g.db.Logs[len(g.db.Logs)-500:]
	}
}
func loadGate() (*Gate, error) {
	ctx, cancel := context.WithCancel(context.Background())
	g := &Gate{ctx: ctx, cancel: cancel, started: time.Now(), lastSample: time.Now(), sessions: map[string]Session{}, attempts: map[string]Attempt{}, challenges: map[string]Challenge{}, counters: map[string]uint64{}, resolvedWhite: map[string]int64{}, resolvedBlack: map[string]int64{}, dnsSlots: make(chan struct{}, 32)}
	g.db = DB{Config: Config{SessionHours: 12, Methods: []string{"account", "captcha", "approval"}, DefaultQuota: Quota{PeriodDays: 30, Action: "block", RateKBps: 128, SlowPercent: 80}, Title: "连接，从这里开始", Subtitle: "欢迎使用共享网络。完成认证，即可安心上网。", Contact: "请联系网络管理员", Warning: "流量已达到管理员设置的上限，请联系管理员解除限制。", BlockWarning: "该网站已被管理员禁止访问。", Accent: "#0067c0", SigningKey: random(32)}, Devices: map[string]*Device{}, Accounts: map[string]Account{}, Requests: map[string]*Request{}}
	b, e := os.ReadFile(filepath.Join(dir, "state.json"))
	if e == nil {
		if e = json.Unmarshal(b, &g.db); e != nil {
			return nil, fmt.Errorf("状态文件损坏，拒绝覆盖: %w", e)
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	if g.db.Devices == nil || g.db.Accounts == nil || g.db.Requests == nil {
		return nil, errors.New("状态文件结构无效")
	}
	if g.db.Config.SigningKey == "" {
		g.db.Config.SigningKey = random(32)
	}
	if g.db.Config.LAN == "" {
		var boot struct{ LAN, Gateway string }
		if b, err := os.ReadFile(filepath.Join(dir, "bootstrap.json")); err == nil && json.Unmarshal(b, &boot) == nil && ifaceRE.MatchString(boot.LAN) && ipv4(boot.Gateway) {
			g.db.Config.LAN = boot.LAN
			g.db.Config.Gateway = boot.Gateway
		}
	}
	if g.db.Config.AdminMACs == nil && validMAC(g.db.Config.AdminMAC) {
		g.db.Config.AdminMACs = []string{g.db.Config.AdminMAC}
	}
	g.blockCounters = map[string]uint64{}
	g.countersDay = time.Now().In(zone).Format("2006-01-02")
	if g.db.Config.AdminHash == "" {
		token, e := os.ReadFile(filepath.Join(dir, "setup.token"))
		if e == nil {
			g.bootToken = strings.TrimSpace(string(token))
		}
		if len(g.bootToken) < 32 {
			g.bootToken = random(24)
			if e = atomicWrite(filepath.Join(dir, "setup.token"), []byte(g.bootToken)); e != nil {
				return nil, e
			}
		}
		g.bootExpiry = time.Now().Add(20 * time.Minute).Unix()
		if info, err := os.Stat(filepath.Join(dir, "setup.token")); err == nil {
			g.bootExpiry = info.ModTime().Add(20 * time.Minute).Unix()
		}
	}
	g.fw = &Firewall{runner: realRun}
	roots, _ := x509.SystemCertPool()
	if roots == nil {
		roots = x509.NewCertPool()
	}
	entries, _ := os.ReadDir("/system/etc/security/cacerts")
	for _, f := range entries {
		b, _ := os.ReadFile("/system/etc/security/cacerts/" + f.Name())
		roots.AppendCertsFromPEM(b)
	}
	resolver := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, n, a string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "udp", "223.5.5.5:53")
	}}
	g.httpClient = &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}, DialContext: (&net.Dialer{Timeout: 6 * time.Second, Resolver: resolver}).DialContext}, CheckRedirect: func(r *http.Request, v []*http.Request) error { return http.ErrUseLastResponse }}
	return g, nil
}
func validMAC(s string) bool {
	if !macRE.MatchString(s) {
		return false
	}
	b, _ := hex.DecodeString(strings.ReplaceAll(s, ":", ""))
	return b[0]&1 == 0 && s != "00:00:00:00:00:00"
}
func ipv4(s string) bool          { ip := net.ParseIP(s); return ip != nil && ip.To4() != nil }
func peer(r *http.Request) string { h, _, _ := net.SplitHostPort(r.RemoteAddr); return h }
func (g *Gate) peerMAC(r *http.Request) (string, error) {
	ip := peer(r)
	if demo && net.ParseIP(ip).IsLoopback() {
		return "02:00:00:00:00:02", nil
	}
	if b, err := os.ReadFile("/proc/net/arp"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) >= 6 && f[0] == ip && f[2] == "0x2" && f[5] == g.db.Config.LAN && validMAC(f[3]) {
				return strings.ToLower(f[3]), nil
			}
		}
	}
	// Only use kernel neighbour bindings on the configured hotspot interface. Ignore all browser-supplied MAC/IP fields.
	out, e := realRun("ip", "-4", "neigh", "show", "dev", g.db.Config.LAN)
	if e != nil {
		return "", g.peerNetworkError(true)
	}
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) < 5 || f[0] != ip {
			continue
		}
		wrongInterface := false
		for i := 0; i+1 < len(f); i++ {
			if f[i] == "dev" && f[i+1] != g.db.Config.LAN {
				wrongInterface = true
			}
		}
		if wrongInterface {
			continue
		}
		for i := 0; i+1 < len(f); i++ {
			if f[i] == "lladdr" && validMAC(f[i+1]) && !strings.Contains(l, "FAILED") {
				return strings.ToLower(f[i+1]), nil
			}
		}
	}
	return "", g.peerNetworkError(false)
}
func (g *Gate) device(mac string) *Device {
	d := g.db.Devices[mac]
	if d == nil {
		n := time.Now().Unix()
		d = &Device{MAC: mac, FirstSeen: n, LastSeen: n, Quota: g.db.Config.DefaultQuota}
		g.db.Devices[mac] = d
	}
	g.roll(d)
	return d
}
func (g *Gate) roll(d *Device) {
	t := time.Now().In(zone)
	day := t.Format("2006-01-02")
	month := t.Format("2006-01")
	days := d.Quota.PeriodDays
	if days < 1 {
		days = 30
	}
	key := strconv.FormatInt((time.Now().Unix()+8*3600)/86400/int64(days), 10)
	if d.Day != day {
		d.Day = day
		d.DayUp = 0
		d.DayDown = 0
		d.DailyBase = 0
	}
	if d.Month != month {
		d.Month = month
		d.MonthBytes = 0
	}
	if d.PeriodKey != key {
		d.PeriodKey = key
		d.PeriodBytes = 0
		d.PeriodBase = 0
	}
}
func used(n, b uint64) uint64 {
	if n < b {
		return 0
	}
	return n - b
}
func (g *Gate) ratio(d *Device) float64 {
	q := d.Quota
	vals := [][2]uint64{{used(d.DayUp+d.DayDown, d.DailyBase), q.Daily}, {used(d.Up+d.Down, d.TotalBase), q.Total}, {used(d.SessionBytes, d.SessionBase), q.Session}, {used(d.PeriodBytes, d.PeriodBase), q.Period}}
	var max float64
	for _, v := range vals {
		if v[1] > 0 {
			r := float64(v[0]) / float64(v[1])
			if r > max {
				max = r
			}
		}
	}
	return max
}
func (g *Gate) state(d *Device) string {
	if d.Banned {
		return "blocked"
	}
	if d.LimitReason != "" {
		if d.Quota.Action == "slow" {
			return "limited"
		}
		return "quota"
	}
	if d.Trusted || d.Until > time.Now().Unix() {
		if d.ManualRateKBps > 0 {
			return "limited"
		}
		return "online"
	}
	return "pending"
}
func (g *Gate) allowed(d *Device) bool { s := g.state(d); return s == "online" || s == "limited" }
func (g *Gate) limited(d *Device) bool {
	return g.effectiveRate(d) > 0
}
func (g *Gate) enforceQuota(d *Device) bool {
	if d.LimitReason == "" && g.ratio(d) >= 1 {
		d.LimitReason = "quota"
		g.event("quota", d.MAC+" 达到流量配额")
		return true
	}
	return false
}
func (g *Gate) grant(d *Device) error {
	return g.grantUntil(d, 0)
}
func (g *Gate) grantUntil(d *Device, expires int64) error {
	if expires != 0 && expires <= time.Now().Unix() {
		return errors.New("上网账号已过期")
	}
	if g.ctx.Err() != nil {
		return errors.New("服务正在停止")
	}
	if d.Banned || d.LimitReason != "" {
		return errors.New("设备已被限制，请联系管理员")
	}
	if g.allowed(d) {
		return g.reconcile()
	}
	g.sample()
	if d.LimitReason != "" {
		return errors.New("设备已达到配额，请联系管理员解除")
	}
	d.Until = time.Now().Add(time.Duration(g.db.Config.SessionHours) * time.Hour).Unix()
	if expires != 0 && expires < d.Until {
		d.Until = expires
	}
	d.SessionBytes = 0
	d.SessionBase = 0
	g.event("auth", d.MAC+" 认证通过")
	return g.reconcile()
}
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, e error) { reply(w, 400, map[string]any{"error": e.Error()}) }
func body(w http.ResponseWriter, r *http.Request, v any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return errors.New("需要 JSON 请求")
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128*1024))
	dec.DisallowUnknownFields()
	if e := dec.Decode(v); e != nil {
		return errors.New("请求格式无效")
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return errors.New("请求格式无效")
	}
	return nil
}
func sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return r.Header.Get("Sec-Fetch-Site") != "cross-site"
	}
	u, e := url.Parse(o)
	return e == nil && u.Host == r.Host && (u.Scheme == "http" || u.Scheme == "https")
}
func (g *Gate) admin(w http.ResponseWriter, r *http.Request) bool {
	c, e := r.Cookie("ug_session")
	if e != nil {
		return false
	}
	s, ok := g.sessions[c.Value]
	now := time.Now().Unix()
	if !ok || s.IP != peer(r) || now-s.Last > 1800 {
		delete(g.sessions, c.Value)
		return false
	}
	// Automatic dashboard polling must not keep an unattended administrator session alive forever.
	if r.URL.Path != "/admin-api/dashboard" {
		s.Last = now
		g.sessions[c.Value] = s
		http.SetCookie(w, &http.Cookie{Name: "ug_session", Value: c.Value, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https", MaxAge: 1800})
	}
	return true
}
func (g *Gate) attempt(k string) bool { a := g.attempts[k]; return time.Now().Unix() >= a.Until }
func (g *Gate) badAttempt(k string) {
	a := g.attempts[k]
	a.Failures++
	if a.Failures >= 5 {
		g.notify("auth_failure", "连续认证失败，已暂时限制："+k, "logs", "auth:"+k, true)
		a.Until = time.Now().Add(10 * time.Minute).Unix()
		a.Failures = 0
	}
	g.attempts[k] = a
}
func (g *Gate) publicDevice(d *Device) map[string]any {
	return map[string]any{"device": d, "status": g.state(d), "ratio": g.ratio(d), "limited": g.limited(d), "admin": g.isAdminDevice(d.MAC), "rateKBps": g.effectiveRate(d)}
}
func (g *Gate) cleanupEphemeral() {
	now := time.Now().Unix()
	for id, p := range g.recoveries {
		if p.Expires < now {
			delete(g.recoveries, id)
		}
	}
	for k, s := range g.sessions {
		if now-s.Last > 1800 {
			delete(g.sessions, k)
		}
	}
	for k, c := range g.challenges {
		if c.Expires < now {
			delete(g.challenges, k)
		}
	}
	if len(g.attempts) > 2048 {
		g.attempts = map[string]Attempt{}
	}
}
func (g *Gate) portalAPI(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.ctx.Err() != nil {
		reply(w, 503, map[string]string{"error": "服务正在停止"})
		return
	}
	g.cleanupEphemeral()
	mac, e := g.peerMAC(r)
	if e != nil {
		reply(w, 403, map[string]string{"error": e.Error()})
		return
	}
	d := g.device(mac)
	for _, other := range g.db.Devices {
		if other.MAC != mac && other.IP == peer(r) {
			other.IP = ""
		}
	}
	d.IP = peer(r)
	d.LastSeen = time.Now().Unix()
	g.enforceQuota(d)
	if r.Method == "GET" && r.URL.Path == "/api/status" {
		c := g.db.Config
		var pending *Request
		for _, a := range g.db.Requests {
			if a.MAC == mac && (pending == nil || a.Created > pending.Created) {
				pending = a
			}
		}
		reply(w, 200, map[string]any{"enabled": c.Enabled, "title": c.Title, "subtitle": c.Subtitle, "contact": c.Contact, "warning": c.Warning, "blockWarning": c.BlockWarning, "accent": c.Accent, "portalLogo": c.PortalLogo, "methods": c.Methods, "captchaProvider": c.CaptchaProvider, "captchaSite": c.CaptchaSite, "captchaScript": c.CaptchaScript, "state": g.publicDevice(d), "request": pending})
		return
	}
	if !g.db.Config.Enabled {
		fail(w, errors.New("网络认证当前未开启"))
		return
	}
	if r.Method == "GET" && r.URL.Path == "/api/challenge" {
		if len(g.challenges) >= 1024 {
			fail(w, errors.New("验证请求较多，请稍后重试"))
			return
		}
		answer := ""
		alphabet := "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
		b := make([]byte, 5)
		rand.Read(b)
		for _, v := range b {
			answer += string(alphabet[int(v)%len(alphabet)])
		}
		id := random(16)
		g.challenges[id] = Challenge{mac, answer, time.Now().Add(2 * time.Minute).Unix()}
		reply(w, 200, map[string]string{"id": id, "svg": captchaSVG(answer)})
		return
	}
	if r.Method != "POST" || !sameOrigin(r) {
		reply(w, 403, map[string]string{"error": "请求被拒绝"})
		return
	}
	var v struct{ Method, Username, Password, ChallengeID, Answer, Token, Name, Reason string }
	if e = body(w, r, &v); e != nil {
		fail(w, e)
		return
	}
	if d.Banned || d.LimitReason != "" {
		fail(w, errors.New("设备已被限制，请联系管理员解除"))
		return
	}
	if r.URL.Path == "/api/request" {
		if !contains(g.db.Config.Methods, "approval") {
			fail(w, errors.New("审批认证未开启"))
			return
		}
		if len(v.Name) > 100 || len(v.Reason) > 1000 {
			fail(w, errors.New("昵称或理由过长"))
			return
		}
		for _, a := range g.db.Requests {
			if a.MAC == mac && a.Status == "pending" && a.Expires > time.Now().Unix() {
				reply(w, 200, a)
				return
			}
		}
		if len(g.db.Requests) >= 2000 {
			fail(w, errors.New("审批记录已满，请管理员清理历史记录"))
			return
		}
		key := "request:" + mac
		if !g.attempt(key) {
			fail(w, errors.New("请求过于频繁"))
			return
		}
		a := &Request{ID: random(12), MAC: mac, Name: v.Name, Reason: v.Reason, Status: "pending", Created: time.Now().Unix(), Expires: time.Now().Add(30 * time.Minute).Unix(), Nonce: random(16)}
		g.db.Requests[a.ID] = a
		g.attempts[key] = Attempt{Until: time.Now().Add(60 * time.Second).Unix()}
		g.event("approval", mac+" 申请上网")
		g.notify("approval", mac+" 申请上网", "approvals", "approval:"+a.ID, true).RequestID = a.ID
		if e = g.save(); e != nil {
			delete(g.db.Requests, a.ID)
			fail(w, e)
			return
		}

		reply(w, 200, a)
		return
	}
	if r.URL.Path != "/api/auth" || !contains(g.db.Config.Methods, v.Method) {
		fail(w, errors.New("认证方式不可用"))
		return
	}
	key := "auth:" + mac
	if !g.attempt(key) {
		fail(w, errors.New("尝试次数过多，10 分钟后重试"))
		return
	}
	ok := false
	accountExpiry := int64(0)
	switch v.Method {
	case "account":
		a, exists := g.db.Accounts[v.Username]
		accountExpiry = a.Expires
		ok = exists && (a.Expires == 0 || a.Expires > time.Now().Unix()) && passwordOK(a.Hash, v.Password)
		if ok && a.Quota != nil {
			d.Quota = *a.Quota
			g.enforceQuota(d)
		}
	case "captcha":
		c, exists := g.challenges[v.ChallengeID]
		delete(g.challenges, v.ChallengeID)
		ok = exists && c.MAC == mac && c.Expires > time.Now().Unix() && strings.EqualFold(c.Answer, strings.TrimSpace(v.Answer))
	case "thirdparty":
		c := g.db.Config
		g.mu.Unlock()
		ok = g.verifyCaptcha(c, v.Token, peer(r))
		g.mu.Lock()
		ok = ok && g.db.Config.Enabled && contains(g.db.Config.Methods, "thirdparty") && c.CaptchaSite == g.db.Config.CaptchaSite && c.CaptchaSecret == g.db.Config.CaptchaSecret && c.CaptchaProvider == g.db.Config.CaptchaProvider
	}
	if !ok {
		g.badAttempt(key)
		fail(w, errors.New("认证未通过，请检查后重试"))
		return
	}
	delete(g.attempts, key)
	if e = g.grantUntil(d, accountExpiry); e != nil {
		fail(w, e)
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}
func contains(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}
func (g *Gate) adminAPI(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.ctx.Err() != nil {
		reply(w, 503, map[string]string{"error": "服务正在停止"})
		return
	}
	g.cleanupEphemeral()
	if r.Method == "POST" && !sameOrigin(r) {
		reply(w, 403, map[string]string{"error": "跨站请求被拒绝"})
		return
	}
	path := r.URL.Path
	if g.db.Config.AdminHash != "" {
		mac, err := g.peerMAC(r)
		if err != nil || !g.isAdminDevice(mac) {
			reply(w, 403, map[string]string{"error": "此设备没有后台管理权限"})
			return
		}
	}
	if path == "/admin-api/ready" && r.Method == "GET" {
		reply(w, 200, map[string]any{"version": currentVersion(), "setup": g.db.Config.AdminHash == ""})
		return
	}
	if path == "/admin-api/setup" && r.Method == "POST" {
		g.prepareAdminSetup(w, r)
		return
	}
	if path == "/admin-api/login" && r.Method == "POST" {
		var v struct{ Username, Password, Token string }
		if e := body(w, r, &v); e != nil {
			fail(w, e)
			return
		}
		key := "login:" + peer(r)
		if !g.attempt(key) {
			reply(w, 429, map[string]string{"error": "密码错误过多，请在 10 分钟后重试"})
			return
		}
		if g.db.Config.AdminHash == "" || v.Username != g.db.Config.AdminName || !passwordOK(g.db.Config.AdminHash, v.Password) {
			g.badAttempt(key)
			reply(w, 401, map[string]string{"error": "账号或密码错误"})
			return
		}
		delete(g.attempts, key)
		g.adminSession(w, r)
		reply(w, 200, map[string]bool{"ok": true})
		return
	}

	if !g.admin(w, r) {
		reply(w, 401, map[string]string{"error": "请先登录管理后台"})
		return
	}
	if path == "/admin-api/recovery-prepare" && r.Method == "POST" {
		var v struct{}
		if e := body(w, r, &v); e != nil {
			fail(w, e)
			return
		}
		if g.db.Config.RecoveryHash != "" {
			fail(w, errors.New("已配置恢复密钥"))
			return
		}
		g.prepareRecovery(w, r, "enroll", g.db.Config.AdminName, g.db.Config.AdminHash)
		return
	}
	if g.updateAPI(w, r) || g.notificationsAPI(w, r) {
		return
	}
	if r.Method == "GET" {
		switch path {
		case "/admin-api/dashboard":
			devices := []map[string]any{}
			var up, down, today, month, total uint64
			var online, approved, limited int
			for _, d := range g.db.Devices {
				g.roll(d)
				devices = append(devices, g.publicDevice(d))
				today += d.DayUp + d.DayDown
				month += d.MonthBytes
				total += d.Up + d.Down
				up += uint64(d.RateUp)
				down += uint64(d.RateDown)
				if ipv4(d.IP) && time.Now().Unix()-d.LastSeen < 120 {
					online++
				}
				if g.allowed(d) {
					approved++
				}
				if g.limited(d) {
					limited++
				}
			}
			requests := []*Request{}
			for _, a := range g.db.Requests {
				requests = append(requests, a)
			}
			sort.Slice(requests, func(i, j int) bool { return requests[i].Created > requests[j].Created })
			reply(w, 200, map[string]any{"enabled": g.db.Config.Enabled, "uptime": int(time.Since(g.started).Seconds()), "devices": devices, "requests": requests, "today": today, "month": month, "total": total, "up": up, "down": down, "online": online, "approved": approved, "limited": limited, "blocked": g.blockedPackets, "blackHits": g.blackPackets, "rates": g.rates, "version": currentVersion(), "arch": buildArch()})
		case "/admin-api/config":
			c := g.db.Config
			c.AdminHash = ""
			c.RecoveryHash = ""
			c.SigningKey = ""
			c.CaptchaSecret = ""
			c.Push.Secret = ""
			c.Push.SMTPPassword = ""
			reply(w, 200, map[string]any{"config": c, "hasRecoveryKey": g.db.Config.RecoveryHash != "", "hasCaptchaSecret": g.db.Config.CaptchaSecret != "", "hasPushSecret": g.db.Config.Push.Secret != "", "hasSMTPPassword": g.db.Config.Push.SMTPPassword != ""})
		case "/admin-api/accounts":
			a := []Account{}
			for _, v := range g.db.Accounts {
				v.Hash = ""
				a = append(a, v)
			}
			reply(w, 200, a)
		case "/admin-api/logs":
			reply(w, 200, g.db.Logs)
		case "/admin-api/diagnostics":
			reply(w, 200, g.diagnostics())
		default:
			reply(w, 404, map[string]string{"error": "接口不存在"})
		}
		return
	}
	if r.Method != "POST" {
		reply(w, 405, map[string]string{"error": "方法不支持"})
		return
	}
	var e error
	switch path {
	case "/admin-api/logout":
		c, _ := r.Cookie("ug_session")
		delete(g.sessions, c.Value)
		http.SetCookie(w, &http.Cookie{Name: "ug_session", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		reply(w, 200, map[string]bool{"ok": true})
		return
	case "/admin-api/config":
		var v Config
		if e = body(w, r, &v); e != nil {
			break
		}
		old := g.db.Config
		v.NotifyPush = old.NotifyPush
		v.AutoUpdateCheck = old.AutoUpdateCheck
		v.Enabled = old.Enabled
		v.AdminMAC = old.AdminMAC
		v.AdminMACs = old.AdminMACs
		v.AdminName = old.AdminName
		v.AdminHash = old.AdminHash
		v.RecoveryHash = old.RecoveryHash
		v.SigningKey = old.SigningKey
		v.OffloadOriginal = old.OffloadOriginal
		if v.CaptchaSecret == "" {
			v.CaptchaSecret = old.CaptchaSecret
		}
		if v.Push.Secret == "" {
			v.Push.Secret = old.Push.Secret
		}
		if v.Push.SMTPPassword == "" {
			v.Push.SMTPPassword = old.Push.SMTPPassword
		}
		if e = validateConfig(v); e != nil {
			break
		}
		if !demo && (old.LAN != v.LAN || old.Gateway != v.Gateway) {
			validation := validateNetwork(inspectNetwork(), v.LAN, v.Gateway)
			if validation.Status != "valid" {
				e = errors.New(validation.Message)
				break
			}
		}
		if old.Enabled && (old.LAN != v.LAN || old.Gateway != v.Gateway) {
			e = errors.New("修改热点接口/网关前请先关闭认证")
			break
		}
		listener, listenErr := g.prepareRemoteApproval(v.RemoteApproval)
		if listenErr != nil {
			e = fmt.Errorf("远程审批监听失败: %w", listenErr)
			break
		}
		g.db.Config = v
		g.resolvedWhite = map[string]int64{}
		g.resolvedBlack = map[string]int64{}
		e = g.reconcile()
		if e != nil {
			g.db.Config = old
			g.reconcile()
			if listener != nil {
				listener.Close()
			}
		} else {
			g.commitRemoteApproval(listener)
			if old.RemoteApproval != v.RemoteApproval {
				g.remoteTestNonce = ""
			}
		}
		if e != nil {
			fail(w, e)
		} else {
			reply(w, 200, map[string]bool{"ok": true})
		}
		return
	case "/admin-api/toggle":
		var v struct{ Enabled, Confirmed bool }
		if e = body(w, r, &v); e != nil {
			break
		}
		if v.Enabled {
			if e = validateConfig(g.db.Config); e != nil {
				break
			}
			if !g.db.Config.Verified || !v.Confirmed {
				reply(w, 400, map[string]string{"code": "confirmation_required", "error": "请在服务设置中核对接口、加速设置和 Root 恢复通道，勾选确认并保存后再开启认证"})
				return
			}
			if e = g.networkPreflight(); e != nil {
				break
			}
			g.db.Config.Enabled = true
			e = g.reconcile()
			if e != nil {
				g.db.Config.Enabled = false
				g.fw.cleanup()
			}
		} else {
			g.sample()
			g.db.Config.Enabled = false
			e = g.fw.cleanup()
			g.restoreOffload()
		}
		g.event("network", fmt.Sprintf("认证服务启用=%v", g.db.Config.Enabled))
	case "/admin-api/device":
		var v struct {
			MAC, Action, Note string
			RateKBps          int
			Quota             *Quota
		}
		if e = body(w, r, &v); e != nil {
			break
		}
		v.MAC = strings.ToLower(v.MAC)
		if !validMAC(v.MAC) {
			e = errors.New("MAC 地址无效")
			break
		}
		g.sample()
		d := g.device(v.MAC)
		switch v.Action {
		case "admin-add", "admin-remove":
			e = g.setAdminDevice(v.MAC, v.Action == "admin-add")
			if e != nil {
				fail(w, e)
				return
			}
			reply(w, 200, map[string]bool{"ok": true})
			return
		case "rate":
			if v.RateKBps < 0 || v.RateKBps > 102400 {
				e = errors.New("限速须为 0–102400 KiB/s，0 表示取消独立限速")
				break
			}
			d.ManualRateKBps = v.RateKBps
		case "allow":
			e = g.grant(d)
		case "block":
			d.Banned = true
			d.Until = 0
		case "revoke":
			d.Until = 0
			d.Trusted = false
		case "trust":
			d.Trusted = true
			d.Banned = false
		case "untrust":
			d.Trusted = false
		case "release":
			d.Banned = false
			d.LimitReason = ""
			d.DailyBase = d.DayUp + d.DayDown
			d.TotalBase = d.Up + d.Down
			d.SessionBase = d.SessionBytes
			d.PeriodBase = d.PeriodBytes
			d.Until = time.Now().Add(time.Duration(g.db.Config.SessionHours) * time.Hour).Unix()
		case "reset":
			d.Up = 0
			d.Down = 0
			d.DayUp = 0
			d.DayDown = 0
			d.SessionBytes = 0
			d.MonthBytes = 0
			d.PeriodBytes = 0
			d.TotalBase = 0
			d.DailyBase = 0
			d.SessionBase = 0
			d.PeriodBase = 0
			d.LimitReason = ""
		case "delete":
			// 删除设备记录：下次扫描到该 MAC 时 device() 会重新创建一个全新设备
			// （无信任、无封禁、无授权、无流量统计），等同新访客接入。
			// 管理员权限（Config.AdminMACs）不在此清除，需单独走 admin-remove。
			delete(g.db.Devices, v.MAC)
		case "edit":
			if len(v.Note) > 100 {
				e = errors.New("备注过长")
				break
			}
			d.Note = v.Note
			if v.Quota != nil {
				if e = validateQuota(*v.Quota); e == nil {
					d.Quota = *v.Quota
				}
			}
		default:
			e = errors.New("未知设备操作")
		}
		if e == nil {
			g.event("device", v.MAC+" "+v.Action)
			e = g.reconcile()
		}
	case "/admin-api/account":
		var v struct {
			Name, Password, Action string
			Expires                int64
			Quota                  *Quota
		}
		if e = body(w, r, &v); e != nil {
			break
		}
		if v.Name == "" || len(v.Name) > 64 {
			e = errors.New("账号名称无效")
			break
		}
		if v.Action == "delete" {
			delete(g.db.Accounts, v.Name)
		} else {
			if !strong(v.Password) {
				e = errors.New("密码须至少 8 位且包含字母和数字")
				break
			}
			if v.Quota != nil {
				if e = validateQuota(*v.Quota); e != nil {
					break
				}
			}
			g.db.Accounts[v.Name] = Account{v.Name, passwordHash(v.Password), v.Expires, v.Quota}
		}
		g.event("account", "上网账号已更新")
	case "/admin-api/decision":
		var v struct{ ID, Action, Reason string }
		if e = body(w, r, &v); e == nil {
			e = g.decide(v.ID, v.Action, v.Reason)
		}
	case "/admin-api/password":
		var v struct{ Username, Old, New string }
		if e = body(w, r, &v); e != nil {
			break
		}
		if !passwordOK(g.db.Config.AdminHash, v.Old) || !strong(v.New) || v.Username == "" || len(v.Username) > 64 {
			e = errors.New("原密码错误，或新账号/密码不符合要求")
			break
		}
		g.db.Config.AdminName = v.Username
		g.db.Config.AdminHash = passwordHash(v.New)
		g.sessions = map[string]Session{}
		g.event("admin", "管理员凭据已更新，所有会话已注销")
	case "/admin-api/remote-approval-test":
		g.sendRemoteApprovalTest(w)
		return
	case "/admin-api/push-test":
		p := g.db.Config.Push
		a := Request{ID: "test", MAC: "02:00:00:00:00:01", Name: "推送测试", Reason: "这是一条测试消息，审批按钮无效。", Created: time.Now().Unix(), Expires: time.Now().Add(5 * time.Minute).Unix()}
		go g.pushRequest(p, a)
	case "/admin-api/prune":
		for k, a := range g.db.Requests {
			if a.Status != "pending" || a.Expires < time.Now().Unix() {
				delete(g.db.Requests, k)
			}
		}
		g.event("admin", "已清理结束/过期的审批记录")
	default:
		e = errors.New("接口不存在")
	}
	if e == nil {
		e = g.save()
	}
	if e != nil {
		fail(w, e)
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}
func validateQuota(q Quota) error {
	if q.PeriodDays < 1 || q.PeriodDays > 365 || q.RateKBps < 1 || q.RateKBps > 102400 || q.SlowPercent < 0 || q.SlowPercent > 100 || (q.Action != "block" && q.Action != "slow") {
		return errors.New("配额参数无效：周期 1–365 天，速率 1–102400 KiB/s，阈值 0–100%")
	}
	return nil
}
func validateConfig(c Config) error {
	if err := validateRemoteApproval(c.RemoteApproval); err != nil {
		return err
	}
	for _, mac := range adminDeviceMACs(c) {
		if !validMAC(mac) {
			return errors.New("管理员设备 MAC 地址无效")
		}
	}
	if c.PortalHost != "" && (!domainRE.MatchString(c.PortalHost) || strings.HasPrefix(c.PortalHost, "*.") || strings.Contains(c.PortalHost, "..") || ipv4(c.PortalHost)) {
		return errors.New("认证页域名须为精确域名，不含协议、端口或通配符")
	}
	if !ifaceRE.MatchString(c.LAN) || !ipv4(c.Gateway) || (c.AdminHash != "" && len(adminDeviceMACs(c)) == 0) {
		return errors.New("请填写有效热点接口和 IPv4 网关")
	}
	if c.SessionHours < 1 || c.SessionHours > 720 {
		return errors.New("会话时长须在 1–720 小时内")
	}
	if len(c.Methods) == 0 {
		return errors.New("至少选择一种认证方式")
	}
	for _, m := range c.Methods {
		if !contains([]string{"account", "captcha", "approval", "thirdparty"}, m) {
			return errors.New("认证方式无效")
		}
	}
	if e := validateQuota(c.DefaultQuota); e != nil {
		return e
	}
	if len(c.Black)+len(c.White) > 200 {
		return errors.New("黑白名单合计最多 200 项")
	}
	for _, s := range append(append([]string{}, c.Black...), c.White...) {
		if !ipv4(s) && (!domainRE.MatchString(s) || strings.Contains(s, "..")) {
			return errors.New("名单仅支持 IPv4、精确域名或 *.域名: " + s)
		}
	}
	for _, logo := range []string{c.SiteLogo, c.PortalLogo} {
		if _, _, err := decodeLogo(logo); err != nil {
			return err
		}
	}
	if len(c.Title) > 120 || len(c.Subtitle) > 500 || len(c.Contact) > 500 || len(c.Warning) > 1000 || len(c.BlockWarning) > 1000 || !regexp.MustCompile(`^#[0-9a-fA-F]{6}$`).MatchString(c.Accent) {
		return errors.New("页面文案或主题色无效")
	}
	if contains(c.Methods, "thirdparty") {
		if !contains([]string{"hcaptcha", "turnstile", "custom"}, c.CaptchaProvider) || c.CaptchaSite == "" || c.CaptchaSecret == "" {
			return errors.New("请完整配置第三方验证服务")
		}
		if c.CaptchaProvider == "custom" && (!httpsURL(c.CaptchaVerify) || !httpsURL(c.CaptchaScript)) {
			return errors.New("自定义验证码需要 HTTPS 验证 API 和脚本地址")
		}
	}
	p := c.Push
	if p.Kind != "" && !contains([]string{"wecom", "feishu", "webhook", "smtp"}, p.Kind) {
		return errors.New("推送类型无效")
	}
	if p.Kind != "" && p.Kind != "smtp" && !httpsURL(p.URL) {
		return errors.New("推送 Webhook 必须使用 HTTPS")
	}
	if p.PublicBase != "" {
		u, e := url.Parse(p.PublicBase)
		if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && net.ParseIP(u.Hostname()) != nil && net.ParseIP(u.Hostname()).IsPrivate())) {
			return errors.New("审批地址须为 HTTPS 公网/VPN 地址，或 HTTP 私网 IPv4 地址")
		}
	}
	if p.Kind == "smtp" {
		if p.SMTPHost == "" || strings.ContainsAny(p.SMTPHost, "\r\n /:") || p.SMTPPort < 1 || p.SMTPPort > 65535 || strings.ContainsAny(p.SMTPFrom+p.SMTPTo, "\r\n") || !strings.Contains(p.SMTPFrom, "@") || !strings.Contains(p.SMTPTo, "@") {
			return errors.New("SMTP 主机、端口、发件人或收件人无效")
		}
	}
	return nil
}
func httpsURL(s string) bool {
	u, e := url.Parse(s)
	return e == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.Fragment == ""
}
func (g *Gate) verifyCaptcha(c Config, token, ip string) bool {
	if len(token) < 1 || len(token) > 4096 {
		return false
	}
	endpoint := c.CaptchaVerify
	if c.CaptchaProvider == "hcaptcha" {
		endpoint = "https://api.hcaptcha.com/siteverify"
	}
	if c.CaptchaProvider == "turnstile" {
		endpoint = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	}
	values := url.Values{"secret": {c.CaptchaSecret}, "response": {token}, "sitekey": {c.CaptchaSite}}
	if addr := net.ParseIP(ip); addr != nil && !addr.IsPrivate() && !addr.IsLoopback() {
		values.Set("remoteip", ip)
	}
	r, e := g.httpClient.PostForm(endpoint, values)
	if e != nil {
		return false
	}
	defer r.Body.Close()
	var v struct {
		Success  bool
		Hostname string
	}
	if r.StatusCode != 200 || json.NewDecoder(io.LimitReader(r.Body, 65536)).Decode(&v) != nil {
		return false
	}
	return v.Success && (c.PortalHost == "" || c.CaptchaProvider == "custom" || strings.EqualFold(v.Hostname, c.PortalHost))
}
func (g *Gate) decide(id, action, reason string) error {
	a := g.db.Requests[id]
	if a == nil || a.Status != "pending" || a.Expires < time.Now().Unix() {
		return errors.New("审批不存在、已处理或已过期")
	}
	if action != "allow" && action != "deny" || len(reason) > 1000 {
		return errors.New("审批参数无效")
	}
	if action == "allow" {
		if e := g.grant(g.device(a.MAC)); e != nil {
			return e
		}
	}
	a.Status = action
	a.DecisionReason = reason
	a.Decided = time.Now().Unix()
	a.Nonce = ""
	g.event("approval", a.MAC+" 审批结果: "+action)
	return nil
}
func (g *Gate) approvalToken(a Request, action string) string {
	payload := a.ID + "|" + action + "|" + strconv.FormatInt(a.Expires, 10) + "|" + a.Nonce
	h := hmac.New(sha256.New, []byte(g.db.Config.SigningKey))
	h.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + hex.EncodeToString(h.Sum(nil))
}
func (g *Gate) parseApproval(token string) (string, string, error) {
	a := strings.Split(token, ".")
	if len(a) != 2 {
		return "", "", errors.New("审批链接无效")
	}
	b, e := base64.RawURLEncoding.DecodeString(a[0])
	if e != nil {
		return "", "", errors.New("审批链接无效")
	}
	h := hmac.New(sha256.New, []byte(g.db.Config.SigningKey))
	h.Write(b)
	sig, e := hex.DecodeString(a[1])
	if e != nil || !hmac.Equal(h.Sum(nil), sig) {
		return "", "", errors.New("审批签名无效")
	}
	p := strings.Split(string(b), "|")
	if len(p) != 4 {
		return "", "", errors.New("审批链接无效")
	}
	req := g.db.Requests[p[0]]
	if req == nil || req.Status != "pending" || p[3] != req.Nonce || p[2] != strconv.FormatInt(req.Expires, 10) || req.Expires < time.Now().Unix() || !contains([]string{"allow", "deny"}, p[1]) {
		return "", "", errors.New("审批已过期或已处理")
	}
	return p[0], p[1], nil
}
func (g *Gate) approvalPage(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.approvalPageLocked(w, r)
}
func (g *Gate) approvalPageLocked(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	token := r.URL.Query().Get("token")
	if r.Method == "POST" {
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "请求无效", 400)
			return
		}
		token = r.PostForm.Get("token")
	}
	id, action, e := g.parseApproval(token)
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	if r.Method == "POST" {
		e = g.decide(id, action, r.Form.Get("reason"))
		if e == nil {
			e = g.save()
		}
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		fmt.Fprint(w, "<!doctype html><meta charset=utf-8><meta name=viewport content='width=device-width'><h2>审批已完成</h2><p>您可以关闭此页面。</p>")
		return
	}
	if r.Method != "GET" {
		w.WriteHeader(405)
		return
	}
	a := g.db.Requests[id]
	label := "同意上网"
	if action == "deny" {
		label = "拒绝上网"
	}
	fmt.Fprintf(w, `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/style.css"><body><main class="auth-shell"><section class="card auth-card"><div class="badge">UFI-GATE · 审批确认</div><h1>%s</h1><p>%s · %s</p><p>%s</p><form method="post" action="/approve"><input type="hidden" name="token" value="%s"><label>处理备注<input name="reason" maxlength="1000"></label><button class="primary">确认%s</button></form><p class="muted">链接在 30 分钟后失效。此操作只能执行一次。</p></section></main></body></html>`, htmlEscape(label), htmlEscape(a.Name), htmlEscape(a.MAC), htmlEscape(a.Reason), htmlEscape(token), htmlEscape(label))
}
func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&#39;").Replace(s)
}
func (g *Gate) sendRequest(p Push, a Request) error {
	if p.Kind == "" {
		return nil
	}
	g.mu.Lock()
	base := strings.TrimRight(p.PublicBase, "/")
	if g.db.Config.RemoteApproval.Enabled {
		base = strings.TrimRight(g.db.Config.RemoteApproval.PublicURL, "/")
	}
	if base == "" {
		base = "http://" + g.db.Config.Gateway + ":8081"
	}
	allow := base + "/approve?token=" + url.QueryEscape(g.approvalToken(a, "allow"))
	deny := base + "/approve?token=" + url.QueryEscape(g.approvalToken(a, "deny"))
	g.mu.Unlock()
	safeVisitor := func(v string) string {
		return strings.NewReplacer("[", "［", "]", "］", "(", "（", ")", "）", "<", "＜", ">", "＞", "`", "ˋ").Replace(v)
	}
	text := fmt.Sprintf("设备申请上网\n昵称：%s\nMAC：%s\n理由：%s\n申请时间：%s\n有效期：30 分钟", safeVisitor(a.Name), a.MAC, safeVisitor(a.Reason), time.Unix(a.Created, 0).In(zone).Format("2006-01-02 15:04:05"))
	return g.sendPush(p, "设备申请上网", text, allow, deny, &a)
}
func (g *Gate) sendPush(p Push, title, text, allow, deny string, a *Request) error {
	var e error
	if p.Kind == "smtp" {
		e = g.sendMail(p, text, allow, deny)
	} else {
		var payload any
		switch p.Kind {
		case "wecom":
			payload = map[string]any{"msgtype": "markdown", "markdown": map[string]string{"content": text + "\n[同意上网](" + allow + ")  [拒绝上网](" + deny + ")"}}
		case "feishu":
			payload = map[string]any{"msg_type": "interactive", "card": map[string]any{"header": map[string]any{"title": map[string]string{"tag": "plain_text", "content": title}}, "elements": []any{map[string]any{"tag": "div", "text": map[string]string{"tag": "plain_text", "content": text}}, map[string]any{"tag": "action", "actions": []any{map[string]any{"tag": "button", "text": map[string]string{"tag": "plain_text", "content": "同意上网"}, "type": "primary", "url": allow}, map[string]any{"tag": "button", "text": map[string]string{"tag": "plain_text", "content": "拒绝上网"}, "url": deny}}}}}}
			if p.Secret != "" {
				timestamp := strconv.FormatInt(time.Now().Unix(), 10)
				h := hmac.New(sha256.New, []byte(timestamp+"\n"+p.Secret))
				m := payload.(map[string]any)
				m["timestamp"] = timestamp
				m["sign"] = base64.StdEncoding.EncodeToString(h.Sum(nil))
			}
		default:
			payload = map[string]any{"title": title, "content": text, "request": a, "allow_url": allow, "deny_url": deny}
		}
		if allow == "" {
			switch p.Kind {
			case "wecom":
				payload = map[string]any{"msgtype": "text", "text": map[string]string{"content": title + "\n" + text}}
			case "feishu":
				m := payload.(map[string]any)
				m["msg_type"] = "text"
				m["content"] = map[string]string{"text": title + "\n" + text}
				delete(m, "card")
			}
		}
		b, _ := json.Marshal(payload)
		req, requestErr := http.NewRequestWithContext(g.ctx, "POST", p.URL, strings.NewReader(string(b)))
		if requestErr != nil {
			return requestErr
		}
		req.Header.Set("Content-Type", "application/json")
		if p.Kind == "webhook" && p.Secret != "" {
			req.Header.Set("Authorization", "Bearer "+p.Secret)
		}
		res, er := g.httpClient.Do(req)
		e = er
		if er == nil {
			raw, _ := io.ReadAll(io.LimitReader(res.Body, 65536))
			res.Body.Close()
			if res.StatusCode < 200 || res.StatusCode >= 300 {
				e = errors.New("推送 HTTP 状态 " + strconv.Itoa(res.StatusCode))
			} else {
				var v map[string]any
				json.Unmarshal(raw, &v)
				for _, k := range []string{"errcode", "code", "StatusCode"} {
					if c, ok := v[k].(float64); ok && c != 0 {
						e = errors.New("服务返回错误代码 " + strconv.Itoa(int(c)))
					}
				}
			}
		}
	}
	return e
}
func (g *Gate) pushRequest(p Push, a Request) {
	e := g.sendRequest(p, a)
	g.mu.Lock()
	defer g.mu.Unlock()
	if e != nil {
		g.event("push", "推送失败: "+safeError(e.Error(), p))
	} else {
		g.event("push", p.Kind+" 推送已发送")
	}
	_ = g.save()
}
func safeError(s string, p Push) string {
	for _, v := range []string{p.URL, p.Secret, p.SMTPPassword} {
		if v != "" {
			s = strings.ReplaceAll(s, v, "[隐藏]")
		}
	}
	if len(s) > 500 {
		s = s[:500]
	}
	return s
}
func curlQuote(s string) string {
	return "\"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\r", "", "\n", "").Replace(s) + "\""
}
func (g *Gate) sendMail(p Push, text, allow, deny string) error {
	// UFI ships curl with TLS and SMTP support; keep secrets in a 0600 file, never in argv.
	curl := "/data/data/com.minikano.f50_sms/files/curl"
	if demo {
		var e error
		curl, e = exec.LookPath("curl")
		if e != nil {
			return e
		}
	}
	temp, e := os.MkdirTemp(dir, "mail-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(temp)
	msg := "From: " + p.SMTPFrom + "\r\nTo: " + p.SMTPTo + "\r\nSubject: =?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte("UFI-Gate 上网审批请求")) + "?=\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n<div style=\"font-family:sans-serif\"><h2>设备申请上网</h2><pre>" + htmlEscape(text) + "</pre><p><a href=\"" + htmlEscape(allow) + "\">同意上网</a> · <a href=\"" + htmlEscape(deny) + "\">拒绝上网</a></p></div>"
	if allow == "" {
		msg = "From: " + p.SMTPFrom + "\r\nTo: " + p.SMTPTo + "\r\nSubject: UFI-Gate notification\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + text
	}
	if e = os.WriteFile(filepath.Join(temp, "body"), []byte(msg), 0600); e != nil {
		return e
	}
	scheme := "smtp"
	if p.SMTPPort == 465 {
		scheme = "smtps"
	}
	cfg := "url = " + curlQuote(scheme+"://"+p.SMTPHost+":"+strconv.Itoa(p.SMTPPort)) + "\nssl-reqd\nuser = " + curlQuote(p.SMTPUser+":"+p.SMTPPassword) + "\nmail-from = " + curlQuote(p.SMTPFrom) + "\nmail-rcpt = " + curlQuote(p.SMTPTo) + "\nupload-file = " + curlQuote(filepath.Join(temp, "body")) + "\nmax-time = 20\nsilent\nshow-error\n"
	if !demo {
		cfg += "capath = \"/system/etc/security/cacerts\"\n"
	}
	if e = os.WriteFile(filepath.Join(temp, "curl.conf"), []byte(cfg), 0600); e != nil {
		return e
	}
	_, e = realRun(curl, "--config", filepath.Join(temp, "curl.conf"))
	return e
}
func (g *Gate) diagnostics() map[string]any {
	checks := map[string]any{"root": os.Geteuid() == 0, "demo": demo, "lan": g.db.Config.LAN, "gateway": g.db.Config.Gateway, "ipv6": "启用时阻断热点 IPv6，防止绕过", "hardware": "Android 标准 offload 自动关闭；厂商专有加速须现场检测", "domainFiltering": "基于 DNS 与已解析 IPv4；HTTPS 无法无证书警告重定向，加密 DNS/VPN/CDN 存在限制"}
	for _, bin := range []string{"iptables", "ip6tables", "ip", "settings"} {
		_, e := exec.LookPath(bin)
		checks[bin] = e == nil
	}
	checks["preflight"] = "未检测"
	if g.db.Config.LAN != "" {
		if e := g.networkPreflight(); e != nil {
			checks["preflight"] = e.Error()
		} else {
			checks["preflight"] = "通过"
		}
	}
	out, _ := realRun("ip", "-4", "addr", "show")
	checks["interfaces"] = out
	return checks
}
func (g *Gate) restoreOffload() {
	if demo {
		return
	}
	if g.db.Config.OffloadOriginal != "" {
		if g.db.Config.OffloadOriginal == "null" {
			_, err := realRun("settings", "delete", "global", "tether_offload_disabled")
			if err != nil {
				g.event("error", "标准 offload 恢复失败: "+err.Error())
				return
			}
		} else {
			_, err := realRun("settings", "put", "global", "tether_offload_disabled", g.db.Config.OffloadOriginal)
			if err != nil {
				g.event("error", "标准 offload 恢复失败: "+err.Error())
				return
			}
		}
		g.db.Config.OffloadOriginal = ""
	}
}
func (g *Gate) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if len(r.Host) > 256 || r.ContentLength > 128*1024 {
			http.Error(w, "请求过大", 413)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && networkPending() {
			reply(w, 503, map[string]string{"error": "网络配置维护中，请等待修复完成后重试"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
func staticFile(w http.ResponseWriter, r *http.Request, name string) {
	b, e := web.ReadFile("web/" + name)
	if e != nil {
		http.Error(w, "资源不存在", 404)
		return
	}
	if strings.HasSuffix(name, ".css") {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	} else if strings.HasSuffix(name, ".js") {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	w.Write(b)
}
func (g *Gate) portalHandler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("/site-logo", g.siteLogo)
	m.HandleFunc("/default-logo.png", defaultLogo)
	m.HandleFunc("/api/admin-access/", g.portalAdminAPI)
	m.HandleFunc("/api/", g.portalAPI)
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/style.css":
			staticFile(w, r, "style.css")
		case "/admin-access":
			staticFile(w, r, "admin-access.html")
		case "/admin-access.js":
			staticFile(w, r, "admin-access.js")
		case "/recovery.js":
			staticFile(w, r, "recovery.js")
		case "/portal.js":
			staticFile(w, r, "portal.js")
		case "/":
			staticFile(w, r, "portal.html")
		default:
			if r.Method != "GET" {
				http.Error(w, "方法不支持", 405)
				return
			}
			g.mu.Lock()
			gateway := g.db.Config.Gateway
			if g.db.Config.PortalHost != "" {
				gateway = g.db.Config.PortalHost
			}
			g.mu.Unlock()
			http.Redirect(w, r, "http://"+gateway+":8090/?reason=network", http.StatusFound)
		}
	})
	return g.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		g.mu.Lock()
		gateway := g.db.Config.Gateway
		portalHost := g.db.Config.PortalHost
		black := matches(host, g.db.Config.Black) || contains(g.destinationIPs(false), host)
		g.mu.Unlock()
		if host != gateway && host != portalHost && !(demo && (host == "127.0.0.1" || host == "localhost")) {
			reason := "network"
			if black {
				reason = "black"
			}
			if portalHost != "" {
				gateway = portalHost
			}
			http.Redirect(w, r, "http://"+gateway+":8090/?reason="+reason, http.StatusFound)
			return
		}
		m.ServeHTTP(w, r)
	}))
}
func (g *Gate) adminHandler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("/site-logo", g.siteLogo)
	m.HandleFunc("/default-logo.png", defaultLogo)
	m.HandleFunc("/api/admin-access/", g.portalAdminAPI)
	m.HandleFunc("/admin-api/", g.adminAPI)
	m.HandleFunc("/approve", g.approvalPage)
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/style.css" {
			staticFile(w, r, "style.css")
		} else if r.URL.Path == "/recovery.js" {
			staticFile(w, r, "recovery.js")
		} else if r.URL.Path == "/admin.js" {
			staticFile(w, r, "admin.js")
		} else if r.URL.Path == "/admin-ui.js" {
			staticFile(w, r, "admin-ui.js")
		} else if r.URL.Path == "/admin-updates.js" {
			staticFile(w, r, "admin-updates.js")
		} else if r.URL.Path == "/marked.min.js" {
			staticFile(w, r, "marked.min.js")
		} else if r.URL.Path == "/purify.min.js" {
			staticFile(w, r, "purify.min.js")
		} else if r.URL.Path == "/" {
			staticFile(w, r, "admin.html")
		} else {
			http.NotFound(w, r)
		}
	})
	return g.protectAdmin(g.middleware(m))
}

var realRun = executeCommand

func executeCommand(bin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	b, e := cmd.CombinedOutput()
	if e != nil {
		return string(b), fmt.Errorf("%s 失败: %s", filepath.Base(bin), strings.TrimSpace(string(b)))
	}
	return string(b), nil
}

func main() {
	flag.StringVar(&dir, "dir", "/data/ufi-gate", "状态目录")
	flag.BoolVar(&demo, "demo", false, "仅用于本机演示，不操作防火墙")
	recoverFlag := flag.Bool("recover", false, "清除本插件防火墙规则")
	cleanupOnly := flag.Bool("cleanup-rules", false, "仅清除规则，用于 supervisor 崩溃恢复")
	setup := flag.Bool("setup-token", false, "重新生成初始化 token（仅未初始化）")
	resetAdmin := flag.Bool("reset-admin", false, "通过本地控制通道重置管理员，保留联网设备")
	if updateCLIFlags() {
		return
	}
	os.MkdirAll(dir, 0700)
	if *resetAdmin {
		if e := requestAdminReset(); e != nil {
			log.Fatal(e)
		}
		return
	}
	if *recoverFlag || *cleanupOnly {
		f := Firewall{runner: realRun}
		if e := f.cleanup(); e != nil {
			log.Fatal(e)
		}
		if *recoverFlag {
			g, e := loadGate()
			if e != nil {
				log.Fatal(e)
			}
			g.db.Config.Enabled = false
			g.restoreOffload()
			g.event("recovery", "已紧急恢复网络")
			if e = g.save(); e != nil {
				log.Fatal(e)
			}
		}
		return
	}
	lock, e := os.OpenFile(filepath.Join(dir, "service.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		log.Fatal(e)
	}
	defer lock.Close()
	if e = lockFile(lock); e != nil {
		log.Fatal("服务已运行")
	}
	// Read state only after taking the service lock; maintenance may have replaced it.
	if networkPending() && !*networkWorker {
		log.Fatal("存在网络配置修复事务，请通过插件启动服务恢复")
	}
	g, e := loadGate()
	if e != nil {
		log.Fatal(e)
	}
	if *setup {
		if g.db.Config.AdminHash != "" {
			log.Fatal("管理员已初始化；如需重置请备份 state.json 后停服操作")
		}
		g.bootToken = random(24)
		atomicWrite(filepath.Join(dir, "setup.token"), []byte(g.bootToken))
		fmt.Println(g.bootToken)
		return
	}
	if demo {
		g.db.Config.LAN = "lo"
		g.db.Config.Gateway = "127.0.0.1"
		g.db.Config.Verified = true
	}
	bind := "0.0.0.0"
	if demo {
		bind = "127.0.0.1"
	}
	// Bind every service before inserting any rule. A busy port must never leave the hotspot blocked.
	portal, e := net.Listen("tcp4", bind+":8090")
	if e != nil {
		log.Fatal(e)
	}
	defer portal.Close()
	admin, e := net.Listen("tcp4", bind+":8081")
	if e != nil {
		log.Fatal(e)
	}
	defer admin.Close()
	udp, e := net.ListenPacket("udp4", bind+":5354")
	if e != nil {
		log.Fatal(e)
	}
	defer udp.Close()
	tcp, e := net.Listen("tcp4", bind+":5354")
	if e != nil {
		log.Fatal(e)
	}
	defer tcp.Close()
	pserver := &http.Server{Handler: g.portalHandler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
	aserver := &http.Server{Handler: g.adminHandler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
	control, err := g.startControl()
	if err != nil {
		log.Fatal(err)
	}
	defer control.Close()
	g.mu.Lock()
	remoteListener, remoteErr := g.prepareRemoteApproval(g.db.Config.RemoteApproval)
	if remoteErr != nil {
		g.event("error", "远程审批监听失败，请检查端口后重新保存设置: "+remoteErr.Error())
	} else {
		g.commitRemoteApproval(remoteListener)
	}
	g.mu.Unlock()
	go pserver.Serve(portal)
	go aserver.Serve(&adminListener{Listener: admin, gate: g})
	go g.serveDNS(udp, tcp)
	g.mu.Lock()
	if g.db.Config.Enabled {
		if e = g.networkPreflight(); e == nil {
			e = g.reconcile()
		}
		if e != nil {
			g.fw.cleanup()
			g.restoreOffload()
			g.db.Config.Enabled = false
			g.event("error", "开机恢复失败: "+e.Error())
		}
	}
	if !demo {
		v := validateNetwork(inspectNetwork(), g.db.Config.LAN, g.db.Config.Gateway)
		if v.Status != "valid" {
			g.event("error", "热点配置诊断："+v.Message+"；"+networkRepairHint)
		}
	}
	g.event("service", "服务启动 "+currentVersion())
	if e = g.save(); e != nil {
		log.Printf("启动状态保存失败: %v", e)
	} else {
		g.ready = true
	}
	g.mu.Unlock()
	go g.loop()
	go g.notificationLoop()
	if !demo {
		go g.watchHost()
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	select {
	case <-signals:
	case <-g.ctx.Done():
	}
	g.cancel()
	g.mu.Lock()
	if g.remoteServer != nil {
		_ = g.remoteServer.Close()
	}
	g.sample()
	g.fw.cleanup()
	g.restoreOffload()
	g.save()
	g.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pserver.Shutdown(ctx)
	aserver.Shutdown(ctx)
}
