(function (global) {
  'use strict';

  const KEY = 'ufi-gate-demo-v1';
  const MiB = 1048576;
  const channel = typeof BroadcastChannel === 'function' ? new BroadcastChannel('ufi-gate-demo') : null;

  function uid() {
    return Math.random().toString(36).slice(2, 10);
  }

  function defaultQuota() {
    return {
      dailyMiB: 200,
      totalMiB: 0,
      sessionMiB: 0,
      periodMiB: 1024,
      periodDays: 30,
      action: 'block',
      rateKBps: 128,
      slowPercent: 80
    };
  }

  function num(n) {
    const v = Number(n);
    return Number.isFinite(v) && v > 0 ? v : 0;
  }

  function cloneQuota(q) {
    q = q || {};
    return {
      dailyMiB: num(q.dailyMiB),
      totalMiB: num(q.totalMiB),
      sessionMiB: num(q.sessionMiB),
      periodMiB: num(q.periodMiB),
      periodDays: Math.min(365, Math.max(1, Math.round(num(q.periodDays) || 30))),
      action: q.action === 'slow' ? 'slow' : 'block',
      rateKBps: Math.min(102400, Math.max(1, Math.round(num(q.rateKBps) || 128))),
      slowPercent: Math.min(100, Math.max(0, Math.round(Number(q.slowPercent) || 0)))
    };
  }

  function device(partial) {
    return Object.assign({
      mac: '',
      ip: '',
      name: '',
      note: '',
      trusted: false,
      banned: false,
      up: 0,
      down: 0,
      dayUp: 0,
      dayDown: 0,
      sessionBytes: 0,
      periodBytes: 0,
      baseDay: 0,
      baseTotal: 0,
      baseSession: 0,
      basePeriod: 0,
      rateUp: 0,
      rateDown: 0,
      manualRateKBps: 0,
      until: 0,
      firstSeen: Date.now(),
      lastSeen: Date.now(),
      quota: null
    }, partial);
  }

  function visitor(id, mac, ip, name) {
    return {
      id: id,
      mac: mac,
      ip: ip,
      name: name,
      connected: false,
      phase: 'offline',
      visit: '',
      captcha: null,
      thirdOk: false,
      error: ''
    };
  }

  function defaultState() {
    const now = Date.now();
    const quota = defaultQuota();
    const rates = [];
    for (let i = 0; i < 36; i++) {
      const wave = 0.45 + 0.55 * Math.sin(i / 5);
      rates.push({
        t: now - (36 - i) * 8000,
        up: Math.round(40 * 1024 * wave),
        down: Math.round(220 * 1024 * (0.6 + 0.4 * Math.cos(i / 4)))
      });
    }
    return {
      version: 1,
      started: now,
      loggedIn: false,
      adminName: 'admin',
      adminPassword: 'admin123',
      adminMacs: ['02:11:22:33:44:55'],
      armed: false,
      section: 'dashboard',
      filter: '',
      sortBy: 'recent',
      simulating: { phone: false, tablet: false, pc: false },
      config: {
        enabled: true,
        verified: true,
        methods: ['account', 'captcha', 'approval'],
        sessionHours: 12,
        title: '连接，从这里开始',
        subtitle: '欢迎使用共享网络。完成认证，即可安心上网。',
        contact: '请联系网络管理员',
        warning: '流量已达到管理员设置的上限，请联系管理员解除限制。',
        blockWarning: '该网站已被管理员禁止访问。',
        accent: '#0067c0',
        lan: 'br0',
        gateway: '192.168.0.1',
        portalHost: '',
        captchaProvider: 'hcaptcha',
        black: ['ads.example.com'],
        white: ['wifi.example.com'],
        pushKind: 'wecom',
        pushURL: 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=demo',
        remoteEnabled: false,
        remotePublicURL: '',
        remotePort: 8091,
        defaultQuota: quota
      },
      accounts: [{ name: 'guest', password: 'wifi2026', expires: 0, quota: null }],
      devices: [
        device({
          mac: '02:11:22:33:44:55',
          ip: '192.168.0.10',
          name: '管理员手机',
          note: '日常管理',
          trusted: true,
          up: 86 * MiB,
          down: 740 * MiB,
          dayUp: 12 * MiB,
          dayDown: 96 * MiB,
          sessionBytes: 40 * MiB,
          periodBytes: 420 * MiB,
          rateUp: 18 * 1024,
          rateDown: 240 * 1024,
          until: now + 12 * 3600 * 1000,
          firstSeen: now - 20 * 86400000,
          lastSeen: now - 20000,
          quota: cloneQuota(quota)
        }),
        device({
          mac: '02:C0:FE:00:00:18',
          ip: '192.168.0.18',
          name: '书房笔记本',
          note: '家人',
          up: 210 * MiB,
          down: 980 * MiB,
          dayUp: 8 * MiB,
          dayDown: 120 * MiB,
          sessionBytes: 30 * MiB,
          periodBytes: 300 * MiB,
          rateUp: 8 * 1024,
          rateDown: 90 * 1024,
          until: now + 5 * 3600 * 1000,
          firstSeen: now - 6 * 86400000,
          lastSeen: now - 40000,
          quota: cloneQuota(quota)
        })
      ],
      requests: [],
      logs: [
        { time: now - 3600000, kind: 'service', message: '演示服务已就绪。此环境不修改防火墙。' },
        { time: now - 120000, kind: 'device', message: '书房笔记本 保持在线' }
      ],
      messages: [],
      clients: {
        phone: visitor('phone', '02:A1:11:00:00:01', '192.168.0.21', '小米 14'),
        tablet: visitor('tablet', '02:A1:11:00:00:02', '192.168.0.22', 'iPad'),
        pc: visitor('pc', '02:A1:11:00:00:03', '192.168.0.23', '客厅电脑')
      },
      rates: rates
    };
  }

  function load() {
    try {
      const raw = localStorage.getItem(KEY);
      if (!raw) return defaultState();
      const parsed = JSON.parse(raw);
      if (!parsed || parsed.version !== 1 || !parsed.config || !parsed.clients) return defaultState();
      return parsed;
    } catch (e) {
      return defaultState();
    }
  }

  function save(state) {
    const text = JSON.stringify(state);
    try { localStorage.setItem(KEY, text); } catch (e) { /* private mode */ }
    if (channel) channel.postMessage({ type: 'state', text: text });
    return state;
  }

  function update(mutator) {
    const state = load();
    mutator(state);
    return save(state);
  }

  function reset() {
    return save(defaultState());
  }

  function subscribe(fn) {
    function onStorage(e) {
      if (e.key === KEY && e.newValue) {
        try { fn(JSON.parse(e.newValue)); } catch (err) { /* ignore */ }
      }
    }
    function onMsg(ev) {
      if (ev.data && ev.data.type === 'state') {
        try { fn(JSON.parse(ev.data.text)); } catch (err) { /* ignore */ }
      }
    }
    window.addEventListener('storage', onStorage);
    if (channel) channel.addEventListener('message', onMsg);
    return function () {
      window.removeEventListener('storage', onStorage);
      if (channel) channel.removeEventListener('message', onMsg);
    };
  }

  function log(state, kind, message) {
    state.logs.push({ time: Date.now(), kind: kind, message: message });
    if (state.logs.length > 500) state.logs.splice(0, state.logs.length - 500);
  }

  function pushLabel(kind) {
    return { wecom: '企业微信', feishu: '飞书', webhook: '通用 Webhook', smtp: '邮件', '': '未开启推送' }[kind] || kind || '未开启推送';
  }

  function quotaOf(item, config) {
    return item.quota || config.defaultQuota;
  }

  function usedDay(d) { return Math.max(0, d.dayUp + d.dayDown - (d.baseDay || 0)); }
  function usedTotal(d) { return Math.max(0, d.up + d.down - (d.baseTotal || 0)); }
  function usedSession(d) { return Math.max(0, d.sessionBytes - (d.baseSession || 0)); }
  function usedPeriod(d) { return Math.max(0, d.periodBytes - (d.basePeriod || 0)); }

  function ratio(d, config) {
    const q = quotaOf(d, config);
    const parts = [];
    if (q.dailyMiB > 0) parts.push(usedDay(d) / (q.dailyMiB * MiB));
    if (q.totalMiB > 0) parts.push(usedTotal(d) / (q.totalMiB * MiB));
    if (q.sessionMiB > 0) parts.push(usedSession(d) / (q.sessionMiB * MiB));
    if (q.periodMiB > 0) parts.push(usedPeriod(d) / (q.periodMiB * MiB));
    return parts.length ? Math.max.apply(null, parts) : 0;
  }

  function isAdmin(state, mac) {
    return state.adminMacs.some(item => item.toLowerCase() === String(mac).toLowerCase());
  }

  function statusOf(d, state, now) {
    now = now || Date.now();
    if (d.banned) return 'blocked';
    const q = quotaOf(d, state.config);
    const r = ratio(d, state.config);
    if (r >= 1) return q.action === 'slow' ? 'limited' : 'quota';
    const session = d.trusted || d.until > now;
    if (session && q.slowPercent > 0 && r >= q.slowPercent / 100) return 'limited';
    if (session && d.manualRateKBps > 0) return 'limited';
    if (session) return 'online';
    return 'pending';
  }

  function findDevice(state, mac) {
    const key = String(mac || '').toLowerCase();
    return state.devices.find(d => d.mac.toLowerCase() === key) || null;
  }

  function ensureDevice(state, c) {
    let d = findDevice(state, c.mac);
    if (!d) {
      d = device({
        mac: c.mac,
        ip: c.ip,
        name: c.name,
        quota: cloneQuota(state.config.defaultQuota)
      });
      state.devices.push(d);
      log(state, 'device', '发现新设备 ' + c.name + ' · ' + c.mac);
    }
    d.ip = c.ip || d.ip;
    if (!d.name) d.name = c.name;
    d.lastSeen = Date.now();
    return d;
  }

  function pendingRequest(state, mac) {
    const now = Date.now();
    const key = String(mac).toLowerCase();
    return state.requests.find(r => r.mac.toLowerCase() === key && r.status === 'pending' && r.expires > now) || null;
  }

  function applyPhaseFromStatus(state, c, now) {
    if (!c.connected) return;
    if (!state.config.enabled) {
      c.phase = 'open';
      return;
    }
    const d = findDevice(state, c.mac);
    if (!d) {
      c.phase = 'portal';
      return;
    }
    const st = statusOf(d, state, now);
    if (st === 'blocked') c.phase = 'blocked';
    else if (st === 'quota') c.phase = 'quota';
    else if (st === 'online') c.phase = 'online';
    else if (st === 'limited') c.phase = 'limited';
    else c.phase = 'portal';
  }

  function syncClient(state, c, now) {
    if (!c.connected) return;
    if (c.phase === 'whitelist' || c.phase === 'black') return;
    if (c.phase === 'waiting' && pendingRequest(state, c.mac)) return;
    applyPhaseFromStatus(state, c, now || Date.now());
  }

  function syncMac(state, mac) {
    const now = Date.now();
    Object.keys(state.clients).forEach(id => {
      const c = state.clients[id];
      if (c.mac.toLowerCase() === String(mac).toLowerCase()) syncClient(state, c, now);
    });
  }

  function connect(state, id) {
    const c = state.clients[id];
    if (!c) return;
    const fresh = !c.connected;
    c.connected = true;
    c.visit = '';
    c.error = '';
    c.thirdOk = false;
    ensureDevice(state, c);
    if (pendingRequest(state, c.mac) && state.config.enabled) c.phase = 'waiting';
    else applyPhaseFromStatus(state, c, Date.now());
    if (fresh) log(state, 'device', c.name + ' 连接热点 UFI-WiFi');
  }

  function disconnect(state, id) {
    const c = state.clients[id];
    if (!c || !c.connected) return;
    c.connected = false;
    c.phase = 'offline';
    c.visit = '';
    c.error = '';
    state.simulating[id] = false;
    const d = findDevice(state, c.mac);
    if (d) {
      d.rateUp = 0;
      d.rateDown = 0;
      d.lastSeen = Date.now();
    }
    log(state, 'device', c.name + ' 断开热点');
  }

  function hostMatches(rule, host) {
    rule = String(rule || '').trim().toLowerCase();
    host = String(host || '').trim().toLowerCase();
    if (!rule || !host) return false;
    if (rule === host) return true;
    if (rule.startsWith('*.')) {
      const suffix = rule.slice(2);
      return host === suffix || host.endsWith('.' + suffix);
    }
    return false;
  }

  function listed(list, host) {
    return (list || []).some(rule => hostMatches(rule, host));
  }

  function browse(state, id, kind) {
    const c = state.clients[id];
    if (!c) return;
    if (!c.connected) connect(state, id);
    c.error = '';
    if (kind === 'white') {
      const host = (state.config.white && state.config.white[0]) || 'wifi.example.com';
      if (listed(state.config.white, host)) {
        c.phase = 'whitelist';
        c.visit = host;
        log(state, 'policy', '免认证放行 ' + host + ' · ' + c.mac);
        return;
      }
    }
    if (kind === 'black') {
      const host = (state.config.black && state.config.black[0]) || 'ads.example.com';
      const d = findDevice(state, c.mac);
      const st = d ? statusOf(d, state, Date.now()) : 'pending';
      if (state.config.enabled && (st === 'online' || st === 'limited') && listed(state.config.black, host)) {
        c.phase = 'black';
        c.visit = host;
        log(state, 'policy', '黑名单拦截 ' + host + ' · ' + c.mac);
        return;
      }
    }
    c.visit = '';
    if (c.phase === 'waiting' && pendingRequest(state, c.mac)) return;
    applyPhaseFromStatus(state, c, Date.now());
  }

  function leaveSpecial(state, id) {
    const c = state.clients[id];
    if (!c) return;
    c.visit = '';
    applyPhaseFromStatus(state, c, Date.now());
  }

  function grant(state, d, reason) {
    const hours = Math.min(720, Math.max(1, Number(state.config.sessionHours) || 12));
    d.banned = false;
    d.until = Date.now() + hours * 3600 * 1000;
    d.sessionBytes = 0;
    d.baseSession = 0;
    log(state, 'auth', reason);
    syncMac(state, d.mac);
  }

  function issueCaptcha(state, id) {
    const alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789';
    let code = '';
    for (let i = 0; i < 4; i++) code += alphabet[Math.floor(Math.random() * alphabet.length)];
    state.clients[id].captcha = { code: code, exp: Date.now() + 120000 };
    state.clients[id].error = '';
    return code;
  }

  function authAccount(state, id, username, password) {
    const c = state.clients[id];
    const d = ensureDevice(state, c);
    const acc = state.accounts.find(a => a.name === username);
    if (!acc || acc.password !== password) {
      c.error = '账号或密码不正确';
      log(state, 'auth', '账号认证失败 · ' + c.mac);
      return false;
    }
    if (acc.expires && acc.expires < Date.now()) {
      c.error = '该上网账号已到期';
      return false;
    }
    if (acc.quota) d.quota = cloneQuota(acc.quota);
    grant(state, d, '账号认证通过 · ' + username + ' · ' + c.mac);
    c.error = '';
    return true;
  }

  function authCaptcha(state, id, answer) {
    const c = state.clients[id];
    const d = ensureDevice(state, c);
    const ch = c.captcha;
    if (!ch || ch.exp < Date.now()) {
      c.error = '验证码已过期，请点击图片更换';
      return false;
    }
    if (String(answer || '').trim().toUpperCase() !== ch.code) {
      c.error = '验证码不正确';
      log(state, 'auth', '图形验证码失败 · ' + c.mac);
      issueCaptcha(state, id);
      return false;
    }
    grant(state, d, '图形验证码通过 · ' + c.mac);
    c.captcha = null;
    c.error = '';
    return true;
  }

  function authThird(state, id) {
    const c = state.clients[id];
    const d = ensureDevice(state, c);
    if (!c.thirdOk) {
      c.error = '请先完成人机验证';
      return false;
    }
    grant(state, d, '人机验证通过（演示） · ' + c.mac);
    c.thirdOk = false;
    c.error = '';
    return true;
  }

  function requestAccess(state, id, name, reason) {
    const c = state.clients[id];
    ensureDevice(state, c);
    const existing = pendingRequest(state, c.mac);
    if (existing) {
      c.phase = 'waiting';
      c.error = '';
      return existing;
    }
    const req = {
      id: uid(),
      mac: c.mac,
      ip: c.ip,
      deviceName: c.name,
      name: String(name || '').trim() || '匿名访客',
      reason: String(reason || '').trim(),
      status: 'pending',
      decisionReason: '',
      created: Date.now(),
      expires: Date.now() + 30 * 60 * 1000,
      decided: 0
    };
    state.requests.unshift(req);
    c.phase = 'waiting';
    c.error = '';
    const channelName = pushLabel(state.config.pushKind);
    const remote = state.config.remoteEnabled && state.config.remotePublicURL;
    const link = remote ? '\n远程审批 ' + String(state.config.remotePublicURL).replace(/\/$/, '') + '/approve?token=demo-' + req.id : '';
    state.messages.unshift({
      id: uid(),
      time: Date.now(),
      unread: true,
      kind: 'approval',
      channel: channelName,
      title: '新的上网申请',
      body: req.name + ' 请求上网\n设备 ' + c.name + ' · ' + c.mac + '\n' + (req.reason || '未填写理由') + link,
      requestId: req.id
    });
    log(state, 'approval', '收到上网申请 · ' + req.name + ' · ' + c.mac + (state.config.pushKind ? ' · 已模拟推送到' + channelName : ' · 未配置外部推送'));
    return req;
  }

  function decide(state, id, action, reason) {
    const req = state.requests.find(r => r.id === id);
    if (!req || req.status !== 'pending') return false;
    if (req.expires < Date.now()) {
      req.status = 'expired';
      log(state, 'approval', '申请已过期 · ' + req.name);
      return false;
    }
    req.status = action === 'allow' ? 'allow' : 'deny';
    req.decisionReason = reason || '';
    req.decided = Date.now();
    const d = findDevice(state, req.mac) || ensureDevice(state, { mac: req.mac, ip: req.ip, name: req.deviceName });
    if (req.status === 'allow') {
      grant(state, d, '审批同意 · ' + req.name + ' · ' + req.mac);
    } else {
      log(state, 'approval', '审批拒绝 · ' + req.name + (reason ? ' · ' + reason : ''));
      Object.keys(state.clients).forEach(cid => {
        const c = state.clients[cid];
        if (c.mac.toLowerCase() === req.mac.toLowerCase() && c.connected) {
          c.phase = 'portal';
          c.error = '申请被拒绝：' + (reason || '请联系管理员');
        }
      });
    }
    state.messages.unshift({
      id: uid(),
      time: Date.now(),
      unread: true,
      kind: 'result',
      channel: pushLabel(state.config.pushKind),
      title: req.status === 'allow' ? '已同意上网' : '已拒绝申请',
      body: req.name + ' · ' + req.mac + (reason ? '\n' + reason : ''),
      requestId: req.id
    });
    return true;
  }

  function normalizeMac(s) {
    const hex = String(s || '').toLowerCase().replace(/[^0-9a-f]/g, '');
    if (hex.length !== 12) return '';
    return hex.match(/.{2}/g).join(':').toUpperCase();
  }

  function deviceAction(state, mac, action, extra) {
    extra = extra || {};
    if (action === 'manual') {
      const macNorm = normalizeMac(extra.mac);
      if (!macNorm) throw new Error('请输入 MAC，例如 02:00:00:00:00:01');
      let item = findDevice(state, macNorm);
      if (!item) {
        item = device({ mac: macNorm, name: '手动录入', ip: '', quota: cloneQuota(state.config.defaultQuota) });
        state.devices.push(item);
      }
      if (extra.op === 'block') item.banned = true;
      else if (extra.op === 'trust') item.trusted = true;
      else if (extra.op === 'allow') grant(state, item, '手动预授权 · ' + macNorm);
      log(state, 'device', '手动录入 ' + macNorm);
      return;
    }
    const d = findDevice(state, mac);
    if (!d) return;
    if (action === 'allow') { grant(state, d, '管理员放行 · ' + d.mac); return; }
    if (action === 'edit') {
      d.note = extra.note || '';
      d.quota = cloneQuota(extra.quota);
      log(state, 'device', '更新配额与备注 · ' + d.mac);
      syncMac(state, d.mac);
      return;
    }
    if (action === 'rate') {
      const rate = Number(extra.rate);
      if (!Number.isInteger(rate) || rate < 1 || rate > 102400) throw new Error('请输入 1–102400 的整数');
      d.manualRateKBps = rate;
      log(state, 'device', '独立限速 ' + rate + ' KiB/s · ' + d.mac);
      syncMac(state, d.mac);
      return;
    }
    if (action === 'rate-clear') {
      d.manualRateKBps = 0;
      log(state, 'device', '取消独立限速 · ' + d.mac);
      syncMac(state, d.mac);
      return;
    }
    if (action === 'admin-add') {
      if (!isAdmin(state, d.mac)) state.adminMacs.push(d.mac);
      log(state, 'admin', '添加管理员设备 · ' + d.mac);
      return;
    }
    if (action === 'admin-remove') {
      if (state.adminMacs.length <= 1 && isAdmin(state, d.mac)) throw new Error('至少保留一台管理员设备');
      state.adminMacs = state.adminMacs.filter(m => m.toLowerCase() !== d.mac.toLowerCase());
      log(state, 'admin', '移除管理员设备 · ' + d.mac);
      return;
    }
    if (action === 'release') {
      d.banned = false;
      d.baseDay = d.dayUp + d.dayDown;
      d.baseTotal = d.up + d.down;
      d.baseSession = d.sessionBytes;
      d.basePeriod = d.periodBytes;
      grant(state, d, '解除限制并给予新一轮配额 · ' + d.mac);
      return;
    }
    if (action === 'trust') {
      d.trusted = true;
      d.banned = false;
      log(state, 'device', '永久信任 · ' + d.mac);
      syncMac(state, d.mac);
      return;
    }
    if (action === 'untrust') {
      d.trusted = false;
      log(state, 'device', '取消永久信任 · ' + d.mac);
      syncMac(state, d.mac);
      return;
    }
    if (action === 'revoke') {
      d.trusted = false;
      d.until = 0;
      log(state, 'device', '清除会话 · ' + d.mac);
      Object.keys(state.clients).forEach(cid => {
        const c = state.clients[cid];
        if (c.mac.toLowerCase() === d.mac.toLowerCase() && c.connected) {
          c.phase = state.config.enabled ? 'portal' : 'open';
          c.visit = '';
        }
      });
      return;
    }
    if (action === 'block') {
      d.banned = true;
      d.trusted = false;
      d.until = 0;
      log(state, 'device', '封禁 · ' + d.mac);
      syncMac(state, d.mac);
      return;
    }
    if (action === 'reset') {
      d.up = d.down = d.dayUp = d.dayDown = d.sessionBytes = d.periodBytes = 0;
      d.baseDay = d.baseTotal = d.baseSession = d.basePeriod = 0;
      d.rateUp = d.rateDown = 0;
      log(state, 'device', '重置流量 · ' + d.mac);
      syncMac(state, d.mac);
      return;
    }
    if (action === 'delete') {
      const adminKept = isAdmin(state, d.mac);
      state.devices = state.devices.filter(x => x.mac !== d.mac);
      log(state, 'device', '删除记录 · ' + d.mac + (adminKept ? '（管理员权限仍保留）' : ''));
      Object.keys(state.clients).forEach(cid => {
        if (state.clients[cid].mac.toLowerCase() === mac.toLowerCase()) disconnect(state, cid);
      });
    }
  }

  function toggleAuth(state, enabled) {
    state.config.enabled = !!enabled;
    state.armed = false;
    if (!enabled) {
      Object.keys(state.clients).forEach(id => {
        const c = state.clients[id];
        if (c.connected) {
          c.phase = 'open';
          c.visit = '';
        }
      });
      log(state, 'service', '认证已关闭，热点恢复原有准入（演示）');
    } else {
      log(state, 'service', '认证已开启。未认证设备会进入门户（演示，不写入 iptables）。');
      Object.keys(state.clients).forEach(id => {
        const c = state.clients[id];
        if (!c.connected) return;
        c.visit = '';
        if (pendingRequest(state, c.mac)) c.phase = 'waiting';
        else applyPhaseFromStatus(state, c, Date.now());
      });
    }
  }

  function applyTick(state) {
    const now = Date.now();
    let up = 0;
    let down = 0;
    let any = false;
    Object.keys(state.simulating).forEach(id => {
      if (!state.simulating[id]) return;
      const c = state.clients[id];
      if (!c || !c.connected) {
        state.simulating[id] = false;
        return;
      }
      const d = findDevice(state, c.mac);
      if (!d) return;
      const st = statusOf(d, state, now);
      if (st !== 'online' && st !== 'limited') {
        d.rateUp = 0;
        d.rateDown = 0;
        state.simulating[id] = false;
        if (c.phase === 'online' || c.phase === 'limited' || c.phase === 'portal') applyPhaseFromStatus(state, c, now);
        log(state, 'quota', c.name + ' 停止计流量，当前状态 ' + st);
        return;
      }
      any = true;
      const factor = st === 'limited' ? 0.18 : 1;
      const addDown = Math.round(20 * MiB * factor);
      const addUp = Math.round(1.2 * MiB * factor);
      d.down += addDown;
      d.up += addUp;
      d.dayDown += addDown;
      d.dayUp += addUp;
      d.sessionBytes += addDown + addUp;
      d.periodBytes += addDown + addUp;
      d.rateDown = addDown;
      d.rateUp = addUp;
      d.lastSeen = now;
      up += addUp;
      down += addDown;
      const next = statusOf(d, state, now);
      if (next !== st) {
        log(state, 'quota', c.name + ' 配额状态变为 ' + next);
        if (c.phase === 'online' || c.phase === 'limited') {
          c.phase = next === 'online' || next === 'limited' || next === 'quota' || next === 'blocked' ? next : c.phase;
        }
      }
    });
    if (!any) return false;
    state.rates.push({ t: now, up: up, down: down });
    if (state.rates.length > 60) state.rates.splice(0, state.rates.length - 60);
    return true;
  }

  function bytes(n) {
    n = Number(n) || 0;
    if (n >= 1073741824) return (n / 1073741824).toFixed(2) + ' GiB';
    if (n >= 1048576) return (n / 1048576).toFixed(1) + ' MiB';
    if (n >= 1024) return (n / 1024).toFixed(1) + ' KiB';
    return Math.round(n) + ' B';
  }

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, ch => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    }[ch]));
  }

  function stamp(t) {
    if (!t) return '—';
    return new Date(t).toLocaleString('zh-CN', { hour12: false });
  }

  function strong(p) {
    return p.length >= 8 && p.length <= 72 && /[A-Za-z]/.test(p) && /[0-9]/.test(p);
  }

  global.UFIDemo = {
    KEY: KEY,
    MiB: MiB,
    load: load,
    save: save,
    update: update,
    reset: reset,
    subscribe: subscribe,
    defaultQuota: defaultQuota,
    cloneQuota: cloneQuota,
    quotaOf: quotaOf,
    ratio: ratio,
    statusOf: statusOf,
    isAdmin: isAdmin,
    findDevice: findDevice,
    usedDay: usedDay,
    usedTotal: usedTotal,
    usedSession: usedSession,
    usedPeriod: usedPeriod,
    connect: connect,
    disconnect: disconnect,
    browse: browse,
    leaveSpecial: leaveSpecial,
    authAccount: authAccount,
    authCaptcha: authCaptcha,
    authThird: authThird,
    issueCaptcha: issueCaptcha,
    requestAccess: requestAccess,
    decide: decide,
    deviceAction: deviceAction,
    toggleAuth: toggleAuth,
    applyTick: applyTick,
    normalizeMac: normalizeMac,
    pushLabel: pushLabel,
    pendingRequest: pendingRequest,
    bytes: bytes,
    esc: esc,
    stamp: stamp,
    strong: strong,
    log: log
  };
})(window);
