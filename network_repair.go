package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type NetworkJournal struct {
	ID           string
	Phase        string
	Backup       string
	LAN          string
	Gateway      string
	HadBootstrap bool
}
type NetworkResult struct {
	ID                     string
	Phase                  string
	Message                string
	ConfigRestored         bool
	ServiceRunning         bool
	AuthenticationDisabled bool
	Backup                 string
	Diagnostics            string
}

var networkWrite = durableNetworkWrite
var networkService = networkServiceCommand
var networkHealth = localNetwork
var networkSleep = time.Sleep

func networkServiceCommand(script, action string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	out, e := exec.CommandContext(ctx, "sh", script, action).CombinedOutput()
	if e != nil {
		return fmt.Errorf("服务 %s 失败：%s (%w)", action, out, e)
	}
	return nil
}

// Temp file mode does not inherit a permissive pre-existing .tmp. Rename and fsync
// the parent before advancing the journal, so a power loss can be replayed safely.
func durableNetworkWrite(path string, b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".network-write-")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
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
	if e = os.Rename(tmp, path); e != nil {
		return e
	}
	return syncNetworkDir(filepath.Dir(path))
}
func networkJSON(name string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return networkWrite(filepath.Join(dir, name), b)
}
func networkPending() bool {
	_, e := os.Stat(filepath.Join(dir, "network-journal.json"))
	return !os.IsNotExist(e)
}
func networkState() ([]byte, error) {
	b, e := os.ReadFile(filepath.Join(dir, "state.json"))
	if e != nil {
		return nil, fmt.Errorf("无法读取已有状态，尚未安装或状态不可读：%w", e)
	}
	var db DB
	if e = json.Unmarshal(b, &db); e != nil {
		return nil, errors.New("状态损坏，拒绝覆盖")
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(b, &root) != nil || root["Config"] == nil || string(root["Config"]) == "null" || db.Devices == nil || db.Accounts == nil || db.Requests == nil {
		return nil, errors.New("状态结构无效，拒绝覆盖")
	}
	return b, nil
}

// Patch only the named network fields. Preserve unknown/future business fields.
func patchNetwork(b []byte, state bool, lan, gateway string) ([]byte, error) {
	var root map[string]json.RawMessage
	if e := json.Unmarshal(b, &root); e != nil || root == nil {
		return nil, errors.New("配置 JSON 无效")
	}
	target := root
	if state {
		target = nil
		if e := json.Unmarshal(root["Config"], &target); e != nil || target == nil {
			return nil, errors.New("Config 无效")
		}
	}
	if lan != "" {
		target["LAN"], _ = json.Marshal(lan)
		target["Gateway"], _ = json.Marshal(gateway)
	}
	if state {
		target["Enabled"] = json.RawMessage("false")
		root["Config"], _ = json.Marshal(target)
	}
	return json.MarshalIndent(root, "", "  ")
}
func saveNetworkJournal(j NetworkJournal) error { return networkJSON("network-journal.json", j) }
func closeNetworkJournal() error {
	if e := os.Remove(filepath.Join(dir, "network-journal.json")); e != nil {
		return e
	}
	return syncNetworkDir(dir)
}
func acquireServiceFile() (*os.File, error) {
	f, e := os.OpenFile(filepath.Join(dir, "service.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = lockFile(f); e != nil {
		f.Close()
		return nil, errors.New("旧服务尚未退出，拒绝修改配置")
	}
	return f, nil
}
func waitNetwork(lan, gateway string) error {
	good := 0
	for i := 0; i < 25; i++ {
		v, e := networkHealth()
		if e == nil && v.LAN == lan && v.Gateway == gateway && !v.Enabled {
			good++
			if good == 3 {
				return nil
			}
		} else {
			good = 0
		}
		networkSleep(time.Second)
	}
	return errors.New("服务就绪/实际接口/网关/认证关闭检查未通过")
}
func networkOutcome(j NetworkJournal, phase, msg string, restored bool) NetworkResult {
	r := NetworkResult{ID: j.ID, Phase: phase, Message: msg, ConfigRestored: restored, Backup: j.Backup, Diagnostics: filepath.Join(dir, "network-repair.log")}
	saved, e := readNetworkConfig("state.json", true)
	if e == nil {
		r.AuthenticationDisabled = !saved.Enabled
	}
	if live, e := networkHealth(); e == nil {
		r.ServiceRunning = true
		r.AuthenticationDisabled = r.AuthenticationDisabled && !live.Enabled
	} else if serviceLock, e := acquireServiceFile(); e == nil {
		// A stopped worker cannot enforce authentication; retain the saved-state check.
		serviceLock.Close()
	} else {
		// Failed health does not prove that a still-running worker has disabled rules.
		r.AuthenticationDisabled = false
	}
	return r
}
func runNetworkRepair(lan, gateway, id string, resume bool) (NetworkResult, error) {
	lock, e := acquireUpdateLock()
	if e != nil {
		return NetworkResult{}, e
	}
	defer lock.Close()
	if _, e = os.Stat(filepath.Join(dir, "update-journal.json")); !os.IsNotExist(e) {
		return NetworkResult{}, errors.New("存在更新事务，请先恢复更新，不能同时修复网络配置")
	}
	j := NetworkJournal{ID: id, Phase: "stopping", LAN: lan, Gateway: gateway}
	if resume {
		b, e := os.ReadFile(filepath.Join(dir, "network-journal.json"))
		if e != nil {
			return NetworkResult{}, e
		}
		if json.Unmarshal(b, &j) != nil || j.ID == "" || !contains([]string{"stopping", "backed_up"}, j.Phase) || !ifaceRE.MatchString(j.ID) || filepath.Clean(j.Backup) != filepath.Join(dir, "network-backup-"+j.ID) {
			return NetworkResult{}, errors.New("网络恢复记录损坏，拒绝覆盖；请保留诊断文件")
		}
		return rollbackNetwork(j, errors.New("上次配置修复中断，恢复原配置并关闭认证"))
	}
	if networkPending() {
		return NetworkResult{}, errors.New("存在中断的配置修复，请先启动服务执行恢复")
	}
	if !ifaceRE.MatchString(id) {
		return NetworkResult{}, errors.New("任务编号无效")
	}
	v := validateNetwork(inspectNetwork(), lan, gateway)
	if v.Status != "valid" {
		return NetworkResult{}, errors.New(v.Message)
	}
	if _, e = networkState(); e != nil {
		return NetworkResult{}, e
	}
	bootstrap, e := os.ReadFile(filepath.Join(dir, "bootstrap.json"))
	if e != nil && !os.IsNotExist(e) {
		return NetworkResult{}, e
	}
	j.HadBootstrap = e == nil
	if j.HadBootstrap {
		if _, e = patchNetwork(bootstrap, false, lan, gateway); e != nil {
			return NetworkResult{}, e
		}
	}
	j.Backup = filepath.Join(dir, "network-backup-"+id)
	if e = os.Mkdir(j.Backup, 0700); e != nil {
		return NetworkResult{}, e
	}
	if e = saveNetworkJournal(j); e != nil {
		return NetworkResult{}, e
	}
	_ = networkJSON("network-result.json", networkOutcome(j, "stopping", "正在停止服务并保存最终流量", false))
	if e = networkService(filepath.Join(dir, "service.sh"), "stop-network"); e != nil {
		return rollbackNetwork(j, e)
	}
	serviceLock, e := acquireServiceFile()
	if e != nil {
		return rollbackNetwork(j, e)
	}
	e = backupNetwork(&j)
	if e == nil {
		e = writeNetworkTarget(j)
	}
	serviceLock.Close()
	if e != nil {
		return rollbackNetwork(j, e)
	}
	if e = networkService(filepath.Join(dir, "service.sh"), "start-network"); e == nil {
		e = waitNetwork(lan, gateway)
	}
	if e != nil {
		return rollbackNetwork(j, e)
	}
	r := networkOutcome(j, "success", "接口和网关已修改，认证保持关闭；请检查设置后手动开启", false)
	if e = networkJSON("network-result.json", r); e != nil {
		return rollbackNetwork(j, e)
	}
	if e = closeNetworkJournal(); e != nil {
		return r, e
	}
	return r, nil
}
func backupNetwork(j *NetworkJournal) error {
	// Only called after supervisor is stopped and service.lock has been acquired.
	b, e := networkState()
	if e != nil {
		return e
	}
	b, e = patchNetwork(b, true, "", "")
	if e != nil {
		return e
	}
	if e = networkWrite(filepath.Join(j.Backup, "state.json"), b); e != nil {
		return e
	}
	if j.HadBootstrap {
		b, e = os.ReadFile(filepath.Join(dir, "bootstrap.json"))
		if e != nil {
			return e
		}
		if _, e = patchNetwork(b, false, "", ""); e != nil {
			return e
		}
		if e = networkWrite(filepath.Join(j.Backup, "bootstrap.json"), b); e != nil {
			return e
		}
	}
	j.Phase = "backed_up"
	return saveNetworkJournal(*j)
}
func writeNetworkTarget(j NetworkJournal) error {
	// Recheck after shutdown; a disappearing hotspot must not be committed as valid.
	if v := validateNetwork(inspectNetwork(), j.LAN, j.Gateway); v.Status != "valid" {
		return errors.New(v.Message)
	}
	b, e := os.ReadFile(filepath.Join(j.Backup, "state.json"))
	if e != nil {
		return e
	}
	b, e = patchNetwork(b, true, j.LAN, j.Gateway)
	if e != nil {
		return e
	}
	if e = networkWrite(filepath.Join(dir, "state.json"), b); e != nil {
		return e
	}
	b = []byte("{}")
	if j.HadBootstrap {
		b, e = os.ReadFile(filepath.Join(j.Backup, "bootstrap.json"))
		if e != nil {
			return e
		}
	}
	b, e = patchNetwork(b, false, j.LAN, j.Gateway)
	if e != nil {
		return e
	}
	return networkWrite(filepath.Join(dir, "bootstrap.json"), b)
}
func rollbackNetwork(j NetworkJournal, cause error) (NetworkResult, error) {
	fail := func(e error) (NetworkResult, error) {
		r := networkOutcome(j, "recovery_required", cause.Error()+"；自动恢复未完成："+e.Error(), false)
		_ = networkJSON("network-result.json", r)
		return r, errors.New(r.Message)
	}
	if e := networkService(filepath.Join(dir, "service.sh"), "stop-network"); e != nil {
		return fail(e)
	}
	lock, e := acquireServiceFile()
	if e != nil {
		return fail(e)
	}
	if j.Phase == "stopping" {
		e = backupNetwork(&j)
	}
	if e != nil {
		lock.Close()
		return fail(e)
	}
	b, e := os.ReadFile(filepath.Join(j.Backup, "state.json"))
	if e == nil {
		b, e = patchNetwork(b, true, "", "")
	}
	if e == nil {
		e = networkWrite(filepath.Join(dir, "state.json"), b)
	}
	if e == nil {
		if j.HadBootstrap {
			b, e = os.ReadFile(filepath.Join(j.Backup, "bootstrap.json"))
			if e == nil {
				e = networkWrite(filepath.Join(dir, "bootstrap.json"), b)
			}
		} else {
			e = os.Remove(filepath.Join(dir, "bootstrap.json"))
			if os.IsNotExist(e) {
				e = nil
			}
			if e == nil {
				e = syncNetworkDir(dir)
			}
		}
	}
	lock.Close()
	if e != nil {
		return fail(e)
	}
	old, e := readNetworkConfig("state.json", true)
	if e != nil {
		return fail(e)
	}
	e = networkService(filepath.Join(dir, "service.sh"), "start-network")
	if e == nil {
		e = waitNetwork(old.LAN, old.Gateway)
	}
	r := networkOutcome(j, "rolled_back", cause.Error()+"；原配置已恢复，认证保持关闭", true)
	if e != nil {
		r.Phase = "recovery_required"
		r.Message += "；服务恢复失败：" + e.Error()
		_ = networkJSON("network-result.json", r)
		return r, errors.New(r.Message)
	}
	if e = networkJSON("network-result.json", r); e != nil {
		return fail(e)
	}
	if e = closeNetworkJournal(); e != nil {
		return r, e
	}
	return r, errors.New(r.Message)
}
