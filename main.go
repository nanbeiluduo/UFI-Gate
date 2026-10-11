// UFI-Gate: standalone IPv4 captive portal for rooted UFI-TOOLS devices.
// No third-party runtime dependencies. See README for protocol/firmware limits.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
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

const version = "1.0.2"

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
	MAC, IP, Name, Note string
	FirstSeen, LastSeen, Until int64
	Trusted, Banned bool
	LimitReason string
	Quota Quota
	Up, Down, DayUp, DayDown, SessionBytes, PeriodBytes, MonthBytes uint64
	Day, Month, PeriodKey string
	TotalBase, DailyBase, SessionBase, PeriodBase uint64
	RateUp, RateDown float64
}
type Account struct {
	Name, Hash string
	Expires int64
	Quota *Quota
}
type Push struct {
	Kind, URL, Secret, PublicBase, SMTPHost, SMTPUser, SMTPPassword, SMTPFrom, SMTPTo string
	SMTPPort int
}
type Config struct {
	PortalHost string
	Enabled bool
	LAN, Gateway, AdminMAC string
	SessionHours int
	Methods []string
	DefaultQuota Quota
	Black, White []string
	Title, Subtitle, Contact, Warning, BlockWarning, Accent string
	CaptchaProvider, CaptchaSite, CaptchaSecret, CaptchaVerify, CaptchaScript string
	Push Push
	AdminName, AdminHash, SigningKey string
	OffloadOriginal string
	Verified bool
}
type Request struct {
	ID, MAC, Name, Reason, Status, DecisionReason, Nonce string
	Created, Expires, Decided int64
}
type DB struct {
	Config Config
	Devices map[string]*Device
	Accounts map[string]Account
	Requests map[string]*Request
	Logs []Event
}
type Event struct {
	Time int64
	Kind, Message string
}
type Session struct {
	Last int64
	IP string
}
type Attempt struct {
	Failures int
	Until int64
}
type Challenge struct {
	MAC, Answer string
	Expires int64
}
type Gate struct {
	mu sync.Mutex
	db DB
	sessions map[string]Session
	attempts map[string]Attempt
	challenges map[string]Challenge
	counters map[string]uint64
	resolvedWhite, resolvedBlack map[string]int64
	bootToken string
	bootExpiry int64
	fw *Firewall
	started time.Time
	rates []RatePoint
	lastSample time.Time
	ctx context.Context
	cancel context.CancelFunc
	httpClient *http.Client
	dnsSlots chan struct{}
	countersDay string
	blockedPackets, blackPackets uint64
	blockCounters map[string]uint64
}
type RatePoint struct {
	Time int64
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
	g.db.Logs = append(g.db.Logs, Event{time.Now().Unix(), k, m})
	if len(g.db.Logs) > 500 {
		g.db.Logs = g.db.Logs[len(g.db.Logs)-500:]
	}
}
