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
	if e := f.ip("nat", "-A", "UFIG_TEST", "-p", "tcp", "--dport", "80", "-j", "REDIRECT", "--to-ports", "8090"); e != nil {
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
