package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const updateProtocol = 1

var updatePublicKey string // Base64 Ed25519 key injected by build.py. Never a private key.
var releaseVersion string

func currentVersion() string {
	if releaseVersion != "" {
		return releaseVersion
	}
	return version
}
func versionParts(s string) ([3]int64, error) {
	var out [3]int64
	s = strings.TrimPrefix(s, "v")
	if !regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(s) {
		return out, errors.New("版本须为 X.Y.Z")
	}
	for i, v := range strings.Split(s, ".") {
		n, e := strconv.ParseInt(v, 10, 32)
		if e != nil {
			return out, e
		}
		out[i] = n
	}
	return out, nil
}
func compareVersions(a, b string) (int, error) {
	x, e := versionParts(a)
	if e != nil {
		return 0, e
	}
	y, e := versionParts(b)
	if e != nil {
		return 0, e
	}
	for i := range x {
		if x[i] > y[i] {
			return 1, nil
		}
		if x[i] < y[i] {
			return -1, nil
		}
	}
	return 0, nil
}
func buildArch() string {
	if runtime.GOARCH == "arm" {
		return "arm32"
	}
	if runtime.GOARCH == "arm64" {
		return "arm64"
	}
	return runtime.GOARCH
}

type VersionInfo struct {
	Version  string `json:"version"`
	Arch     string `json:"arch"`
	Protocol int    `json:"protocol"`
	Legacy   bool   `json:"legacy"`
}

func binaryVersion(path string) (VersionInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	raw, e := exec.CommandContext(ctx, path, "--version-json").Output()
	var v VersionInfo
	if e != nil {
		return v, e
	}
	if e = json.Unmarshal(raw, &v); e != nil {
		return v, e
	}
	_, e = versionParts(v.Version)
	return v, e
}
func installedVersion() (VersionInfo, error) {
	path := filepath.Join(dir, "ufi-gate")
	if _, e := os.Stat(path); os.IsNotExist(e) {
		return VersionInfo{}, nil
	}
	if v, e := updateBinaryVersion(path); e == nil {
		return v, nil
	}
	// Older releases expose their last startup version in state.json, not a CLI flag.
	raw, e := os.ReadFile(filepath.Join(dir, "state.json"))
	if e != nil {
		return VersionInfo{}, errors.New("无法识别本地版本，拒绝覆盖")
	}
	var state struct{ Logs []Event }
	if json.Unmarshal(raw, &state) != nil {
		return VersionInfo{}, errors.New("旧状态无法读取，拒绝覆盖")
	}
	for i := len(state.Logs) - 1; i >= 0; i-- {
		l := state.Logs[i]
		if l.Kind == "service" && strings.HasPrefix(l.Message, "服务启动 ") {
			v := strings.TrimPrefix(l.Message, "服务启动 ")
			if _, e = versionParts(v); e == nil {
				return VersionInfo{Version: v, Legacy: true}, nil
			}
		}
	}
	return VersionInfo{}, errors.New("无法识别本地版本，拒绝覆盖；请保留配置并通过恢复流程处理")
}
func allowInstall(target string, installed VersionInfo, repair bool) error {
	if _, e := versionParts(target); e != nil {
		return e
	}
	if installed.Version == "" {
		return nil
	}
	c, e := compareVersions(target, installed.Version)
	if e != nil {
		return e
	}
	if c < 0 {
		return fmt.Errorf("已阻止降级：本地 %s，安装包 %s", installed.Version, target)
	}
	if c == 0 && !repair {
		return errors.New("已安装同版本；请明确选择修复安装")
	}
	return nil
}
