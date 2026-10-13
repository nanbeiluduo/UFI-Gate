package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func hostapdSockets() []string {
	paths := map[string]bool{}
	// Android vendors use both filesystem and abstract UNIX sockets.
	if data, err := os.ReadFile("/proc/net/unix"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 8 && fields[4] == "0002" {
				path := fields[7]
				if strings.Contains(strings.ToLower(path), "hostapd") && (strings.HasPrefix(path, "/") || strings.HasPrefix(path, "@")) {
					paths[path] = true
				}
			}
		}
	}
	for _, root := range []string{"/data/vendor/wifi/hostapd", "/data/misc/wifi/hostapd", "/data/misc/apexdata/com.android.wifi/hostapd", "/var/run/hostapd", "/run/hostapd"} {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if entry.IsDir() && strings.Count(strings.TrimPrefix(path, root), string(os.PathSeparator)) > 2 {
				return filepath.SkipDir
			}
			if entry.Type()&os.ModeSocket != 0 {
				paths[path] = true
			}
			return nil
		})
	}
	result := []string{}
	for path := range paths {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func hostapdRequest(path, command string, deadline time.Time) (string, error) {
	if time.Now().After(deadline) {
		return "", errors.New("踢线操作超时")
	}
	local := filepath.Join(dir, "kick-"+random(6))
	conn, err := net.DialUnix("unixgram", &net.UnixAddr{Name: local, Net: "unixgram"}, &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		return "", err
	}
	defer os.Remove(local)
	defer conn.Close()
	if err = conn.SetDeadline(deadline); err != nil {
		return "", err
	}
	if _, err = conn.Write([]byte(command)); err != nil {
		return "", err
	}
	buf := make([]byte, 8192)
	n, err := conn.Read(buf)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(buf[:n])), nil
}

func deauthenticateOnce(mac string, paths []string, request func(string, string, time.Time) (string, error)) error {
	if !validMAC(mac) {
		return errors.New("MAC 地址无效")
	}
	mac = strings.ToLower(mac)
	deadline := time.Now().Add(4 * time.Second)
	failures := []string{}
	for _, path := range paths {
		if time.Now().After(deadline) {
			failures = append(failures, "探测超时")
			break
		}
		end := time.Now().Add(500 * time.Millisecond)
		if end.After(deadline) {
			end = deadline
		}
		// Find the BSS actually serving this station; global sockets/other BSSes cannot be used blindly.
		station, err := request(path, "STA "+mac, end)
		if err != nil {
			failures = append(failures, path+": "+err.Error())
			continue
		}
		fields := strings.Fields(station)
		if len(fields) == 0 || !strings.EqualFold(fields[0], mac) {
			failures = append(failures, path+": 未查到该设备")
			continue
		}
		response, err := request(path, "DEAUTHENTICATE "+mac, deadline)
		if err == nil && response == "OK" {
			return nil
		}
		if err != nil {
			failures = append(failures, path+": "+err.Error())
		} else {
			failures = append(failures, path+": "+response)
		}
	}
	if len(paths) == 0 {
		return errors.New("未发现可用的 hostapd 数据报控制口，固件可能未开放此接口")
	}
	return fmt.Errorf("hostapd 踢线未成功：%s", strings.Join(failures, "；"))
}

// Called with g.mu held. Bounded best effort; clearing authorization does not depend on hardware support.
func (g *Gate) kickStation(mac string) {
	if demo {
		g.event("kick", mac+" 演示模式，不执行 WiFi 踢线")
		return
	}
	if err := deauthenticateOnce(mac, hostapdSockets(), hostapdRequest); err != nil {
		g.event("kick", mac+" "+err.Error())
		return
	}
	g.event("kick", mac+" hostapd 已接受单次断开命令（设备可能自动重连）")
}
