package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

func updateCLIFlags() bool {
	versionFlag := flag.Bool("version-json", false, "输出服务版本")
	installed := flag.Bool("installed-info", false, "只读查询已安装版本")
	health := flag.Bool("health", false, "查询运行中服务健康状态")
	stage := flag.String("apply-update", "", "安装暂存文件")
	target := flag.String("update-version", "", "目标版本")
	id := flag.String("update-id", "", "更新任务编号")
	inherited := flag.Bool("inherited-lock", false, "继承更新锁")
	resume := flag.Bool("resume-update", false, "恢复中断的更新")
	repair := flag.Bool("repair", false, "允许同版本修复")
	flag.Parse()
	if *versionFlag {
		_ = json.NewEncoder(os.Stdout).Encode(VersionInfo{Version: currentVersion(), Arch: buildArch(), Protocol: updateProtocol})
		return true
	}
	if *installed {
		v, e := installedVersion()
		if e != nil {
			log.Fatal(e)
		}
		if running, e := localHealth(); e == nil {
			if c, _ := compareVersions(running.Version, v.Version); v.Version == "" || c > 0 {
				v = running
			}
		}
		_ = json.NewEncoder(os.Stdout).Encode(v)
		return true
	}
	if *health {
		v, e := localHealth()
		if e != nil {
			log.Fatal(e)
		}
		_ = json.NewEncoder(os.Stdout).Encode(v)
		return true
	}
	if *stage == "" && !*resume {
		return false
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		log.Fatal(e)
	}
	var lock *os.File
	var e error
	if *inherited {
		lock = os.NewFile(3, "update-lock")
		if lock == nil {
			log.Fatal("更新锁无效")
		}
		if _, e = lock.Stat(); e != nil {
			log.Fatal(e)
		}
	} else {
		lock, e = acquireUpdateLock()
		if e != nil {
			log.Fatal(e)
		}
	}
	defer lock.Close()
	if !*resume {
		if _, e = os.Stat(filepath.Join(dir, "update-journal.json")); e == nil {
			log.Fatal("存在中断的更新，请先执行恢复")
		}
	}
	if *id == "" {
		*id = random(8)
	}
	if *target == "" {
		*target = currentVersion()
	}
	if e = runInstall(*stage, *target, *id, *repair, *resume); e != nil {
		status := readUpdateStatus()
		if (!*resume && status.ID != *id) || !contains([]string{"rolled_back", "rollback_failed", "failed"}, status.Phase) {
			_ = saveUpdateStatus(UpdateStatus{ID: *id, Version: *target, Phase: "failed", Message: e.Error()})
		}
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	fmt.Println("VERIFIED_HEALTHY")
	return true
}
