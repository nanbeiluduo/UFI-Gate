package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Injectable process boundaries let transaction tests run without touching host networking.
var updateServiceCommand = serviceCommand
var updateWaitHealth = waitHealth
var updateBinaryVersion = binaryVersion
var updateCheckSpace = checkUpdateSpace
var serviceFiles = []string{"ufi-gate", "service.sh", "emergency-recover.sh"}

func extractUpdate(data []byte, stage string) error {
	z, e := gzip.NewReader(bytes.NewReader(data))
	if e != nil {
		return e
	}
	defer z.Close()
	tr := tar.NewReader(io.LimitReader(z, 64<<20))
	seen := map[string]bool{}
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		if !contains(serviceFiles, h.Name) || seen[h.Name] || h.Typeflag != tar.TypeReg || h.Size < 1 || h.Size > 32<<20 {
			return errors.New("更新包包含不允许的文件")
		}
		seen[h.Name] = true
		f, e := os.OpenFile(filepath.Join(stage, h.Name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
		if e != nil {
			return e
		}
		_, e = io.Copy(f, tr)
		ce := f.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
	}
	if len(seen) != len(serviceFiles) {
		return errors.New("更新包文件不完整")
	}
	return nil
}
func validateStaged(stage, version string) error {
	for _, n := range serviceFiles {
		s, e := os.Lstat(filepath.Join(stage, n))
		if e != nil {
			return e
		}
		if !s.Mode().IsRegular() {
			return errors.New("安装文件必须是普通文件")
		}
	}
	v, e := updateBinaryVersion(filepath.Join(stage, "ufi-gate"))
	if e != nil {
		return fmt.Errorf("候选服务无法运行：%w", e)
	}
	if v.Version != version || v.Arch != buildArch() || v.Protocol != updateProtocol {
		return errors.New("候选服务版本、架构或协议不匹配")
	}
	return nil
}
func copyAtomic(src, dst string, mode os.FileMode) error {
	b, e := os.ReadFile(src)
	if e != nil {
		return e
	}
	if e = atomicWrite(dst, b); e != nil {
		return e
	}
	return os.Chmod(dst, mode)
}
func localHealth() (VersionInfo, error) {
	var v VersionInfo
	t := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(dir, "control.sock"))
	}}
	defer t.CloseIdleConnections()
	c := &http.Client{Transport: t, Timeout: 2 * time.Second}
	r, e := c.Get("http://local/health")
	if e != nil {
		return v, e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return v, errors.New("服务尚未就绪")
	}
	e = json.NewDecoder(r.Body).Decode(&v)
	return v, e
}
func waitHealth(expected string) error {
	consecutive := 0
	for i := 0; i < 30; i++ {
		v, e := localHealth()
		if e == nil && v.Version == expected {
			consecutive++
			if consecutive >= 3 {
				return nil
			}
		} else {
			consecutive = 0
		}
		time.Sleep(time.Second)
	}
	return errors.New("启动健康检查超时或版本不一致")
}
func waitLegacyService() error {
	client := &http.Client{Transport: &http.Transport{}, Timeout: 2 * time.Second}
	defer client.CloseIdleConnections()
	stateBytes, e := os.ReadFile(filepath.Join(dir, "state.json"))
	if e != nil {
		return e
	}
	var state struct{ Config struct{ Gateway string } }
	if e = json.Unmarshal(stateBytes, &state); e != nil {
		return e
	}
	for i := 0; i < 20; i++ {
		raw, e := os.ReadFile(filepath.Join(dir, "worker.pid"))
		if e == nil {
			pid := strings.TrimSpace(string(raw))
			if regexpPID(pid) {
				exe, e := os.Readlink("/proc/" + pid + "/exe")
				if e == nil && exe == filepath.Join(dir, "ufi-gate") {
					req, _ := http.NewRequest("GET", "http://127.0.0.1:8090/", nil)
					req.Host = state.Config.Gateway + ":8090"
					res, e := client.Do(req)
					if e == nil {
						res.Body.Close()
						if res.StatusCode == 200 {
							return nil
						}
					}
				}
			}
		}
		time.Sleep(time.Second)
	}
	return errors.New("旧服务已尝试启动，但进程/门户健康检查未通过")
}
func regexpPID(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
func serviceCommand(script, action string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	out, e := exec.CommandContext(ctx, "sh", script, action).CombinedOutput()
	if e != nil {
		return fmt.Errorf("服务 %s 失败：%s (%w)", action, out, e)
	}
	return nil
}

type UpdateJournal struct {
	Job                       UpdateStatus
	Stage, Backup, OldVersion string
	HadService                bool
	HadState                  bool
	BackupReady               bool
}

func saveJournal(j UpdateJournal) error {
	b, e := json.Marshal(j)
	if e != nil {
		return e
	}
	return atomicWrite(filepath.Join(dir, "update-journal.json"), b)
}
func launchUpdater(stage string, job UpdateStatus, lock *os.File) error {
	runner := filepath.Join(dir, "update-runner")
	// Keep a separate executable alive while the main daemon and its binary are replaced.
	self, e := os.Executable()
	if e != nil {
		return e
	}
	if e = copyAtomic(self, runner, 0700); e != nil {
		return e
	}
	f, e := os.OpenFile(filepath.Join(dir, "update.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	cmd := exec.Command(runner, "--dir", dir, "--apply-update", stage, "--update-version", job.Version, "--update-id", job.ID, "--inherited-lock")
	cmd.Stdout = f
	cmd.Stderr = f
	cmd.ExtraFiles = []*os.File{lock}
	detachUpdater(cmd)
	if e = cmd.Start(); e != nil {
		return e
	}
	return cmd.Process.Release()
}
func runInstall(stage, target, id string, repair, resume bool) error {
	if demo {
		return errors.New("演示模式禁止安装")
	}
	if resume {
		raw, e := os.ReadFile(filepath.Join(dir, "update-journal.json"))
		if e != nil {
			return e
		}
		var j UpdateJournal
		if e = json.Unmarshal(raw, &j); e != nil {
			return e
		}
		if filepath.Clean(j.Backup) != filepath.Join(dir, "update-backup") || filepath.Dir(filepath.Clean(j.Stage)) != filepath.Clean(dir) || !stringsHasStagePrefix(filepath.Base(j.Stage)) {
			return errors.New("恢复记录路径无效")
		}
		return rollbackInstall(j, errors.New("上次更新中断，恢复旧服务"))
	}
	stage = filepath.Clean(stage)
	if filepath.Dir(stage) != filepath.Clean(dir) || !stringsHasStagePrefix(filepath.Base(stage)) {
		return errors.New("安装暂存目录无效")
	}
	if e := validateStaged(stage, target); e != nil {
		return e
	}
	installed, e := installedVersion()
	if e != nil {
		return e
	}
	if e = allowInstall(target, installed, repair); e != nil {
		return e
	}
	if running, e := localHealth(); e == nil {
		if e = allowInstall(target, running, repair); e != nil {
			return e
		}
	}
	j := UpdateJournal{Job: UpdateStatus{ID: id, Version: target, Phase: "preparing", Message: "备份服务文件"}, Stage: stage, Backup: filepath.Join(dir, "update-backup"), OldVersion: installed.Version, HadService: installed.Version != ""}
	if e = updateCheckSpace(stage); e != nil {
		return e
	}
	if e = os.MkdirAll(j.Backup, 0700); e != nil {
		return e
	}
	// A journal is committed only after all executable backups are complete.
	for _, n := range serviceFiles {
		if _, e = os.Stat(filepath.Join(dir, n)); e == nil {
			if e = copyAtomic(filepath.Join(dir, n), filepath.Join(j.Backup, n), 0700); e != nil {
				return e
			}
		} else if j.HadService {
			return fmt.Errorf("旧安装缺少 %s，拒绝自动覆盖", n)
		}
	}
	self, e := os.Executable()
	if e != nil {
		return e
	}
	if e = copyAtomic(self, filepath.Join(dir, "update-runner"), 0700); e != nil {
		return e
	}
	if e = saveJournal(j); e != nil {
		return e
	}
	// Install the recovery-aware supervisor first; it keeps the boot recovery hook during stop.
	if e = copyAtomic(filepath.Join(stage, "service.sh"), filepath.Join(dir, "service.sh"), 0700); e != nil {
		return rollbackInstall(j, e)
	}
	j.Job.Phase = "stopping"
	j.Job.Message = "停止服务，保存最终状态"
	_ = saveUpdateStatus(j.Job)
	if j.HadService {
		if e = updateServiceCommand(filepath.Join(dir, "service.sh"), "stop-update"); e != nil {
			return rollbackInstall(j, e)
		}
	}
	// stop persists final counters and disables authentication. Preserve this exact snapshot.
	if _, e = os.Stat(filepath.Join(dir, "state.json")); e == nil {
		j.HadState = true
		if e = copyAtomic(filepath.Join(dir, "state.json"), filepath.Join(j.Backup, "state.json"), 0600); e != nil {
			return rollbackInstall(j, e)
		}
	} else if !os.IsNotExist(e) {
		return rollbackInstall(j, e)
	}
	j.BackupReady = true
	if e = saveJournal(j); e != nil {
		return rollbackInstall(j, e)
	}
	j.Job.Phase = "installing"
	j.Job.Message = "替换服务文件"
	_ = saveUpdateStatus(j.Job)
	for _, n := range serviceFiles {
		if e = copyAtomic(filepath.Join(stage, n), filepath.Join(dir, n), 0700); e != nil {
			return rollbackInstall(j, e)
		}
	}
	if !j.HadService {
		if _, e = os.Stat(filepath.Join(dir, "state.json")); os.IsNotExist(e) {
			if _, e = os.Stat(filepath.Join(stage, "bootstrap.json")); e == nil {
				if e = copyAtomic(filepath.Join(stage, "bootstrap.json"), filepath.Join(dir, "bootstrap.json"), 0600); e != nil {
					return rollbackInstall(j, e)
				}
			}
		}
	}
	j.Job.Phase = "starting"
	j.Job.Message = "等待新服务健康检查"
	_ = saveUpdateStatus(j.Job)
	if e = updateServiceCommand(filepath.Join(dir, "service.sh"), "start-update"); e == nil {
		e = updateWaitHealth(target)
	}
	if e != nil {
		return rollbackInstall(j, e)
	}
	j.Job.Phase = "success"
	j.Job.Message = "更新完成，认证服务保持关闭，请检查后手动开启"
	if e = saveUpdateStatus(j.Job); e != nil {
		return e
	}
	if e = os.Remove(filepath.Join(dir, "update-journal.json")); e != nil {
		return e
	}
	_ = os.RemoveAll(stage)
	return nil
}
func stringsHasStagePrefix(s string) bool { return len(s) > 13 && s[:13] == "update-stage-" }
func rollbackInstall(j UpdateJournal, cause error) error {
	j.Job.Phase = "rolling_back"
	j.Job.Message = cause.Error()
	_ = saveUpdateStatus(j.Job)
	if j.HadService || j.BackupReady { // Stop any worker before restoring files.
		if e := updateServiceCommand(filepath.Join(dir, "service.sh"), "stop-update"); e != nil {
			return rollbackFailure(j, e)
		}
	}
	if !j.HadService {
		j.Job.Phase = "failed"
		j.Job.Message = "首次安装未成功：" + cause.Error()
		_ = saveUpdateStatus(j.Job)
		_ = os.Remove(filepath.Join(dir, "update-journal.json"))
		return cause
	}
	for _, n := range serviceFiles {
		if e := copyAtomic(filepath.Join(j.Backup, n), filepath.Join(dir, n), 0700); e != nil {
			return rollbackFailure(j, e)
		}
	}
	if j.BackupReady && j.HadState {
		if _, e := os.Stat(filepath.Join(j.Backup, "state.json")); e == nil {
			if e = copyAtomic(filepath.Join(j.Backup, "state.json"), filepath.Join(dir, "state.json"), 0600); e != nil {
				return rollbackFailure(j, e)
			}
		}
	}
	// Legacy releases support start, but not start-update nor the health endpoint.
	action := "start-update"
	if v, e := updateBinaryVersion(filepath.Join(dir, "ufi-gate")); e != nil || v.Protocol < 1 {
		action = "start"
	}
	if e := updateServiceCommand(filepath.Join(dir, "service.sh"), action); e != nil {
		return rollbackFailure(j, e)
	}
	if action == "start-update" {
		if e := updateWaitHealth(j.OldVersion); e != nil {
			return rollbackFailure(j, e)
		}
	} else if e := waitLegacyService(); e != nil {
		return rollbackFailure(j, e)
	}
	j.Job.Phase = "rolled_back"
	j.Job.Message = "更新未成功，已恢复旧服务：" + cause.Error()
	_ = saveUpdateStatus(j.Job)
	_ = os.Remove(filepath.Join(dir, "update-journal.json"))
	return cause
}
func rollbackFailure(j UpdateJournal, e error) error {
	j.Job.Phase = "rollback_failed"
	j.Job.Message = "自动恢复失败，请在宿主恢复网络并查看 update.log：" + e.Error()
	_ = saveUpdateStatus(j.Job)
	return errors.New(j.Job.Message)
}
