package main

// Root maintenance diagnostics. No state document or identity secrets leave this API.
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const networkRepairHint = "当前热点接口配置可能不正确，请在 UFI-TOOLS 插件菜单中打开“修改热点接口与网关”检查。"

type NetworkConfig struct {
	LAN     string
	Gateway string
	Enabled bool
}
type NetworkInterface struct {
	Name         string
	Up           bool
	Loopback     bool
	PointToPoint bool
	Addresses    []string
	Complete     bool
	Uplink       bool
}
type NetworkInventory struct {
	Interfaces []NetworkInterface
	Complete   bool
	RouteKnown bool
	Warnings   []string
}
type NetworkValidation struct {
	Status  string
	Code    string
	Message string
}
type NetworkCandidate struct {
	LAN     string
	Gateway string
	Reason  string
}
type NetworkReport struct {
	Protocol        int
	Running         *NetworkConfig
	Saved           *NetworkConfig
	Bootstrap       *NetworkConfig
	Interfaces      []NetworkInterface
	Candidates      []NetworkCandidate
	Recommended     *NetworkCandidate
	Validation      NetworkValidation
	SavedValidation *NetworkValidation
	Warnings        []string
}

var networkInterfaces = net.Interfaces
var networkAddrs = func(i net.Interface) ([]net.Addr, error) { return i.Addrs() }
var inspectNetwork = systemNetwork

func (g *Gate) peerNetworkError(commandFailed bool) error {
	v := validateNetwork(inspectNetwork(), g.db.Config.LAN, g.db.Config.Gateway)
	if v.Status == "invalid" {
		return errors.New(networkRepairHint)
	}
	if v.Status == "unknown" || commandFailed {
		return errors.New("无法确认热点设备：系统查询失败或权限不足，请在插件菜单检测环境。")
	}
	return errors.New("热点接口配置有效，但未找到当前设备的 MAC。请确认连接此热点后重试，仍失败请在插件菜单检测环境。")
}

// netlink through the Go standard library first; Android permission/SELinux failures
// fall back to ordinary `ip -4 addr show`, not JSON, jq, Python or a full iproute2.
func systemNetwork() NetworkInventory {
	n := NetworkInventory{}
	list, err := networkInterfaces()
	if err == nil {
		n.Complete = true
		for _, i := range list {
			v := NetworkInterface{Name: i.Name, Up: i.Flags&net.FlagUp != 0, Loopback: i.Flags&net.FlagLoopback != 0, PointToPoint: i.Flags&net.FlagPointToPoint != 0, Complete: true, Addresses: []string{}}
			addresses, e := networkAddrs(i)
			if e != nil {
				v.Complete = false
				n.Complete = false
				n.Warnings = append(n.Warnings, "无法读取接口地址："+i.Name)
			}
			for _, a := range addresses {
				ip, _, e := net.ParseCIDR(a.String())
				if e == nil && ip.To4() != nil {
					v.Addresses = append(v.Addresses, a.String())
				}
			}
			n.Interfaces = append(n.Interfaces, v)
		}
	} else {
		n.Warnings = append(n.Warnings, "系统接口查询失败，尝试 ip 文本查询："+err.Error())
	}
	if !n.Complete {
		out, e := realRun("ip", "-4", "addr", "show")
		if e == nil {
			fallback := parseNetworkAddresses(out)
			if fallback.Complete {
				n.Interfaces = fallback.Interfaces
				n.Complete = true
			} else {
				n.Warnings = append(n.Warnings, "ip 地址输出不完整，不能确认接口是否存在")
			}
		} else {
			n.Warnings = append(n.Warnings, "ip 地址查询失败或权限不足："+e.Error())
		}
	}
	routes, e := realRun("ip", "-4", "route", "show", "table", "all")
	if e == nil {
		n.RouteKnown = true
		for _, line := range strings.Split(routes, "\n") {
			f := strings.Fields(line)
			if len(f) > 0 && f[0] == "default" {
				for j := 0; j+1 < len(f); j++ {
					if f[j] == "dev" {
						for k := range n.Interfaces {
							if n.Interfaces[k].Name == f[j+1] {
								n.Interfaces[k].Uplink = true
							}
						}
					}
				}
			}
		}
	} else {
		n.Warnings = append(n.Warnings, "无法查询全部 IPv4 路由；不会自动推荐无法排除的上联接口")
	}
	sort.Slice(n.Interfaces, func(i, j int) bool { return n.Interfaces[i].Name < n.Interfaces[j].Name })
	return n
}
func parseNetworkAddresses(out string) NetworkInventory {
	n := NetworkInventory{}
	current := -1
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		if strings.HasSuffix(f[0], ":") && regexpPID(strings.TrimSuffix(f[0], ":")) {
			name := strings.Split(strings.TrimSuffix(f[1], ":"), "@")[0]
			if !ifaceRE.MatchString(name) {
				continue
			}
			flags := ""
			if a := strings.Index(line, "<"); a >= 0 {
				if b := strings.Index(line[a:], ">"); b >= 0 {
					flags = line[a+1 : a+b]
				}
			}
			n.Interfaces = append(n.Interfaces, NetworkInterface{Name: name, Up: contains(strings.Split(flags, ","), "UP"), Loopback: contains(strings.Split(flags, ","), "LOOPBACK"), PointToPoint: contains(strings.Split(flags, ","), "POINTOPOINT"), Complete: true, Addresses: []string{}})
			current = len(n.Interfaces) - 1
		}
		if current >= 0 {
			for j := 0; j+1 < len(f); j++ {
				if f[j] == "inet" {
					if ip, _, e := net.ParseCIDR(f[j+1]); e == nil && ip.To4() != nil {
						n.Interfaces[current].Addresses = append(n.Interfaces[current].Addresses, f[j+1])
					}
				}
			}
		}
	}
	n.Complete = len(n.Interfaces) > 0
	return n
}
func validateNetwork(n NetworkInventory, lan, gateway string) NetworkValidation {
	bad := func(code, msg string) NetworkValidation { return NetworkValidation{"invalid", code, msg} }
	if !ifaceRE.MatchString(lan) {
		return bad("interface_format", "热点接口名称格式无效")
	}
	ip := net.ParseIP(gateway)
	if ip == nil || ip.To4() == nil || !ip.IsGlobalUnicast() {
		return bad("gateway_format", "网关须为热点侧有效的单播 IPv4 地址")
	}
	var selected *NetworkInterface
	owners := []string{}
	for i := range n.Interfaces {
		v := &n.Interfaces[i]
		if v.Name == lan {
			selected = v
		}
		for _, a := range v.Addresses {
			addr, _, e := net.ParseCIDR(a)
			if e == nil && addr.Equal(ip) {
				owners = append(owners, v.Name)
			}
		}
	}
	if selected == nil {
		if n.Complete {
			return bad("interface_missing", "配置接口不存在："+lan)
		}
		return NetworkValidation{"unknown", "query_failed", "无法验证接口是否存在：系统查询失败或信息不完整"}
	}
	if selected.Loopback || selected.PointToPoint || selected.Uplink || strings.HasPrefix(lan, "rmnet") || strings.HasPrefix(lan, "ccmni") || strings.HasPrefix(lan, "wwan") {
		return bad("interface_unsuitable", "所选接口是回环、点对点或上联接口，不能用作热点接口")
	}
	if !contains(owners, lan) {
		if len(owners) > 0 {
			return bad("gateway_mismatch", fmt.Sprintf("%s 当前配置在 %s，而不是 %s。请检查热点接口。", gateway, strings.Join(owners, "、"), lan))
		}
		if selected.Complete {
			return bad("gateway_mismatch", "该 IPv4 地址未配置在所选接口上")
		}
		return NetworkValidation{"unknown", "query_failed", "无法验证网关归属：接口地址信息不完整"}
	}
	if !selected.Up {
		return bad("interface_down", "所选接口未启用，请先在宿主开启热点")
	}
	return NetworkValidation{"valid", "ok", "接口存在、已启用，热点侧 IPv4 地址匹配"}
}
func readNetworkConfig(name string, state bool) (*NetworkConfig, error) {
	b, e := os.ReadFile(filepath.Join(dir, name))
	if e != nil {
		return nil, e
	}
	if state {
		var v struct{ Config *NetworkConfig }
		e = json.Unmarshal(b, &v)
		if e != nil {
			return nil, e
		}
		if v.Config == nil {
			return nil, errors.New("缺少 Config")
		}
		return v.Config, nil
	}
	var v NetworkConfig
	e = json.Unmarshal(b, &v)
	return &v, e
}
func localNetwork() (*NetworkConfig, error) {
	t := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(dir, "control.sock"))
	}}
	defer t.CloseIdleConnections()
	c := &http.Client{Transport: t, Timeout: 2 * time.Second}
	r, e := c.Get("http://local/network")
	if e != nil {
		return nil, e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return nil, errors.New("服务未就绪或旧版本不支持实际配置查询")
	}
	var v NetworkConfig
	e = json.NewDecoder(r.Body).Decode(&v)
	return &v, e
}
func networkReport(hint, lan, gateway string) NetworkReport {
	n := inspectNetwork()
	r := NetworkReport{Protocol: 1, Interfaces: append([]NetworkInterface{}, n.Interfaces...), Warnings: append([]string{}, n.Warnings...), Candidates: []NetworkCandidate{}}
	for _, x := range []struct {
		name   string
		state  bool
		target **NetworkConfig
	}{{"state.json", true, &r.Saved}, {"bootstrap.json", false, &r.Bootstrap}} {
		v, e := readNetworkConfig(x.name, x.state)
		if e == nil {
			*x.target = v
		} else if !os.IsNotExist(e) {
			r.Warnings = append(r.Warnings, x.name+" 无法读取或格式无效")
		}
	}
	if v, e := localNetwork(); e == nil {
		r.Running = v
	} else {
		r.Warnings = append(r.Warnings, "无法取得运行时配置：服务未运行、未就绪或旧版本不支持；保存值不代表实际运行值")
	}
	if r.Saved != nil {
		v := validateNetwork(n, r.Saved.LAN, r.Saved.Gateway)
		r.SavedValidation = &v
		if r.Bootstrap != nil && (r.Saved.LAN != r.Bootstrap.LAN || r.Saved.Gateway != r.Bootstrap.Gateway) {
			r.Warnings = append(r.Warnings, "保存配置与安装引导不一致；已有状态优先，修改安装输入不会覆盖已有配置")
		}
	}
	if r.Running != nil && r.Saved != nil && *r.Running != *r.Saved {
		r.Warnings = append(r.Warnings, "实际运行配置与保存配置不同")
	}
	for _, i := range n.Interfaces {
		for _, a := range i.Addresses {
			ip, _, e := net.ParseCIDR(a)
			if e != nil {
				continue
			}
			if validateNetwork(n, i.Name, ip.String()).Status != "valid" {
				continue
			}
			reason := "接口已启用且持有该 IPv4 地址；请确认是热点侧地址"
			if ip.String() == hint {
				reason = "与当前访问的本机 IPv4 地址一致（已经系统地址核对）"
			}
			r.Candidates = append(r.Candidates, NetworkCandidate{i.Name, ip.String(), reason})
		}
	}
	// A host hint is never trusted as an address assignment. No name-based positive recommendation.
	matches := []NetworkCandidate{}
	if n.RouteKnown {
		for _, c := range r.Candidates {
			if c.Gateway == hint {
				matches = append(matches, c)
			}
		}
	}
	if len(matches) == 1 {
		r.Recommended = &matches[0]
	} else {
		r.Warnings = append(r.Warnings, "无法唯一可靠推荐，请从候选中确认热点接口和地址；网关不是蜂窝上游默认网关")
	}
	if lan != "" || gateway != "" {
		r.Validation = validateNetwork(n, lan, gateway)
	} else if r.Saved != nil {
		r.Validation = *r.SavedValidation
	} else {
		r.Validation = NetworkValidation{"unknown", "not_selected", "请选择接口和热点侧 IPv4 地址"}
	}
	return r
}
