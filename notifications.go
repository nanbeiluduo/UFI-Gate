package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Notification struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Message   string `json:"message"`
	Target    string `json:"target"`
	Key       string `json:"-"`
	Time      int64  `json:"time"`
	Read      bool   `json:"read"`
	PushState string `json:"pushState"`
	Attempts  int    `json:"-"`
	NextTry   int64  `json:"-"`
	RequestID string `json:"requestID,omitempty"`
}

// Exported persistence fields intentionally use separate wire representation below.
func (n Notification) MarshalJSON() ([]byte, error) {
	type wire Notification
	return json.Marshal(struct {
		wire
		Dedup   string
		Tries   int
		RetryAt int64
	}{wire(n), n.Key, n.Attempts, n.NextTry})
}
func (n *Notification) UnmarshalJSON(b []byte) error {
	type wire Notification
	v := struct {
		wire
		Dedup   string
		Tries   int
		RetryAt int64
	}{}
	if e := json.Unmarshal(b, &v); e != nil {
		return e
	}
	*n = Notification(v.wire)
	n.Key = v.Dedup
	n.Attempts = v.Tries
	n.NextTry = v.RetryAt
	return nil
}

var notificationKinds = map[string]string{"approval": "上网审批", "auth_failure": "认证异常", "quota_warning": "流量即将耗尽", "quota": "流量达到上限", "error": "服务异常", "admin": "管理员变更", "update_available": "发现新版本", "update_result": "更新完成", "update_failed": "更新失败", "push_failed": "推送失败"}

func (g *Gate) pushEnabled(kind string) bool {
	if g.db.Config.NotifyPush == nil {
		return contains([]string{"approval", "quota", "error", "update_failed"}, kind)
	}
	return g.db.Config.NotifyPush[kind]
}
func (g *Gate) notify(kind, message, target, key string, push bool) *Notification {
	now := time.Now().Unix()
	for i := len(g.db.Notifications) - 1; i >= 0; i-- {
		n := &g.db.Notifications[i]
		if n.Key == key && (strings.HasPrefix(key, "release:") || strings.HasPrefix(key, "update:") || now-n.Time < 600) {
			return n
		}
	}
	n := Notification{ID: random(10), Kind: kind, Message: message, Target: target, Key: key, Time: now, PushState: "off"}
	if push && g.pushEnabled(kind) && g.db.Config.Push.Kind != "" {
		n.PushState = "pending"
	}
	g.db.Notifications = append(g.db.Notifications, n)
	if len(g.db.Notifications) > 300 {
		g.db.Notifications = g.db.Notifications[len(g.db.Notifications)-300:]
	}
	return &g.db.Notifications[len(g.db.Notifications)-1]
}
func (g *Gate) notificationEvent(kind, message string) {
	target := "logs"
	switch kind {
	case "quota":
		target = "devices"
	case "admin":
		target = "system"
	case "error":
	default:
		return
	}
	g.notify(kind, message, target, kind+":"+message, true)
}
func (g *Gate) notificationsAPI(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/admin-api/notifications") {
		return false
	}
	if r.Method == "GET" && r.URL.Path == "/admin-api/notifications" {
		unread := 0
		for _, n := range g.db.Notifications {
			if !n.Read {
				unread++
			}
		}
		prefs := map[string]bool{}
		for k := range notificationKinds {
			prefs[k] = g.pushEnabled(k)
		}
		reply(w, 200, map[string]any{"items": g.db.Notifications, "unread": unread, "kinds": notificationKinds, "push": prefs})
		return true
	}
	if r.Method != "POST" {
		http.Error(w, "方法不支持", 405)
		return true
	}
	var e error
	switch r.URL.Path {
	case "/admin-api/notifications/read":
		var v struct {
			ID  string
			All bool
		}
		e = body(w, r, &v)
		if e == nil {
			for i := range g.db.Notifications {
				n := &g.db.Notifications[i]
				if v.All || n.ID == v.ID {
					n.Read = true
				}
			}
			e = g.save()
		}
	case "/admin-api/notifications/settings":
		var v struct{ Push map[string]bool }
		e = body(w, r, &v)
		if e == nil {
			for k := range v.Push {
				if _, ok := notificationKinds[k]; !ok {
					e = errors.New("未知通知类型")
				}
			}
		}
		if e == nil {
			delete(v.Push, "push_failed")
			if v.Push == nil {
				v.Push = map[string]bool{}
			}
			g.db.Config.NotifyPush = v.Push
			e = g.save()
		}
	default:
		e = errors.New("通知接口不存在")
	}
	if e != nil {
		fail(w, e)
	} else {
		reply(w, 200, map[string]bool{"ok": true})
	}
	return true
}
func (g *Gate) notificationLoop() {
	g.mu.Lock()
	raw, _ := os.ReadFile(filepath.Join(dir, "release-cache.json"))
	_ = json.Unmarshal(raw, &g.release)
	if !demo {
		s := readUpdateStatus()
		if updateBusy(s.Phase) {
			if _, e := os.Stat(filepath.Join(dir, "update-journal.json")); os.IsNotExist(e) {
				if lock, e := acquireUpdateLock(); e == nil {
					s.Phase = "failed"
					s.Message = "上次更新准备过程已中断，现有服务未被替换，请重新检查更新"
					_ = saveUpdateStatus(s)
					_ = lock.Close()
				}
			}
		}
	}
	g.mu.Unlock()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-g.ctx.Done():
			return
		case <-ticker.C:
		}
		g.mu.Lock()
		now := time.Now().Unix()
		if g.db.Config.AutoUpdateCheck && !g.updateChecking && now-g.release.Checked >= 86400 {
			_ = g.beginReleaseCheck()
		}
		s := readUpdateStatus()
		if contains([]string{"success", "failed", "rolled_back", "rollback_failed"}, s.Phase) && g.db.LastUpdateNotice != s.ID+":"+s.Phase {
			g.db.LastUpdateNotice = s.ID + ":" + s.Phase
			kind := "update_result"
			if s.Phase != "success" {
				kind = "update_failed"
			}
			g.notify(kind, s.Message, "updates", "update:"+s.ID+":"+s.Phase, true)
		}
		for _, d := range g.db.Devices {
			ratio := g.ratio(d)
			if ratio < 0.8 {
				d.QuotaWarned = false
			}
			if ratio >= 0.8 && ratio < 1 && !d.QuotaWarned {
				g.notify("quota_warning", d.MAC+" 配额用量已达到 80%", "devices", "quota-warning:"+d.MAC, true)
				d.QuotaWarned = true
			}
		}
		var work *Notification
		var approval *Request
		p := g.db.Config.Push
		for i := range g.db.Notifications {
			n := &g.db.Notifications[i]
			if n.PushState != "pending" || n.NextTry > now {
				continue
			}
			if !g.pushEnabled(n.Kind) || p.Kind == "" {
				n.PushState = "off"
				continue
			}
			if n.RequestID != "" {
				a := g.db.Requests[n.RequestID]
				if a == nil || a.Status != "pending" || a.Expires <= now {
					n.PushState = "expired"
					continue
				}
				copyA := *a
				approval = &copyA
			}
			copyN := *n
			work = &copyN
			break
		}
		// The existing five-second persistence loop saves notification changes as well.
		g.mu.Unlock()
		if work == nil {
			continue
		}
		var e error
		if approval != nil {
			e = g.sendRequest(p, *approval)
		} else {
			e = g.sendPush(p, "UFI-Gate · "+notificationKinds[work.Kind], work.Message, "", "", nil)
		}
		g.mu.Lock()
		for i := range g.db.Notifications {
			n := &g.db.Notifications[i]
			if n.ID != work.ID {
				continue
			}
			n.Attempts++
			if e == nil {
				n.PushState = "sent"
			} else if n.Attempts >= 3 {
				n.PushState = "failed"
				g.notify("push_failed", "消息推送失败："+safeError(e.Error(), p), "push", "push-failed:"+n.Kind, false)
			} else {
				n.NextTry = time.Now().Unix() + int64(n.Attempts*30)
			}
			break
		}
		_ = g.save()
		g.mu.Unlock()
	}
}
