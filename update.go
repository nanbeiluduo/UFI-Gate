package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const releaseRepo = "nanbeiluduo/UFI-Gate"
const maxUpdatePackage = 24 << 20

type UpdateAsset struct {
	Arch   string `json:"arch"`
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type UpdateManifest struct {
	Version    string        `json:"version"`
	Protocol   int           `json:"protocol"`
	MinVersion string        `json:"min_version"`
	Assets     []UpdateAsset `json:"assets"`
}
type ReleaseInfo struct {
	Version  string          `json:"version"`
	Notes    string          `json:"notes"`
	URL      string          `json:"url"`
	Checked  int64           `json:"checked"`
	Error    string          `json:"error,omitempty"`
	Manifest *UpdateManifest `json:"manifest,omitempty"`
}
type UpdateStatus struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Phase   string `json:"phase"`
	Message string `json:"message"`
	Time    int64  `json:"time"`
}

func updateBusy(phase string) bool {
	return contains([]string{"downloading", "verified", "preparing", "stopping", "installing", "starting", "rolling_back"}, phase)
}
func saveUpdateStatus(s UpdateStatus) error {
	s.Time = time.Now().Unix()
	raw, e := json.Marshal(s)
	if e != nil {
		return e
	}
	return atomicWrite(filepath.Join(dir, "update-status.json"), raw)
}
func readUpdateStatus() UpdateStatus {
	var s UpdateStatus
	raw, _ := os.ReadFile(filepath.Join(dir, "update-status.json"))
	_ = json.Unmarshal(raw, &s)
	return s
}
func officialURL(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	return contains([]string{"api.github.com", "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com"}, u.Hostname())
}
func (g *Gate) updateClient() *http.Client {
	return &http.Client{Transport: g.httpClient.Transport, Timeout: 5 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 || !officialURL(r.URL.String()) {
			return errors.New("更新下载重定向地址被拒绝")
		}
		return nil
	}}
}
func fetchUpdate(ctx context.Context, c *http.Client, address string, limit int64) ([]byte, error) {
	if !officialURL(address) {
		return nil, errors.New("仅支持官方 GitHub 更新地址")
	}
	r, e := http.NewRequestWithContext(ctx, "GET", address, nil)
	if e != nil {
		return nil, e
	}
	r.Header.Set("User-Agent", "UFI-Gate/"+currentVersion())
	r.Header.Set("Accept", "application/vnd.github+json")
	res, e := c.Do(r)
	if e != nil {
		return nil, e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub 返回 HTTP %d", res.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if e != nil {
		return nil, e
	}
	if int64(len(b)) > limit {
		return nil, errors.New("更新文件超过大小限制")
	}
	return b, nil
}
func verifyManifest(raw, sig []byte, key string) (UpdateManifest, error) {
	var m UpdateManifest
	pub, e := base64.StdEncoding.DecodeString(key)
	if e != nil || len(pub) != ed25519.PublicKeySize {
		return m, errors.New("此构建未配置更新验签公钥，请安装正式签名版本")
	}
	s, e := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if e != nil || !ed25519.Verify(pub, raw, s) {
		return m, errors.New("更新清单签名验证失败")
	}
	if e = json.Unmarshal(raw, &m); e != nil {
		return m, e
	}
	if _, e = versionParts(m.Version); e != nil {
		return m, e
	}
	if _, e = versionParts(m.MinVersion); e != nil {
		return m, e
	}
	if m.Protocol != updateProtocol || len(m.Assets) != 2 {
		return m, errors.New("更新协议不兼容，请更新宿主 JS")
	}
	seen := map[string]bool{}
	for _, a := range m.Assets {
		hash, e := hex.DecodeString(a.SHA256)
		if e != nil || len(hash) != 32 || a.Size < 1 || a.Size > maxUpdatePackage || !contains([]string{"arm32", "arm64"}, a.Arch) || seen[a.Arch] || a.Name != "UFI-Gate-"+m.Version+"-"+a.Arch+"-update.tar.gz" {
			return m, errors.New("更新附件清单无效")
		}
		seen[a.Arch] = true
	}
	return m, nil
}
func (g *Gate) checkRelease() ReleaseInfo {
	info := ReleaseInfo{Checked: time.Now().Unix()}
	ctx, cancel := context.WithTimeout(g.ctx, 40*time.Second)
	defer cancel()
	c := g.updateClient()
	var release struct {
		Tag        string `json:"tag_name"`
		Body       string `json:"body"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	raw, e := fetchUpdate(ctx, c, "https://api.github.com/repos/"+releaseRepo+"/releases/latest", 512<<10)
	if e == nil {
		e = json.Unmarshal(raw, &release)
	}
	if e == nil {
		info.Version = strings.TrimPrefix(release.Tag, "v")
		_, e = versionParts(info.Version)
	}
	if e == nil && (release.Draft || release.Prerelease) {
		e = errors.New("不是正式发布版本")
	}
	if e == nil {
		info.URL = "https://github.com/" + releaseRepo + "/releases/tag/v" + info.Version
		info.Notes = release.Body
		base := "https://github.com/" + releaseRepo + "/releases/download/v" + info.Version + "/"
		var manifest, signature []byte
		manifest, e = fetchUpdate(ctx, c, base+"update.json", 64<<10)
		if e == nil {
			signature, e = fetchUpdate(ctx, c, base+"update.json.sig", 1024)
		}
		if e == nil {
			var m UpdateManifest
			m, e = verifyManifest(manifest, signature, updatePublicKey)
			if e == nil && m.Version != info.Version {
				e = errors.New("发布版本与签名清单不一致")
			}
			if e == nil {
				info.Manifest = &m
			}
		}
	}
	if e != nil {
		info.Error = e.Error()
	}
	return info
}
func (g *Gate) beginReleaseCheck() error {
	if g.updateChecking {
		return errors.New("正在检查更新")
	}
	if time.Now().Unix()-g.release.Checked < 30 {
		return errors.New("请稍后再检查，避免频繁请求 GitHub")
	}
	g.updateChecking = true
	go func() {
		info := g.checkRelease()
		g.mu.Lock()
		defer g.mu.Unlock()
		g.release = info
		g.updateChecking = false
		raw, _ := json.Marshal(info)
		_ = atomicWrite(filepath.Join(dir, "release-cache.json"), raw)
		if info.Error != "" {
			g.notify("update_failed", "版本检查未完成："+info.Error, "updates", "check:"+info.Error, true)
		}
		if cmp, e := compareVersions(info.Version, currentVersion()); e == nil && cmp > 0 && g.db.LastReleaseNotice != info.Version {
			g.db.LastReleaseNotice = info.Version
			g.notify("update_available", "发现新版本 "+info.Version, "updates", "release:"+info.Version, true)
			_ = g.save()
		}
		_ = g.save()
	}()
	return nil
}
func acquireUpdateLock() (*os.File, error) {
	f, e := os.OpenFile(filepath.Join(dir, "update.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = lockFile(f); e != nil {
		f.Close()
		return nil, errors.New("另一个安装或更新任务正在运行")
	}
	return f, nil
}
func (g *Gate) beginUpdate(target string) error {
	if _, e := os.Stat(filepath.Join(dir, "update-journal.json")); e == nil {
		return errors.New("存在中断的更新，请在宿主菜单启动服务以恢复")
	}
	if demo || !contains([]string{"arm32", "arm64"}, buildArch()) {
		return errors.New("当前环境只支持检查，不执行设备更新")
	}
	if g.updatePreparing {
		return errors.New("更新已在进行")
	}
	if g.release.Manifest == nil || g.release.Error != "" || target != g.release.Version {
		return errors.New("请先成功检查并验证更新，再确认版本")
	}
	if cmp, e := compareVersions(target, currentVersion()); e != nil || cmp <= 0 {
		return errors.New("目标版本必须高于当前版本")
	}
	m := *g.release.Manifest
	if cmp, _ := compareVersions(currentVersion(), m.MinVersion); cmp < 0 {
		return errors.New("当前版本不满足升级要求，请更新宿主 JS")
	}
	lock, e := acquireUpdateLock()
	if e != nil {
		return e
	}
	g.updatePreparing = true
	job := UpdateStatus{ID: random(8), Version: target, Phase: "downloading", Message: "正在下载并校验更新包"}
	if e = saveUpdateStatus(job); e != nil {
		lock.Close()
		g.updatePreparing = false
		return e
	}
	go func() {
		defer lock.Close()
		defer func() { g.mu.Lock(); g.updatePreparing = false; g.mu.Unlock() }()
		err := g.downloadAndLaunch(m, job, lock)
		if err != nil {
			job.Phase = "failed"
			job.Message = err.Error()
			_ = saveUpdateStatus(job)
			g.mu.Lock()
			g.notify("update_failed", "更新失败："+err.Error(), "updates", "update:"+job.ID, true)
			_ = g.save()
			g.mu.Unlock()
		}
	}()
	return nil
}
func (g *Gate) downloadAndLaunch(m UpdateManifest, job UpdateStatus, lock *os.File) error {
	var asset UpdateAsset
	for _, a := range m.Assets {
		if a.Arch == buildArch() {
			asset = a
		}
	}
	if asset.Name == "" {
		return errors.New("没有匹配架构的更新包")
	}
	ctx, cancel := context.WithTimeout(g.ctx, 6*time.Minute)
	defer cancel()
	data, e := fetchUpdate(ctx, g.updateClient(), "https://github.com/"+releaseRepo+"/releases/download/v"+m.Version+"/"+asset.Name, asset.Size)
	if e != nil {
		return e
	}
	sum := sha256.Sum256(data)
	if int64(len(data)) != asset.Size || hex.EncodeToString(sum[:]) != asset.SHA256 {
		return errors.New("更新包大小或 SHA-256 校验失败")
	}
	stage := filepath.Join(dir, "update-stage-"+job.ID)
	if e = os.Mkdir(stage, 0700); e != nil {
		return e
	}
	if e = extractUpdate(data, stage); e != nil {
		_ = os.RemoveAll(stage)
		return e
	}
	if e = validateStaged(stage, m.Version); e != nil {
		_ = os.RemoveAll(stage)
		return e
	}
	job.Phase = "verified"
	job.Message = "验证通过，即将停止服务并安装"
	if e = saveUpdateStatus(job); e != nil {
		return e
	}
	return launchUpdater(stage, job, lock)
}

// Called under g.mu after normal administrator authentication and same-origin validation.
func (g *Gate) updateAPI(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/admin-api/update") {
		return false
	}
	if r.Method == "GET" && r.URL.Path == "/admin-api/update" {
		reply(w, 200, map[string]any{"current": currentVersion(), "arch": buildArch(), "release": g.release, "checking": g.updateChecking, "status": readUpdateStatus(), "autoCheck": g.db.Config.AutoUpdateCheck, "signatureReady": updatePublicKey != ""})
		return true
	}
	var e error
	if r.Method != "POST" {
		http.Error(w, "方法不支持", 405)
		return true
	}
	switch r.URL.Path {
	case "/admin-api/update/check":
		e = g.beginReleaseCheck()
	case "/admin-api/update/install":
		var v struct{ Version string }
		e = body(w, r, &v)
		if e == nil {
			e = g.beginUpdate(v.Version)
		}
	case "/admin-api/update/settings":
		var v struct{ AutoCheck bool }
		e = body(w, r, &v)
		if e == nil {
			g.db.Config.AutoUpdateCheck = v.AutoCheck
			e = g.save()
		}
	default:
		e = errors.New("更新接口不存在")
	}
	if e != nil {
		fail(w, e)
	} else {
		reply(w, 200, map[string]bool{"ok": true})
	}
	return true
}
