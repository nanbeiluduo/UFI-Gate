/* Browser-only mock of UFI-Gate. Nothing here touches a network, firewall, or device. */
(function (root, factory) {
  const api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  root.UFIDemo = api;
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  const KEY = 'ufi-gate-demo-state-v1';
  const MiB = 1048576;
  const RECOVERY_KEY = 'DEMO-RECOVERY-KEY';
  const PRESETS = {
    pc: { id: 'pc', MAC: '3c:22:fb:10:8a:01', IP: '192.168.0.10', Name: '管理电脑' },
    phone: { id: 'phone', MAC: '48:2c:a0:21:6c:02', IP: '192.168.0.21', Name: '我的手机' },
    tablet: { id: 'tablet', MAC: '7a:15:9e:33:11:03', IP: '192.168.0.33', Name: '客厅平板' }
  };

  function memoryStorage() {
    const m = new Map();
    return {
      getItem: (k) => (m.has(k) ? m.get(k) : null),
      setItem: (k, v) => m.set(k, String(v)),
      removeItem: (k) => m.delete(k)
    };
  }
  const storage = typeof localStorage !== 'undefined' ? localStorage : memoryStorage();

  function now() { return Math.floor(Date.now() / 1000); }
  function uid() { return 'r' + Date.now().toString(36) + Math.random().toString(36).slice(2, 6); }
  function clone(v) { return structuredClone(v); }
  function quota(partial) {
    return Object.assign({
      Daily: 512 * MiB, Total: 0, Session: 0, Period: 0,
      PeriodDays: 30, Action: 'block', RateKBps: 128, SlowPercent: 80
    }, partial || {});
  }
  function blankDevice(meta, q) {
    const t = now();
    return {
      id: meta.id, MAC: meta.MAC, IP: meta.IP, Name: meta.Name, Note: '',
      Trusted: false, Banned: false, Until: 0, ManualRateKBps: 0,
      Up: 0, Down: 0, DayUp: 0, DayDown: 0, SessionBytes: 0, PeriodBytes: 0, MonthBytes: 0,
      DailyBase: 0, TotalBase: 0, SessionBase: 0, PeriodBase: 0,
      LimitReason: '', FirstSeen: t, LastSeen: t, RateUp: 0, RateDown: 0,
      connected: false, ui: 'wifi', browse: null, captcha: null,
      Quota: quota(q)
    };
  }
  function seedRates() {
    const pts = [];
    for (let i = 0; i < 40; i++) {
      pts.push({
        Up: Math.round(6000 + Math.cos(i / 4) * 2500 + (i % 4) * 400),
        Down: Math.round(70000 + Math.sin(i / 3.2) * 28000 + (i % 6) * 1800)
      });
    }
    return pts;
  }
  function seed() {
    const q = quota();
    const pc = blankDevice(PRESETS.pc, q);
    pc.Note = '日常管理';
    pc.Trusted = true;
    pc.connected = true;
    pc.ui = 'portal';
    pc.Up = 120 * MiB;
    pc.Down = 980 * MiB;
    pc.DayUp = 6 * MiB;
    pc.DayDown = 70 * MiB;
    pc.MonthBytes = 240 * MiB;
    const phone = blankDevice(PRESETS.phone, q);
    const tablet = blankDevice(PRESETS.tablet, q);
    tablet.connected = true;
    tablet.ui = 'portal';
    tablet.Note = '客厅';
    const rates = seedRates();
    const last = rates[rates.length - 1];
    pc.RateUp = last.Up;
    pc.RateDown = last.Down;
    const t = now();
    return {
      version: 1,
      startedAt: Date.now() - (5 * 3600 + 26 * 60) * 1000,
      loggedIn: true,
      admin: { Name: 'admin', Password: 'Admin1234' },
      recoveryKey: RECOVERY_KEY,
      config: {
        Enabled: true,
        Verified: true,
        Methods: ['account', 'captcha', 'approval'],
        SessionHours: 12,
        CaptchaProvider: 'hcaptcha',
        PortalHost: '',
        CaptchaSite: '',
        CaptchaSecret: '',
        CaptchaVerify: '',
        CaptchaScript: '',
        Title: '连接，从这里开始',
        Subtitle: '欢迎使用共享网络。完成认证，即可安心上网。',
        Contact: '请联系网络管理员',
        Warning: '流量已达到管理员设置的上限，请联系管理员解除限制。',
        BlockWarning: '该网站已被管理员禁止访问。',
        Accent: '#0067c0',
        Black: ['ads.example.com'],
        White: ['captive.apple.com', '*.msftconnecttest.com'],
        LAN: 'br0',
        Gateway: '192.168.0.1',
        AdminMACs: [PRESETS.pc.MAC],
        DefaultQuota: q,
        Push: {
          Kind: 'wecom',
          URL: 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=demo',
          Secret: '',
          PublicBase: '',
          SMTPHost: '', SMTPPort: 587, SMTPUser: '', SMTPPassword: '', SMTPFrom: '', SMTPTo: ''
        },
        RemoteApproval: { Enabled: false, PublicURL: '', Port: 8091 }
      },
      accounts: [{ Name: 'guest', Password: 'wifi2026', Expires: 0 }],
      devices: { pc: pc, phone: phone, tablet: tablet },
      requests: [],
      messages: [],
      logs: [{ Time: t - 120, Kind: 'system', Message: '演示环境已就绪。这里不会改动真实网络。' }],
      rates: rates,
      blockedPackets: 18,
      blackPackets: 3
    };
  }

  function notify() {
    if (typeof window === 'undefined') return;
    window.dispatchEvent(new CustomEvent('ufi-demo-change'));
    const msg = { type: 'ufi-demo-state' };
    const origin = window.location.origin;
    try {
      if (window.parent && window.parent !== window) window.parent.postMessage(msg, origin);
    } catch (e) { /* ignore */ }
    document.querySelectorAll('iframe').forEach((frame) => {
      try { if (frame.contentWindow) frame.contentWindow.postMessage(msg, origin); } catch (e) { /* ignore */ }
    });
  }
  function load() {
    try {
      const raw = storage.getItem(KEY);
      if (!raw) return seed();
      const data = JSON.parse(raw);
      if (!data || !data.config || !data.devices) return seed();
      return data;
    } catch (e) {
      return seed();
    }
  }
  function save(state) {
    storage.setItem(KEY, JSON.stringify(state));
    notify();
  }
  function update(mutator) {
    const draft = clone(load());
    const out = mutator(draft) || {};
    if (out.error) return { ok: false, error: out.error, state: load() };
    save(draft);
    return { ok: true, error: '', state: draft, detail: out };
  }
  function reset() {
    const fresh = seed();
    save(fresh);
    return fresh;
  }

  function log(state, kind, message) {
    state.logs.push({ Time: now(), Kind: kind, Message: message });
    if (state.logs.length > 500) state.logs.splice(0, state.logs.length - 500);
  }
  function used(n, b) { return n < b ? 0 : n - b; }
  function ratio(d) {
    const q = d.Quota || quota();
    const vals = [
      [used(d.DayUp + d.DayDown, d.DailyBase || 0), q.Daily],
      [used(d.Up + d.Down, d.TotalBase || 0), q.Total],
      [used(d.SessionBytes || 0, d.SessionBase || 0), q.Session],
      [used(d.PeriodBytes || 0, d.PeriodBase || 0), q.Period]
    ];
    let max = 0;
    vals.forEach(([n, den]) => { if (den > 0) max = Math.max(max, n / den); });
    return max;
  }
  function deviceState(d) {
    if (!d) return 'pending';
    if (d.Banned) return 'blocked';
    if (d.LimitReason) return d.Quota && d.Quota.Action === 'slow' ? 'limited' : 'quota';
    if (d.Trusted || d.Until > now()) return d.ManualRateKBps > 0 ? 'limited' : 'online';
    return 'pending';
  }
  function allowed(d) {
    const s = deviceState(d);
    return s === 'online' || s === 'limited';
  }
  function effectiveRate(d) {
    if (!allowed(d)) return 0;
    let rate = d.ManualRateKBps || 0;
    const quotaLimited = (d.LimitReason && d.Quota.Action === 'slow') || (d.Quota.SlowPercent > 0 && ratio(d) >= d.Quota.SlowPercent / 100);
    if (quotaLimited && (rate === 0 || d.Quota.RateKBps < rate)) rate = d.Quota.RateKBps;
    return rate;
  }
  function isAdmin(state, mac) {
    return (state.config.AdminMACs || []).includes(String(mac || '').toLowerCase());
  }
  function byMac(state, mac) {
    mac = String(mac || '').toLowerCase();
    return Object.values(state.devices).find((d) => d.MAC === mac) || null;
  }
  function byId(state, id) { return state.devices[id] || null; }
  function pendingRequest(state, mac) {
    const t = now();
    return state.requests.find((r) => r.MAC === mac && r.Status === 'pending' && r.Expires > t) || null;
  }
  function portalKind(state, d) {
    if (!d) return 'login';
    if (!state.config.Enabled) return 'disabled';
    const st = deviceState(d);
    if (st === 'blocked') return 'blocked';
    if (st === 'quota') return 'quota';
    if (st === 'pending' && pendingRequest(state, d.MAC)) return 'waiting';
    if (st === 'online') return 'success';
    if (st === 'limited') return 'limited';
    return 'login';
  }
  function strong(p) {
    return typeof p === 'string' && p.length >= 8 && p.length <= 72 && /[A-Za-z]/.test(p) && /[0-9]/.test(p);
  }
  function validMAC(mac) {
    return /^[0-9a-f]{2}(:[0-9a-f]{2}){5}$/.test(String(mac || '').toLowerCase());
  }
  function validIPv4(ip) {
    const m = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(String(ip || '').trim());
    return !!m && m.slice(1).every((n) => Number(n) <= 255);
  }
  function normalizeHost(raw) {
    let h = String(raw || '').trim().toLowerCase();
    h = h.replace(/^[a-z]+:\/\//, '').split('/')[0].split('?')[0].split('#')[0];
    h = h.replace(/:\d+$/, '');
    return h;
  }
  function matchRule(host, rule) {
    rule = String(rule || '').trim().toLowerCase();
    if (!rule || !host) return false;
    if (rule.startsWith('*.')) {
      const rest = rule.slice(2);
      return host === rest || host.endsWith('.' + rest);
    }
    return host === rule;
  }
  function classifyHost(state, d, raw) {
    const host = normalizeHost(raw);
    if (!host || !/^[a-z0-9.-]+$/.test(host)) return { host: host, kind: 'invalid' };
    if (!d || !d.connected) return { host: host, kind: 'offline' };
    const st = deviceState(d);
    if (st === 'blocked') return { host: host, kind: 'blocked' };
    if (st === 'quota') return { host: host, kind: 'quota' };
    if ((state.config.Black || []).some((rule) => matchRule(host, rule))) return { host: host, kind: 'black' };
    if (!allowed(d)) {
      if ((state.config.White || []).some((rule) => matchRule(host, rule))) return { host: host, kind: 'white' };
      return { host: host, kind: 'portal' };
    }
    return { host: host, kind: 'open' };
  }
  function validateQuota(q) {
    ['Daily', 'Total', 'Session', 'Period'].forEach((k) => { q[k] = Math.round(Number(q[k])); });
    q.PeriodDays = Math.round(Number(q.PeriodDays));
    q.RateKBps = Math.round(Number(q.RateKBps));
    q.SlowPercent = Math.round(Number(q.SlowPercent));
    if (['Daily', 'Total', 'Session', 'Period'].some((k) => !Number.isFinite(q[k]) || q[k] < 0)) return '流量配额须为非负数';
    if (q.PeriodDays < 1 || q.PeriodDays > 365) return '周期长度须为 1–365 天';
    if (q.RateKBps < 1 || q.RateKBps > 102400) return '限速须为 1–102400 KiB/s';
    if (q.SlowPercent < 0 || q.SlowPercent > 100) return '提前限速阈值须为 0–100';
    if (q.Action !== 'block' && q.Action !== 'slow') return '达到上限后的动作无效';
    return '';
  }
  function pushMessage(state, msg) {
    state.messages.unshift(Object.assign({ id: uid(), time: now(), unread: true, requestId: '' }, msg));
    if (state.messages.length > 40) state.messages.length = 40;
  }
  function channelName(kind) {
    return { wecom: '企业微信', feishu: '飞书', webhook: 'Webhook', smtp: '邮件' }[kind] || '本地预览';
  }
  function grant(state, d, expires) {
    if (d.Banned || d.LimitReason) return '设备已被限制，请联系管理员';
    if (allowed(d)) {
      d.LastSeen = now();
      return '';
    }
    let until = now() + state.config.SessionHours * 3600;
    if (expires && expires < until) until = expires;
    if (expires && expires <= now()) return '上网账号已过期';
    d.Until = until;
    d.SessionBytes = 0;
    d.SessionBase = 0;
    d.LastSeen = now();
    d.ui = 'portal';
    return '';
  }
  function latchQuota(state, d) {
    if (!d.LimitReason && ratio(d) >= 1) {
      d.LimitReason = 'quota';
      log(state, 'quota', d.MAC + ' 达到流量配额');
      return true;
    }
    return false;
  }
  function bytesUntilFull(d) {
    const q = d.Quota;
    const pairs = [
      [q.Daily, used(d.DayUp + d.DayDown, d.DailyBase || 0)],
      [q.Total, used(d.Up + d.Down, d.TotalBase || 0)],
      [q.Session, used(d.SessionBytes || 0, d.SessionBase || 0)],
      [q.Period, used(d.PeriodBytes || 0, d.PeriodBase || 0)]
    ];
    let any = false;
    let need = 0;
    pairs.forEach(([den, n]) => {
      if (den > 0) { any = true; need = Math.max(need, den - n); }
    });
    if (!any) return -1;
    return Math.max(0, need);
  }

  function connect(id) {
    return update((s) => {
      let d = s.devices[id];
      if (!d) {
        if (!PRESETS[id]) return { error: '未知设备' };
        d = blankDevice(PRESETS[id], s.config.DefaultQuota);
        s.devices[id] = d;
      }
      d.connected = true;
      d.LastSeen = now();
      d.ui = 'portal';
      log(s, 'network', d.Name + ' 已关联热点');
    });
  }
  function disconnect(id) {
    return update((s) => {
      const d = s.devices[id];
      if (!d) return { error: '设备不在记录中' };
      d.connected = false;
      d.ui = 'wifi';
      log(s, 'network', d.Name + ' 已断开热点');
    });
  }
  function setUI(id, ui) {
    return update((s) => {
      const d = s.devices[id];
      if (!d) return { error: '设备不在记录中' };
      d.ui = ui;
    });
  }
  function ensureCaptcha(id) {
    const cur = load();
    const d0 = cur.devices[id];
    if (!d0) return { ok: false, error: '设备不在记录中' };
    if (d0.captcha && d0.captcha.exp > Date.now() && d0.captcha.code) return { ok: true, changed: false, code: d0.captcha.code };
    return update((s) => {
      const d = s.devices[id];
      const alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789';
      let code = '';
      for (let i = 0; i < 4; i++) code += alphabet[Math.floor(Math.random() * alphabet.length)];
      d.captcha = { code: code, exp: Date.now() + 120000 };
      return { code: code, changed: true };
    });
  }
  function authAccount(id, user, pass) {
    return update((s) => {
      const d = s.devices[id];
      if (!d || !d.connected) return { error: '请先连接热点' };
      if (!s.config.Enabled) return { error: '认证未开启' };
      if (!s.config.Methods.includes('account')) return { error: '未开启账号认证' };
      const acc = s.accounts.find((a) => a.Name === String(user || '').trim());
      if (!acc || acc.Password !== pass) return { error: '账号或密码不正确' };
      if (acc.Expires && acc.Expires <= now()) return { error: '上网账号已过期' };
      const err = grant(s, d, acc.Expires || 0);
      if (err) return { error: err };
      log(s, 'auth', d.MAC + ' 通过账号认证');
    });
  }
  function authCaptcha(id, answer) {
    return update((s) => {
      const d = s.devices[id];
      if (!d || !d.connected) return { error: '请先连接热点' };
      if (!s.config.Methods.includes('captcha')) return { error: '未开启图形验证码' };
      const cap = d.captcha;
      d.captcha = null;
      if (!cap || cap.exp < Date.now() || String(answer || '').trim().toUpperCase() !== cap.code) {
        return { error: '验证码不正确或已过期' };
      }
      const err = grant(s, d, 0);
      if (err) return { error: err };
      log(s, 'auth', d.MAC + ' 通过图形验证码');
    });
  }
  function authThird(id, ok) {
    return update((s) => {
      const d = s.devices[id];
      if (!d || !d.connected) return { error: '请先连接热点' };
      if (!s.config.Methods.includes('thirdparty')) return { error: '未开启第三方验证' };
      if (!ok) return { error: '请先完成人机验证' };
      const err = grant(s, d, 0);
      if (err) return { error: err };
      log(s, 'auth', d.MAC + ' 通过演示人机验证');
    });
  }
  function requestApproval(id, name, reason) {
    return update((s) => {
      const d = s.devices[id];
      if (!d || !d.connected) return { error: '请先连接热点' };
      if (!s.config.Enabled) return { error: '认证未开启' };
      if (!s.config.Methods.includes('approval')) return { error: '未开启审批申请' };
      name = String(name || '').trim();
      reason = String(reason || '').trim();
      if (!name) return { error: '请填写称呼' };
      if (name.length > 100 || reason.length > 1000) return { error: '内容过长' };
      if (pendingRequest(s, d.MAC)) return {};
      const req = {
        ID: uid(), MAC: d.MAC, Name: name, Reason: reason,
        Status: 'pending', Created: now(), Expires: now() + 1800, DecisionReason: ''
      };
      s.requests.unshift(req);
      d.ui = 'portal';
      const kind = s.config.Push.Kind;
      if (kind) {
        pushMessage(s, {
          channel: kind,
          title: '新的上网申请',
          body: name + '（' + d.Name + ' / ' + d.MAC + '）请求上网。' + (reason || '未填写理由') + '。演示不会向外网发送，只显示在这里和审批中心。',
          requestId: req.ID
        });
        log(s, 'push', '已生成' + channelName(kind) + '审批预览');
      } else {
        log(s, 'push', '收到上网申请，推送渠道未开启');
      }
      log(s, 'auth', d.MAC + ' 提交上网申请');
    });
  }
  function decide(id, action, reason) {
    return update((s) => {
      const req = s.requests.find((r) => r.ID === id);
      if (!req || req.Status !== 'pending') return { error: '申请已处理或不存在' };
      if (req.Expires <= now()) { req.Status = 'expired'; return { notice: '申请已过期' }; }
      reason = String(reason || '').trim();
      if (reason.length > 1000) return { error: '备注过长' };
      req.DecisionReason = reason;
      const d = byMac(s, req.MAC);
      if (action === 'allow') {
        if (!d) return { error: '设备记录已删除' };
        const err = grant(s, d, 0);
        if (err) return { error: err };
        req.Status = 'allow';
        if (d.ui === 'wifi') d.ui = 'portal';
        log(s, 'auth', req.MAC + ' 审批通过');
      } else {
        req.Status = 'deny';
        log(s, 'auth', req.MAC + ' 审批拒绝');
      }
      s.messages.forEach((m) => { if (m.requestId === id) m.unread = false; });
    });
  }
  function renewCaptcha(id) {
    return update((s) => {
      const d = s.devices[id];
      if (!d) return { error: '设备不在记录中' };
      d.captcha = null;
    });
  }
  function canPass(state, d) {
    if (!d.connected) return false;
    if (!state.config.Enabled) return true;
    return allowed(d);
  }
  function addTraffic(id, downBytes) {
    return update((s) => {
      const d = s.devices[id];
      if (!d) return { error: '设备不在记录中' };
      if (!d.connected) return { error: '设备未连接热点，流量不会发生' };
      downBytes = Math.round(Number(downBytes));
      if (!Number.isFinite(downBytes) || downBytes <= 0) return { error: '流量数值无效' };
      if (!canPass(s, d)) {
        s.blockedPackets += 1;
        log(s, 'network', d.MAC + ' 的流量被准入策略丢弃');
        return { dropped: true };
      }
      const up = Math.max(1, Math.round(downBytes * 0.08));
      d.Down += downBytes;
      d.DayDown += downBytes;
      d.Up += up;
      d.DayUp += up;
      d.SessionBytes += downBytes + up;
      d.PeriodBytes += downBytes + up;
      d.MonthBytes += downBytes + up;
      d.RateDown = Math.round(downBytes / 5);
      d.RateUp = Math.round(up / 5);
      d.LastSeen = now();
      s.rates.push({ Up: d.RateUp, Down: d.RateDown });
      if (s.rates.length > 60) s.rates.splice(0, s.rates.length - 60);
      latchQuota(s, d);
      log(s, 'traffic', d.Name + ' 模拟下行 ' + Math.round(downBytes / MiB) + ' MiB');
    });
  }
  function fillQuota(id) {
    const cur = load();
    const d0 = cur.devices[id];
    if (!d0) return { ok: false, error: '设备不在记录中' };
    const need = bytesUntilFull(d0);
    if (need < 0) return { ok: false, error: '这台设备没有配额上限。可在服务设置或设备卡片里填写。' };
    return addTraffic(id, need + MiB);
  }
  function visit(id, raw) {
    return update((s) => {
      const d = s.devices[id];
      if (!d) return { error: '设备不在记录中' };
      const result = classifyHost(s, d, raw);
      if (result.kind === 'invalid') return { error: '请输入域名或 IPv4，例如 example.com' };
      if (result.kind === 'offline') return { error: '请先连接热点' };
      d.browse = result;
      if (result.kind === 'portal') d.ui = 'portal';
      else d.ui = 'browser';
      if (result.kind === 'black') {
        s.blackPackets += 1;
        log(s, 'policy', result.host + ' 命中黑名单');
      } else if (result.kind === 'white') {
        log(s, 'policy', result.host + ' 白名单免认证放行');
      } else if (result.kind === 'portal' || result.kind === 'blocked' || result.kind === 'quota') {
        s.blockedPackets += 1;
      }
      return { kind: result.kind, host: result.host };
    });
  }
  function deviceAction(mac, action, payload) {
    return update((s) => {
      payload = payload || {};
      mac = String(mac || '').toLowerCase();
      if (action === 'manual') {
        if (!validMAC(mac)) return { error: 'MAC 地址无效' };
        if (byMac(s, mac)) return { error: '这台设备已经在列表中' };
        const id = 'm' + mac.replace(/:/g, '').slice(-6);
        const d = blankDevice({ id: id, MAC: mac, IP: '192.168.0.50', Name: '手动录入' }, s.config.DefaultQuota);
        d.connected = true;
        d.ui = 'portal';
        s.devices[id] = d;
        if (payload.op === 'trust') { d.Trusted = true; d.Banned = false; }
        else if (payload.op === 'block') { d.Banned = true; d.Until = 0; }
        else if (payload.op === 'allow') {
          const err = grant(s, d, 0);
          if (err) return { error: err };
        }
        log(s, 'device', mac + ' 手动录入');
        return {};
      }
      const d = byMac(s, mac);
      if (!d && action !== 'manual') return { error: '找不到这台设备' };
      if (action === 'allow') {
        const err = grant(s, d, 0);
        if (err) return { error: err };
      } else if (action === 'block') {
        d.Banned = true;
        d.Until = 0;
      } else if (action === 'revoke') {
        d.Until = 0;
        d.Trusted = false;
      } else if (action === 'trust') {
        d.Trusted = true;
        d.Banned = false;
      } else if (action === 'untrust') {
        d.Trusted = false;
      } else if (action === 'release') {
        d.Banned = false;
        d.LimitReason = '';
        d.DailyBase = d.DayUp + d.DayDown;
        d.TotalBase = d.Up + d.Down;
        d.SessionBase = d.SessionBytes;
        d.PeriodBase = d.PeriodBytes;
        d.Until = now() + s.config.SessionHours * 3600;
      } else if (action === 'reset') {
        d.Up = d.Down = d.DayUp = d.DayDown = d.SessionBytes = d.PeriodBytes = d.MonthBytes = 0;
        d.DailyBase = d.TotalBase = d.SessionBase = d.PeriodBase = 0;
        d.LimitReason = '';
        d.RateUp = d.RateDown = 0;
      } else if (action === 'delete') {
        delete s.devices[d.id];
      } else if (action === 'rate') {
        const rate = Math.round(Number(payload.rate));
        if (!Number.isInteger(rate) || rate < 1 || rate > 102400) return { error: '请输入 1–102400 的整数' };
        d.ManualRateKBps = rate;
      } else if (action === 'rate-clear') {
        d.ManualRateKBps = 0;
      } else if (action === 'edit') {
        const note = String(payload.note || '');
        if (note.length > 100) return { error: '备注过长' };
        const q = clone(payload.quota);
        const err = validateQuota(q);
        if (err) return { error: err };
        d.Note = note;
        d.Quota = q;
        latchQuota(s, d);
      } else if (action === 'admin-add' || action === 'admin-remove') {
        const list = s.config.AdminMACs.map((x) => x.toLowerCase());
        if (action === 'admin-add') {
          if (!list.includes(d.MAC)) list.push(d.MAC);
        } else {
          if (list.length <= 1 && list.includes(d.MAC)) return { error: '至少保留一台管理员设备' };
          const next = list.filter((x) => x !== d.MAC);
          list.length = 0;
          next.forEach((x) => list.push(x));
        }
        s.config.AdminMACs = list;
      } else {
        return { error: '未知设备操作' };
      }
      log(s, 'device', mac + ' ' + action);
    });
  }
  function markAdmin(id) {
    return update((s) => {
      const d = s.devices[id];
      if (!d) return { error: '设备不在记录中' };
      if (!s.config.AdminMACs.includes(d.MAC)) s.config.AdminMACs.push(d.MAC);
      log(s, 'admin', d.MAC + ' 通过门户获得管理设备权限');
    });
  }
  function login(user, pass) {
    return update((s) => {
      if (String(user || '') !== s.admin.Name || pass !== s.admin.Password) return { error: '账号或密码不正确' };
      s.loggedIn = true;
      log(s, 'admin', '管理员登录');
    });
  }
  function logout() {
    return update((s) => { s.loggedIn = false; });
  }
  function changePassword(user, pass) {
    return update((s) => {
      user = String(user || '').trim();
      if (!user || user.length > 64) return { error: '管理员账号无效' };
      if (!strong(pass)) return { error: '密码须至少 8 位，且包含字母和数字' };
      s.admin.Name = user;
      s.admin.Password = pass;
      log(s, 'admin', '已修改管理员账号');
    });
  }
  function recover(key, pass) {
    return update((s) => {
      if (String(key || '').trim() !== s.recoveryKey) return { error: '恢复密钥不正确' };
      if (!strong(pass)) return { error: '新密码须至少 8 位，且包含字母和数字' };
      s.admin.Password = pass;
      s.loggedIn = false;
      log(s, 'admin', '已用恢复密钥重置密码');
    });
  }
  function toggleAuth(enable) {
    return update((s) => {
      s.config.Enabled = !!enable;
      if (enable) s.config.Verified = true;
      log(s, 'network', '认证服务启用=' + (!!enable));
    });
  }
  function addAccount(name, pass, expires) {
    return update((s) => {
      name = String(name || '').trim();
      if (!name || name.length > 64) return { error: '账号名称无效' };
      if (s.accounts.some((a) => a.Name === name)) return { error: '账号已存在' };
      if (!strong(pass)) return { error: '密码须至少 8 位，且包含字母和数字' };
      s.accounts.push({ Name: name, Password: pass, Expires: expires || 0 });
      log(s, 'account', '新建上网账号 ' + name);
    });
  }
  function deleteAccount(name) {
    return update((s) => {
      const before = s.accounts.length;
      s.accounts = s.accounts.filter((a) => a.Name !== name);
      if (s.accounts.length === before) return { error: '账号不存在' };
      log(s, 'account', '删除上网账号 ' + name);
    });
  }
  function prune() {
    return update((s) => {
      s.requests = s.requests.filter((r) => r.Status === 'pending' && r.Expires > now());
      log(s, 'auth', '已清理结束的审批记录');
    });
  }
  function pushTest() {
    return update((s) => {
      const kind = s.config.Push.Kind;
      pushMessage(s, {
        channel: kind || 'demo',
        title: '测试推送',
        body: kind
          ? '这是一条测试消息，渠道显示为' + channelName(kind) + '。演示不会访问外部服务器。'
          : '推送渠道未开启。这条消息只出现在演示收件箱。'
      });
      log(s, 'push', '已生成测试推送');
    });
  }
  function remoteTest() {
    return update((s) => {
      if (!s.config.RemoteApproval.Enabled) return { error: '请先启用远程审批并保存' };
      pushMessage(s, {
        channel: 'remote',
        title: '远程审批测试',
        body: '测试链接只在演示里生效，不会给任何设备放行。公网地址：' + (s.config.RemoteApproval.PublicURL || '（未填写）')
      });
      log(s, 'push', '已生成远程审批测试');
    });
  }
  function readLines(text) {
    return String(text || '').split('\n').map((s) => s.trim().toLowerCase()).filter(Boolean);
  }
  function saveSection(section, values) {
    return update((s) => {
      const c = s.config;
      if (section === 'auth') {
        const methods = (values.Methods || []).filter((m) => ['account', 'captcha', 'approval', 'thirdparty'].includes(m));
        if (!methods.length) return { error: '至少开启一种认证方式' };
        const hours = Math.round(Number(values.SessionHours));
        if (hours < 1 || hours > 720) return { error: '认证有效期须为 1–720 小时' };
        c.Methods = methods;
        c.SessionHours = hours;
        c.CaptchaProvider = values.CaptchaProvider || 'hcaptcha';
        c.PortalHost = String(values.PortalHost || '').trim();
        c.CaptchaSite = String(values.CaptchaSite || '');
        if (values.CaptchaSecret) c.CaptchaSecret = String(values.CaptchaSecret);
        c.CaptchaVerify = String(values.CaptchaVerify || '').trim();
        c.CaptchaScript = String(values.CaptchaScript || '').trim();
        if (methods.includes('thirdparty') && !c.CaptchaSite) return { error: '第三方验证需要填写 Site Key / AppID' };
      } else if (section === 'policy') {
        c.Black = readLines(values.Black);
        c.White = readLines(values.White);
      } else if (section === 'appearance') {
        const accent = String(values.Accent || '').trim();
        if (!/^#[0-9a-fA-F]{6}$/.test(accent)) return { error: '主题色须为 #RRGGBB' };
        const fields = { Title: 120, Subtitle: 500, Contact: 500, Warning: 1000, BlockWarning: 1000 };
        for (const [k, max] of Object.entries(fields)) {
          const v = String(values[k] || '');
          if (v.length > max) return { error: '文案过长' };
          c[k] = v;
        }
        c.Accent = accent;
      } else if (section === 'push') {
        const push = Object.assign({}, c.Push, values.Push || {});
        if (!values.Push || !values.Push.Secret) push.Secret = c.Push.Secret;
        if (!values.Push || !values.Push.SMTPPassword) push.SMTPPassword = c.Push.SMTPPassword;
        push.SMTPPort = Math.round(Number(push.SMTPPort || 587));
        push.Kind = ['', 'wecom', 'feishu', 'webhook', 'smtp'].includes(push.Kind) ? push.Kind : '';
        const remote = Object.assign({}, c.RemoteApproval, values.RemoteApproval || {});
        remote.Port = Math.round(Number(remote.Port || 8091));
        remote.PublicURL = String(remote.PublicURL || '').trim().replace(/\/$/, '');
        if (remote.Enabled) {
          let url;
          try { url = new URL(remote.PublicURL); } catch (e) { return { error: '公网审批地址须为 HTTPS 根地址' }; }
          if (url.protocol !== 'https:' || (url.pathname && url.pathname !== '/') || url.search || url.hash || url.username) {
            return { error: '只接受 HTTPS 根地址，不能带路径、参数或账号' };
          }
          if (remote.Port < 1 || remote.Port > 65535) return { error: '审批端口无效' };
        }
        c.Push = push;
        c.RemoteApproval = remote;
      } else if (section === 'system') {
        const lan = String(values.LAN || '').trim();
        const gw = String(values.Gateway || '').trim();
        if (!/^[A-Za-z0-9._:-]{1,32}$/.test(lan)) return { error: '热点接口名称无效' };
        if (!validIPv4(gw)) return { error: '网关须为 IPv4 地址' };
        const q = clone(values.DefaultQuota);
        const err = validateQuota(q);
        if (err) return { error: err };
        c.LAN = lan;
        c.Gateway = gw;
        c.Verified = !!values.Verified;
        c.DefaultQuota = q;
      } else {
        return { error: '未知设置页' };
      }
      log(s, 'config', '已保存' + section);
    });
  }
  function markMessages(read) {
    return update((s) => {
      if (read === 'clear-read') s.messages = s.messages.filter((m) => m.unread);
      else if (read === 'clear-all') s.messages = [];
      else s.messages.forEach((m) => { m.unread = false; });
    });
  }
  function dashboard(state) {
    const devices = Object.values(state.devices).map((d) => ({
      device: d,
      status: deviceState(d),
      ratio: ratio(d),
      admin: isAdmin(state, d.MAC)
    }));
    let up = 0, down = 0, today = 0, month = 0, total = 0, online = 0, approved = 0, limited = 0;
    const t = now();
    devices.forEach((x) => {
      const d = x.device;
      today += d.DayUp + d.DayDown;
      month += d.MonthBytes || 0;
      total += d.Up + d.Down;
      up += d.RateUp || 0;
      down += d.RateDown || 0;
      if (d.connected && t - d.LastSeen < 120) online += 1;
      if (allowed(d)) approved += 1;
      if (effectiveRate(d) > 0) limited += 1;
    });
    return {
      enabled: state.config.Enabled,
      uptime: Math.max(0, Math.floor((Date.now() - state.startedAt) / 1000)),
      devices: devices,
      requests: state.requests.slice(),
      today: today, month: month, total: total, up: up, down: down,
      online: online, approved: approved, limited: limited,
      blocked: state.blockedPackets, blackHits: state.blackPackets,
      rates: state.rates, version: '1.2.3', arch: 'demo'
    };
  }
  function bytes(n) {
    n = Number(n) || 0;
    if (n >= 1073741824) return (n / 1073741824).toFixed(2) + ' GiB';
    if (n >= 1048576) return (n / 1048576).toFixed(1) + ' MiB';
    if (n >= 1024) return (n / 1024).toFixed(1) + ' KiB';
    return n + ' B';
  }
  function stamp(t) {
    if (!t) return '—';
    try { return new Date(t * 1000).toLocaleString('zh-CN', { hour12: false }); } catch (e) { return '—'; }
  }

  return {
    KEY, MiB, PRESETS, RECOVERY_KEY,
    load, reset, update, seed,
    connect, disconnect, setUI, ensureCaptcha, renewCaptcha,
    authAccount, authCaptcha, authThird, requestApproval, decide,
    addTraffic, fillQuota, visit, deviceAction, markAdmin,
    login, logout, changePassword, recover, toggleAuth,
    addAccount, deleteAccount, prune, pushTest, remoteTest, saveSection, markMessages,
    deviceState, ratio, effectiveRate, isAdmin, portalKind, classifyHost, normalizeHost,
    dashboard, bytes, stamp, strong, validMAC, channelName, pendingRequest, byMac, allowed
  };
});
