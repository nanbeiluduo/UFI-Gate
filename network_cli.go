package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

var networkWorker *bool

func networkCLIFlags() func() bool {
	inspect := flag.Bool("network-inspect", false, "Root：输出脱敏热点检测 JSON")
	repair := flag.Bool("network-repair", false, "Root：事务修复接口及网关")
	resume := flag.Bool("resume-network", false, "Root：恢复中断的网络配置事务")
	offline := flag.Bool("network-offline", false, "Root：停止后恢复网络并关闭认证")
	lan := flag.String("network-lan", "", "热点接口")
	gateway := flag.String("network-gateway", "", "热点侧 IPv4")
	hint := flag.String("network-host", "", "访问地址推荐线索")
	id := flag.String("network-job", "", "维护任务 ID")
	networkWorker = flag.Bool("network-worker", false, "维护事务启动的服务")
	return func() bool {
		if !*inspect && !*repair && !*resume && !*offline {
			return false
		}
		var e error
		if os.Geteuid() != 0 || demo {
			e = errors.New("网络维护命令需要设备 Root，演示模式不可执行")
		} else if *inspect {
			json.NewEncoder(os.Stdout).Encode(networkReport(*hint, *lan, *gateway))
			return true
		} else if *offline {
			e = networkOffline()
		} else {
			if *id == "" {
				*id = random(8)
			}
			var r NetworkResult
			r, e = runNetworkRepair(*lan, *gateway, *id, *resume)
			if r.ID != "" {
				json.NewEncoder(os.Stdout).Encode(r)
			}
		}
		if e != nil {
			json.NewEncoder(os.Stdout).Encode(map[string]string{"error": e.Error()})
			os.Exit(1)
		}
		return true
	}
}
func networkOffline() error {
	lock, e := acquireServiceFile()
	if e != nil {
		return e
	}
	defer lock.Close()
	b, e := networkState()
	if e != nil {
		return e
	}
	var db DB
	if e = json.Unmarshal(b, &db); e != nil {
		return e
	}
	f := Firewall{runner: realRun}
	if e = f.cleanup(); e != nil {
		return e
	}
	// Restore standard tether offload from the old state, without reserializing it.
	if old := db.Config.OffloadOriginal; old != "" {
		if old == "null" {
			_, e = realRun("settings", "delete", "global", "tether_offload_disabled")
		} else {
			_, e = realRun("settings", "put", "global", "tether_offload_disabled", old)
		}
		if e != nil {
			return fmt.Errorf("恢复加速设置失败：%w", e)
		}
	}
	b, e = patchNetwork(b, true, "", "")
	if e != nil {
		return e
	}
	return networkWrite(filepath.Join(dir, "state.json"), b)
}
