(function () {
  'use strict';
  const D = window.UFIDemo;
  const $ = id => document.getElementById(id);
  const names = {
    dashboard: '网络概览',
    devices: '设备管理',
    approvals: '审批中心',
    accounts: '上网账号',
    auth: '认证方式',
    policy: '网址规则',
    appearance: '门户外观',
    designer: '页面设计器',
    push: '消息推送',
    system: '服务设置',
    logs: '运行日志',
    about: '关于'
  };
  const states = {
    online: '已放行', pending: '待认证', limited: '限速中', quota: '配额耗尽',
    blocked: '已封禁', allow: '已同意', deny: '已拒绝', expired: '已过期'
  };
  const iconPaths = {
    dashboard: 'M3 3h7v7H3zM14 3h7v7h-7zM3 14h7v7H3zM14 14h7v7h-7z',
    devices: 'M7 2h10v20H7zM10 18h4',
    approvals: 'M4 4h16v16H4zM7 12l3 3 7-7',
    accounts: 'M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8M4 22a8 8 0 0 1 16 0',
    auth: 'M12 3l8 3v6c0 4-4 7-8 9-4-2-8-5-8-9V6zM9 12l2 2 4-4',
    policy: 'M3 12h18M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18M12 3c5 5 5 13 0 18-5-5-5-13 0-18',
    appearance: 'M4 4h16v16H4zM4 14l5-5 7 7M15 7h2',
    designer: 'M4 4h16v16H4zM4 14l5-5 7 7M15 7h2',
    push: 'M5 17V9a7 7 0 0 1 14 0v8l2 2H3zM9 22h6',
    system: 'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8M12 2v3M12 19v3M2 12h3M19 12h3M5 5l2 2M17 17l2 2M5 19l2-2M17 7l2-2',
    logs: 'M5 3h14v18H5zM8 7h8M8 11h8M8 15h6',
    about: 'M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20M12 10v6M12 7h.01'
  };
  let busy = false;

  function icon(key) {
    return '<svg viewBox="0 0 24 24"><path d="' + iconPaths[key] + '"/></svg>';
  }
  function tag(status) {
    return '<span class="tag ' + status + '">' + (states[status] || D.esc(status)) + '</span>';
  }
  function button(action, label, klass) {
    return '<button type="button" data-action="' + action + '" class="' + (klass || '') + '">' + label + '</button>';
  }
  function field(id, title, val, type, extra) {
    return '<label>' + title + '<input id="' + id + '" type="' + (type || 'text') + '" value="' + D.esc(val == null ? '' : val) + '" ' + (extra || '') + '></label>';
  }
  function area(id, title, val) {
    return '<label>' + title + '<textarea id="' + id + '">' + D.esc(val || '') + '</textarea></label>';
  }
  function select(id, title, opts, val) {
    return '<label>' + title + '<select id="' + id + '">' + opts.map(pair =>
      '<option value="' + pair[0] + '"' + (pair[0] === val ? ' selected' : '') + '>' + pair[1] + '</option>').join('') + '</select></label>';
  }
  function flag(id, title, on) {
    return '<label><input id="' + id + '" type="checkbox"' + (on ? ' checked' : '') + '>' + title + '</label>';
  }
  function toast(msg) {
    const el = $('toast');
    el.textContent = msg;
    el.hidden = false;
    clearTimeout(toast.timer);
    toast.timer = setTimeout(() => { el.hidden = true; }, 3800);
  }
  function confirmBox(message, label) {
    label = label || '确认操作';
    const wait = Math.max(0, Number(window.UFI_CONFIRM_MS == null ? 3000 : window.UFI_CONFIRM_MS));
    return new Promise(resolve => {
      const modal = document.createElement('dialog');
      modal.className = 'confirm-dialog';
      modal.innerHTML = '<h2>请确认操作</h2><div class="dialog-body"><p id="confirm-message"></p><p class="note">请阅读操作影响，等待 ' + Math.ceil(wait / 1000) + ' 秒后手动确认。</p></div><div class="dialog-actions"><button type="button" id="confirm-cancel">取消</button><button type="button" id="confirm-accept" class="primary" disabled></button></div>';
      modal.querySelector('#confirm-message').textContent = message;
      document.body.append(modal);
      const accept = modal.querySelector('#confirm-accept');
      const deadline = performance.now() + wait;
      let settled = false;
      function finish(result) {
        if (settled) return;
        settled = true;
        clearInterval(timer);
        modal.close();
        modal.remove();
        resolve(result);
      }
      function tick() {
        const remaining = Math.max(0, Math.ceil((deadline - performance.now()) / 1000));
        accept.disabled = remaining > 0;
        accept.textContent = label + (remaining ? '（' + remaining + 's）' : '');
      }
      modal.querySelector('#confirm-cancel').onclick = () => finish(false);
      accept.onclick = () => { if (performance.now() >= deadline) finish(true); };
      modal.addEventListener('cancel', event => { event.preventDefault(); finish(false); });
      tick();
      const timer = setInterval(tick, 100);
      modal.showModal();
    });
  }
  function dialog(title, content, onSave, label) {
    const node = $('dialog');
    node.innerHTML = '<h2>' + title + '</h2><form id="dialogForm"><div class="dialog-body">' + content + '</div><div class="dialog-actions">' +
      (onSave ? '<button class="primary" type="submit">' + (label || '保存') + '</button>' : '') +
      '<button type="button" id="closeDialog">关闭</button></div><div id="dialogError" class="error"></div></form>';
    node.showModal();
    $('closeDialog').onclick = () => node.close();
    $('dialogForm').onsubmit = async event => {
      event.preventDefault();
      if (!onSave) return;
      try {
        const keep = await onSave();
        if (keep === false) return;
        node.close();
        render(D.load());
      } catch (e) {
        $('dialogError').textContent = e.message;
      }
    };
  }
  function quotaFields(q, prefix) {
    prefix = prefix || 'q';
    return '<div class="form-grid">' +
      field(prefix + 'daily', '自然日配额 / MiB', q.dailyMiB, 'number', 'min="0" step="1"') +
      field(prefix + 'total', '历史累计配额 / MiB', q.totalMiB, 'number', 'min="0" step="1"') +
      field(prefix + 'session', '单次认证累计 / MiB', q.sessionMiB, 'number', 'min="0" step="1"') +
      field(prefix + 'period', '周期配额 / MiB', q.periodMiB, 'number', 'min="0" step="1"') +
      field(prefix + 'days', '周期长度 / 天', q.periodDays, 'number', 'min="1" max="365"') +
      select(prefix + 'action', '达到 100% 后', [['block', '断网，等待管理员解除'], ['slow', '限速，等待管理员解除']], q.action) +
      field(prefix + 'rate', '限速 / KiB/s', q.rateKBps, 'number', 'min="1" max="102400"') +
      field(prefix + 'slow', '提前限速阈值 / %', q.slowPercent, 'number', 'min="0" max="100"') +
      '</div><p class="note">配额填 0 表示不限量；提前阈值填 0 表示不提前限速。演示初始值是每日 200 MiB，方便观察限速和断网。真实插件的默认配额为 0。</p>';
  }
  function readQuota(prefix) {
    prefix = prefix || 'q';
    const read = id => {
      const value = Number($(prefix + id).value);
      if (!Number.isFinite(value) || value < 0) throw new Error('流量配额须为非负数');
      return value;
    };
    return {
      dailyMiB: read('daily'),
      totalMiB: read('total'),
      sessionMiB: read('session'),
      periodMiB: read('period'),
      periodDays: Number($(prefix + 'days').value),
      action: $(prefix + 'action').value,
      rateKBps: Number($(prefix + 'rate').value),
      slowPercent: Number($(prefix + 'slow').value)
    };
  }
  function typing() {
    const el = document.activeElement;
    return !!(el && el.closest && el.closest('#view') && ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName));
  }

  function login(state) {
    $('root').innerHTML = '<main class="auth-shell"><section class="card auth-card"><div class="logo custom-logo"><img src="../assets/logo.png" alt="网站 Logo"></div>' +
      '<div class="badge">UFI-GATE · ADMINISTRATION</div><h1>欢迎回来</h1>' +
      '<p class="muted">登录后管理设备、流量和上网许可。这是浏览器里的演示后台。</p>' +
      '<form id="login">' + field('user', '管理员账号', 'admin', 'text', 'required maxlength="64" autocomplete="username"') +
      field('pass', '密码', '', 'password', 'required minlength="8" maxlength="128" autocomplete="current-password"') +
      '<button class="primary" id="loginbtn" type="submit">登录管理后台</button></form>' +
      '<p><button type="button" id="forgot" class="button">忘记密码？使用恢复密钥</button></p>' +
      '<div id="loginerror" class="error" role="alert"></div>' +
      '<div class="notice">演示账号 admin，密码 admin123。恢复密钥 DEMO-KEY。不会连接真实设备。</div>' +
      '<div class="footnote">闲置 30 分钟自动退出 · 演示不会计时</div></section></main>';
    $('login').onsubmit = event => {
      event.preventDefault();
      const name = $('user').value.trim();
      const pass = $('pass').value;
      if (name !== state.adminName || pass !== state.adminPassword) {
        $('loginerror').textContent = '账号或密码不正确';
        return;
      }
      render(D.update(s => { s.loggedIn = true; D.log(s, 'admin', '管理员登录（演示）'); }));
    };
    $('forgot').onclick = () => {
      $('root').querySelector('.auth-card').insertAdjacentHTML('beforeend', '');
      dialog('使用恢复密钥', field('rkey', '恢复密钥', '', 'password', 'required') +
        field('np1', '新密码', '', 'password', 'required minlength="8"') +
        field('np2', '确认新密码', '', 'password', 'required minlength="8"') +
        '<p class="note">演示密钥是 DEMO-KEY。真实插件会要求下载新的 TXT 恢复密钥后，旧密钥才失效。</p>',
      () => {
        if ($('rkey').value.trim() !== 'DEMO-KEY') throw new Error('恢复密钥不正确');
        if ($('np1').value !== $('np2').value) throw new Error('两次新密码不一致');
        if (!D.strong($('np1').value)) throw new Error('密码至少 8 位，并包含字母和数字');
        D.update(s => {
          s.adminPassword = $('np1').value;
          s.loggedIn = false;
          D.log(s, 'admin', '已用恢复密钥重置密码（演示）');
        });
        toast('密码已重置，请用新密码登录');
        render(D.load());
      }, '重置密码');
    };
  }

  function shell(state) {
    const unread = state.messages.filter(m => m.unread).length;
    $('root').innerHTML = '<div class="app"><aside class="sidebar" id="sidebar"><div class="brand"><img src="../assets/logo.png" alt="">UFI-Gate<small>YOUR NETWORK, YOUR RULES</small></div>' +
      '<nav class="nav">' + Object.keys(names).map(key => '<button type="button" data-nav="' + key + '">' + icon(key) + names[key] + '</button>').join('') + '</nav>' +
      '<footer>本地演示 · 不改防火墙<br>1.2.3<br><button type="button" id="logout">退出登录</button></footer></aside>' +
      '<main class="main"><header class="topbar"><div><span class="badge">NETWORK WORKSPACE</span><h1 id="heading"></h1><span class="muted" id="subheading"></span></div>' +
      '<div class="row">' + button('refresh', '刷新') + button('toggle', '认证服务', 'toggle') +
      '<button type="button" class="message-float" id="inbox">消息 <span id="inbox-count">' + unread + '</span></button></div></header><section id="view"></section></main></div>';
    document.querySelectorAll('[data-nav]').forEach(el => el.onclick = () => {
      render(D.update(s => { s.section = el.dataset.nav; }));
    });
    $('logout').onclick = () => render(D.update(s => { s.loggedIn = false; D.log(s, 'admin', '退出登录'); }));
    $('inbox').onclick = openInbox;
    bindActions();
  }

  function header(state) {
    $('heading').textContent = names[state.section] || '网络概览';
    $('subheading').textContent = state.config.enabled ? '认证正在运行 · 演示数据每秒可刷新' : '认证尚未开启 · 访客可以直接使用网络';
    document.querySelectorAll('[data-nav]').forEach(btn => btn.classList.toggle('active', btn.dataset.nav === state.section));
    const toggle = document.querySelector('[data-action="toggle"]');
    if (toggle) toggle.textContent = state.config.enabled ? '关闭认证' : '开启认证';
    const count = $('inbox-count');
    if (count) count.textContent = String(state.messages.filter(m => m.unread).length);
    document.documentElement.style.setProperty('--accent', state.config.accent || '#0067c0');
  }

  function metric(title, value, detail) {
    return '<article class="card metric"><div class="caption">' + title + '</div><div class="value">' + value + '</div><div class="detail">' + detail + '</div></article>';
  }
  function chart(state) {
    const points = state.rates || [];
    const max = Math.max(1, ...points.map(p => Math.max(p.up, p.down)));
    const w = 720;
    const left = 46;
    const right = w - 12;
    const path = key => points.map((p, i) => (i ? 'L' : 'M') + (left + i * (right - left) / Math.max(1, points.length - 1)).toFixed(1) + ',' + (150 - p[key] / max * 120).toFixed(1)).join(' ');
    return '<svg class="chart" viewBox="0 0 ' + w + ' 190" role="img" aria-label="上下行速率">' +
      [30, 70, 110, 150].map(y => '<path class="gridline" d="M' + left + ' ' + y + 'H' + right + '"/>').join('') +
      '<text x="2" y="24">' + D.bytes(max) + '/s</text>' +
      '<path d="' + path('up') + '" stroke="var(--accent)" stroke-width="2.5" fill="none"/>' +
      '<path d="' + path('down') + '" stroke="#799c45" stroke-width="2.5" fill="none"/>' +
      '<text x="' + left + '" y="182">较早</text><text x="' + (w - 36) + '" y="182">现在</text></svg>';
  }
  function renderDashboard(state) {
    const now = Date.now();
    const online = state.devices.filter(d => D.statusOf(d, state, now) === 'online' || D.statusOf(d, state, now) === 'limited').length;
    const active = state.devices.filter(d => now - d.lastSeen < 120000).length + Object.values(state.clients).filter(c => c.connected).length;
    const today = state.devices.reduce((sum, d) => sum + d.dayUp + d.dayDown, 0);
    const month = state.devices.reduce((sum, d) => sum + d.periodBytes, 0);
    const total = state.devices.reduce((sum, d) => sum + d.up + d.down, 0);
    const down = state.devices.reduce((sum, d) => sum + (d.rateDown || 0), 0);
    const up = state.devices.reduce((sum, d) => sum + (d.rateUp || 0), 0);
    const pending = state.requests.filter(r => r.status === 'pending' && r.expires > now);
    const limited = state.devices.filter(d => D.statusOf(d, state, now) === 'limited').length;
    const top = state.devices.slice().sort((a, b) => (b.up + b.down) - (a.up + a.down)).slice(0, 5);
    $('view').innerHTML = '<div class="metrics">' +
      metric('当前活跃设备', String(active), '已放行 ' + online + ' 台') +
      metric('今日用量', D.bytes(today), '自然日 · 演示可加速') +
      metric('当前下载', D.bytes(down) + '/s', '上传 ' + D.bytes(up) + '/s') +
      metric('待处理申请', String(pending.length), '限速中 ' + limited + ' 台') +
      '</div><div class="grid dashboard-grid"><article class="card"><div class="row between"><h2>网络流量</h2><span class="muted">演示曲线</span></div>' + chart(state) +
      '<div class="legend"><span><i></i>上传</span><span><i class="down"></i>下载</span></div></article>' +
      '<article class="card"><h2>流量概览</h2><div class="ring-row"><svg class="ring" viewBox="0 0 150 150"><circle cx="75" cy="75" r="57" stroke="var(--line)" stroke-width="12" fill="none"/><circle cx="75" cy="75" r="57" stroke="var(--accent)" stroke-width="12" fill="none" stroke-dasharray="' + (358 * Math.min(1, today / Math.max(month, 1))).toFixed(1) + ' 358" transform="rotate(-90 75 75)"/><text x="75" y="70" text-anchor="middle">今日占周期</text><text x="75" y="92" text-anchor="middle">' + Math.round(today / Math.max(month, 1) * 100) + '%</text></svg><div><p class="muted">周期 ' + D.bytes(month) + '</p><p class="muted">累计 ' + D.bytes(total) + '</p><p class="tiny">演示运行 ' + Math.max(1, Math.floor((now - state.started) / 60000)) + ' 分钟</p></div></div></article>' +
      '<article class="card"><h2>待办审批</h2>' + (pending.length ? pending.slice(0, 5).map(requestRow).join('') : '<div class="empty">暂时没有待处理申请<br><span class="tiny">访客提交申请后会出现在这里，也会进入消息</span></div>') + '</article>' +
      '<article class="card"><h2>用量排行</h2>' + (top.length ? top.map((d, i) => '<div class="rank"><span class="rank-name">' + (i + 1) + ' · ' + D.esc(d.note || d.name || d.mac) + '</span><strong>' + D.bytes(d.up + d.down) + '</strong></div>').join('') : '<div class="empty">连接设备后开始统计</div>') + '</article></div>';
    bindActions($('view'));
  }
  function requestRow(r) {
    return '<div class="request"><div class="row between"><strong>' + D.esc(r.name || '匿名访客') + '</strong>' + tag(r.status) + '</div><p>' + D.esc(r.reason || '未填写理由') + '</p><p class="tiny">' + D.esc(r.deviceName || '') + ' · ' + D.esc(r.mac) + ' · ' + D.stamp(r.created) + '</p>' +
      (r.decisionReason ? '<p class="muted">处理备注：' + D.esc(r.decisionReason) + '</p>' : '') +
      (r.status === 'pending' ? '<div class="row" data-id="' + r.id + '">' + button('allow-request', '同意上网', 'primary') + button('deny-request', '拒绝', 'danger') + '</div>' : '') + '</div>';
  }
  function deviceCard(state, d) {
    const now = Date.now();
    const status = D.statusOf(d, state, now);
    const pct = Math.min(100, Math.round(D.ratio(d, state.config) * 100));
    const q = D.quotaOf(d, state.config);
    const hasQuota = q.dailyMiB || q.totalMiB || q.sessionMiB || q.periodMiB;
    return '<article class="card device"><div class="row between"><h3>' + D.esc(d.note || d.name || '未命名设备') + '</h3>' + tag(status) + '</div>' +
      '<div class="address">' + D.esc(d.mac) + '<br>' + D.esc(d.ip || '暂无 IP') + (d.trusted ? ' · 永久信任' : '') + (D.isAdmin(state, d.mac) ? ' · 管理员设备' : '') + (d.manualRateKBps ? ' · 独立限速 ' + d.manualRateKBps + ' KiB/s' : '') + '</div>' +
      '<div class="device-stats"><span>今日 <strong>' + D.bytes(d.dayUp + d.dayDown) + '</strong></span><span>累计 <strong>' + D.bytes(d.up + d.down) + '</strong></span></div>' +
      '<div class="progress"><span class="' + (pct >= 100 ? 'bad' : pct >= 80 ? 'warn' : '') + '" style="width:' + pct + '%"></span></div>' +
      '<div class="row between tiny"><span>' + (hasQuota ? '配额用量 ' + pct + '%' : '尚未设置流量上限') + '</span><span>↑ ' + D.bytes(d.rateUp) + '/s · ↓ ' + D.bytes(d.rateDown) + '/s</span></div>' +
      '<div class="actions" data-mac="' + D.esc(d.mac) + '">' +
      button('device-allow', '放行') + button('device-edit', '配额与备注') + button('device-rate', d.manualRateKBps ? '修改限速' : '一键限速') +
      (d.manualRateKBps ? button('device-rate-clear', '取消独立限速') : '') +
      button(D.isAdmin(state, d.mac) ? 'device-admin-remove' : 'device-admin-add', D.isAdmin(state, d.mac) ? '移除管理员权限' : '设为管理员设备') +
      button('device-release', '解除限制') + button(d.trusted ? 'device-untrust' : 'device-trust', d.trusted ? '取消信任' : '永久信任') +
      button('device-revoke', '清除会话') + button('device-block', '封禁', 'danger') + button('device-reset', '重置流量', 'danger') + button('device-delete', '删除记录', 'danger') +
      '</div></article>';
  }
  function renderDevices(state) {
    const q = state.filter.toLowerCase();
    const items = state.devices.filter(d => (d.mac + ' ' + d.ip + ' ' + d.note + ' ' + d.name).toLowerCase().includes(q));
    items.sort((a, b) => state.sortBy === 'traffic' ? (b.up + b.down) - (a.up + a.down) : b.lastSeen - a.lastSeen);
    $('view').innerHTML = '<div class="toolbar"><input id="search" placeholder="搜索 MAC、IP、备注" value="' + D.esc(state.filter) + '">' +
      '<select id="sort"><option value="recent">最近活跃</option><option value="traffic"' + (state.sortBy === 'traffic' ? ' selected' : '') + '>流量从高到低</option></select>' +
      button('manual', '录入 MAC') + '</div><div class="device-grid">' +
      (items.length ? items.map(d => deviceCard(state, d)).join('') : '<div class="card empty">暂无匹配设备<br><span class="tiny">连接热点后，设备会自动出现</span></div>') + '</div>';
    $('search').oninput = event => {
      const pos = event.target.selectionStart;
      D.update(s => { s.filter = event.target.value; });
      renderDevices(D.load());
      const input = $('search');
      if (input) { input.focus(); input.setSelectionRange(pos, pos); }
    };
    $('sort').onchange = event => { D.update(s => { s.sortBy = event.target.value; }); renderDevices(D.load()); };
    bindActions($('view'));
  }
  function renderApprovals(state) {
    $('view').innerHTML = '<article class="card"><div class="row between"><h2>联网请求</h2>' + button('prune', '清理已结束记录') + '</div>' +
      (state.requests.length ? state.requests.map(requestRow).join('') : '<div class="empty">尚无上网申请</div>') + '</article>';
    bindActions($('view'));
  }
  function renderAccounts(state) {
    $('view').innerHTML = '<article class="card"><div class="row between"><h2>上网账号</h2>' + button('new-account', '新建账号', 'primary') + '</div>' +
      '<p class="muted">这些账号用于访客联网认证，与管理员账号独立。演示初始账号 guest / wifi2026。</p>' +
      state.accounts.map(a => '<div class="request"><div class="row between"><strong>' + D.esc(a.name) + '</strong><button type="button" data-action="delete-account" data-name="' + D.esc(a.name) + '" class="danger">删除</button></div><p class="tiny">到期时间 ' + (a.expires ? D.stamp(a.expires) : '永久有效') + (a.quota ? ' · 独立配额' : '') + '</p></div>').join('') +
      (!state.accounts.length ? '<div class="empty">先为访客创建一个上网账号</div>' : '') + '</article>';
    bindActions($('view'));
  }
  function renderLogs(state) {
    $('view').innerHTML = '<article class="card"><h2>运行记录</h2><p class="muted">保留最近 500 条。演示不会出现真实推送密钥。</p>' +
      state.logs.slice().reverse().map(l => '<div class="log-row"><div class="muted">' + D.stamp(l.time) + '</div><div>' + D.esc(l.kind) + '</div><div>' + D.esc(l.message) + '</div></div>').join('') + '</article>';
  }
  function renderAbout(state) {
    const mins = Math.max(1, Math.floor((Date.now() - state.started) / 60000));
    $('view').innerHTML = '<article class="card"><div class="about-hero"><img class="about-logo" src="../assets/logo.png" alt="UFI-Gate"><h2>UFI-Gate</h2><p class="muted">随身 WiFi 网络准入与流量配额管理</p></div>' +
      '<div class="about-grid"><div><div class="caption">当前版本</div><div class="value">v1.2.3</div></div><div><div class="caption">架构</div><div class="value">演示</div></div><div><div class="caption">运行时间</div><div class="value">' + mins + ' 分钟</div></div><div><div class="caption">认证服务</div><div class="value">' + (state.config.enabled ? '运行中' : '已关闭') + '</div></div></div>' +
      '<h3>项目资源</h3><div class="about-links"><a href="https://github.com/nanbeiluduo/UFI-Gate" target="_blank" rel="noopener">GitHub 仓库 ↗</a>' +
      '<a href="https://github.com/nanbeiluduo/UFI-Gate/releases" target="_blank" rel="noopener">发布历史 ↗</a>' +
      '<a href="https://github.com/nanbeiluduo/UFI-Gate/issues" target="_blank" rel="noopener">问题反馈 ↗</a>' +
      '<a href="https://t.me/nanbeiluduo" target="_blank" rel="noopener">Telegram 联系作者 ↗</a></div>' +
      '<p class="note">此画面复刻管理后台的信息架构。配置只存在于当前浏览器。</p></article>';
  }
  function renderDesigner(state) {
    const cfg = state.config;
    const card = (title, body) => '<article class="card"><h3>' + title + '</h3><div class="pt-protected"><strong>' + D.esc(cfg.title) + '</strong><p>' + body + '</p></div></article>';
    $('view').innerHTML = '<article class="card"><h2>四种访客页面</h2><p class="muted">真实的页面设计器可以添加文本、图片、按钮和公告，并分别发布认证首页、等待审批、认证成功和访问受限。演示只展示这四种状态，认证组件本身不能被盖住。</p></article>' +
      '<div class="grid">' +
      card('认证首页', D.esc(cfg.subtitle)) +
      card('等待审批', '上网请求已送达。审批结果会自动显示。') +
      card('认证成功', '您已通过认证，可以正常使用网络。') +
      card('访问受限', D.esc(cfg.blockWarning)) +
      '</div><p class="note">主题色和标题在「门户外观」里修改，预览会跟着变。</p>';
  }
  function renderSettings(state) {
    const c = state.config;
    const section = state.section;
    let text = '';
    if (section === 'auth') {
      text = '<h2>访客如何连接</h2><p class="muted">可同时开启多种认证方式，由访客自行选择。</p><div class="checks">' +
        ['account', 'captcha', 'approval', 'thirdparty'].map(m => flag('method-' + m, { account: '账号密码', captcha: '图形验证码', approval: '审批申请', thirdparty: '第三方人机验证' }[m], c.methods.includes(m))).join('') +
        '</div><div class="form-grid">' + field('sessionHours', '认证有效期 / 小时', c.sessionHours, 'number', 'min="1" max="720"') +
        select('captchaProvider', '验证码服务', [['hcaptcha', 'hCaptcha'], ['turnstile', 'Cloudflare Turnstile'], ['custom', '自定义兼容接口']], c.captchaProvider || 'hcaptcha') +
        field('portalHost', '认证页域名（可选）', c.portalHost) + '</div>' +
        '<div class="notice">第三方验证码在真机上需要把脚本域名加入白名单。演示用「我不是机器人」代替，不会把 Secret 发到任何地方。</div>';
    } else if (section === 'policy') {
      text = '<h2>网址访问规则</h2><div class="form-grid">' + area('black', '黑名单 · 每行一项', (c.black || []).join('\n')) + area('white', '免认证白名单 · 每行一项', (c.white || []).join('\n')) + '</div>' +
        '<div class="help">支持精确域名、*.example.com 和 IPv4。黑名单优先；白名单只放行列出的地址。演示里「访问白名单 / 黑名单」会使用每一栏的第一行。</div>';
    } else if (section === 'appearance') {
      text = '<h2>让访客看到您的网络</h2><div class="form-grid">' + field('title', '认证页标题', c.title) + field('accent', '主题色', c.accent, 'color') +
        '<div class="full">' + area('subtitle', '欢迎语', c.subtitle) + area('warning', '流量超限提示', c.warning) + area('blockWarning', '网站被禁止时的提示', c.blockWarning) + field('contact', '联系管理员', c.contact) + '</div></div>' +
        '<p class="note">真实后台还可以上传不超过 32 KiB 的网站 Logo 和认证页 Logo。演示使用内置的蓝色盾牌图标。</p>';
    } else if (section === 'push') {
      text = '<h2>让审批及时送达</h2><div class="form-grid">' +
        select('pushKind', '推送渠道', [['', '关闭推送'], ['wecom', '企业微信机器人'], ['feishu', '飞书群机器人'], ['webhook', '通用 Webhook'], ['smtp', 'SMTP 邮件']], c.pushKind) +
        field('pushURL', 'Webhook / HTTPS', c.pushURL, 'url') + '</div>' +
        '<div class="notice">演示不会向企业微信、飞书、Webhook 或邮箱发出真实请求。申请仍会出现在消息和审批中心，渠道名称按这里的选择显示。</div>' +
        button('push-test', '测试推送渠道（先保存）') +
        '<h2>远程审批 · 自备穿透服务</h2><div class="checks">' + flag('remoteEnabled', '启用远程审批', c.remoteEnabled) + '</div><div class="form-grid">' +
        field('remotePublicURL', '公网审批地址（HTTPS，无路径）', c.remotePublicURL, 'url', 'placeholder="https://approve.example.com"') +
        field('remotePort', '本机审批端口', c.remotePort || 8091, 'number', 'min="1024" max="65535"') + '</div>' +
        '<p class="note">真实服务只监听 127.0.0.1，由你自己的 HTTPS 隧道转发。UFI-Gate 不提供公共穿透。演示会在消息正文里写上这个地址。</p>' +
        button('remote-approval-test', '发送远程审批测试（先保存）');
    } else if (section === 'system') {
      text = '<h2>服务与默认策略</h2><div class="form-grid">' + field('lan', '热点接口', c.lan) + field('gateway', '热点 IPv4 网关', c.gateway) + '</div>' +
        '<div class="checks">' + flag('verified', '已核对接口、加速设置，并准备好 Root 恢复通道', state.armed) + '</div>' +
        '<p class="note">管理员设备：' + D.esc(state.adminMacs.join('、')) + '。至少保留一台。勾选此项后才能手动开启认证；关闭认证不需要勾选。</p><h3>新设备默认配额</h3>' +
        quotaFields(c.defaultQuota) +
        '<div class="notice">真机开启认证时会拒绝未认证 IPv4，并阻断该热点的 IPv6。演示只改变访客画面，不改系统防火墙。</div>' +
        '<div class="row">' + button('diagnostics', '运行诊断') + button('change-password', '修改管理账号') + '</div>';
    }
    $('view').innerHTML = '<article class="card" id="settings-card">' + text + '<div class="settings-actions">' + button('save-settings', '保存设置', 'primary') + button('show-portal', '打开认证页面') + '</div></article>';
    bindActions($('view'));
    if (section === 'system' && $('verified')) {
      $('verified').onchange = () => D.update(s => { s.armed = $('verified').checked; });
    }
  }

  function saveSettings(state) {
    const section = state.section;
    D.update(s => {
      const c = s.config;
      if (section === 'auth') {
        c.methods = ['account', 'captcha', 'approval', 'thirdparty'].filter(m => $('method-' + m).checked);
        c.sessionHours = Math.min(720, Math.max(1, Number($('sessionHours').value) || 12));
        c.captchaProvider = $('captchaProvider').value;
        c.portalHost = $('portalHost').value.trim();
      } else if (section === 'policy') {
        const lines = id => $(id).value.split('\n').map(item => item.trim().toLowerCase()).filter(Boolean);
        c.black = lines('black');
        c.white = lines('white');
      } else if (section === 'appearance') {
        c.title = $('title').value.trim() || '连接，从这里开始';
        c.subtitle = $('subtitle').value;
        c.warning = $('warning').value;
        c.blockWarning = $('blockWarning').value;
        c.contact = $('contact').value;
        c.accent = $('accent').value || '#0067c0';
      } else if (section === 'push') {
        c.pushKind = $('pushKind').value;
        c.pushURL = $('pushURL').value.trim();
        c.remoteEnabled = $('remoteEnabled').checked;
        c.remotePublicURL = $('remotePublicURL').value.trim().replace(/\/$/, '');
        c.remotePort = Math.min(65535, Math.max(1024, Number($('remotePort').value) || 8091));
        if (c.remoteEnabled && c.remotePublicURL && !/^https:\/\/[^/\s]+$/i.test(c.remotePublicURL)) throw new Error('公网审批地址需为不带路径的 HTTPS 根地址');
      } else if (section === 'system') {
        c.lan = $('lan').value.trim() || 'br0';
        c.gateway = $('gateway').value.trim() || '192.168.0.1';
        c.verified = $('verified').checked;
        s.armed = $('verified').checked;
        c.defaultQuota = D.cloneQuota(readQuota('q'));
      }
      D.log(s, 'config', '已保存' + (names[section] || '设置'));
    });
    toast('设置已保存');
  }

  function openInbox() {
    const state = D.load();
    const box = $('inbox-box');
    box.innerHTML = '<div class="row between"><h2>消息</h2><div class="row">' + button('read-all', '全部已读') + button('clear-read', '删除已读') + '<button type="button" id="close-inbox">关闭</button></div></div>' +
      (state.messages.length ? state.messages.map(m => '<div class="message-item' + (m.unread ? ' unread' : '') + '"><div class="row between"><strong>' + D.esc(m.title) + '</strong><span class="tag">' + D.esc(m.channel) + '</span></div><p>' + D.esc(m.body) + '</p><p class="tiny">' + D.stamp(m.time) + '</p>' +
        (m.kind === 'approval' ? '<div class="row" data-id="' + m.requestId + '">' + button('allow-request', '同意上网', 'primary') + button('deny-request', '拒绝', 'danger') + '</div>' : '') + '</div>').join('') : '<div class="empty">没有消息</div>');
    box.showModal();
    $('close-inbox').onclick = () => box.close();
    D.update(s => s.messages.forEach(m => { m.unread = false; }));
    bindActions(box);
  }

  async function doAction(action, el) {
    const state = D.load();
    if (action === 'refresh') { render(D.load()); return; }
    if (action === 'toggle') {
      const enabled = state.config.enabled;
      if (!enabled) {
        const onSystem = state.section === 'system' && $('verified');
        const armed = onSystem ? $('verified').checked : state.armed;
        const saved = state.config.verified;
        const unsaved = onSystem && $('lan') && ($('lan').value.trim() !== state.config.lan || $('gateway').value.trim() !== state.config.gateway);
        if (!armed || !saved || unsaved) {
          D.update(s => { s.section = 'system'; });
          render(D.load());
          const item = $('verified');
          if (item) {
            item.scrollIntoView({ block: 'center' });
            item.focus();
            item.closest('label').animate([{ backgroundColor: 'transparent' }, { backgroundColor: '#ffd96666' }, { backgroundColor: 'transparent' }], { duration: 1600 });
          }
          toast('每次手动开启认证前，请勾选确认项。改过接口或网关时，先保存再开启。');
          return;
        }
      }
      const ok = await confirmBox(enabled ? '关闭认证将恢复原有网络准入。是否继续？' : '启用后，未认证的访客会先看到认证页。演示不阻断真实网络。继续？', enabled ? '确认关闭' : '确认开启');
      if (!ok) return;
      render(D.update(s => D.toggleAuth(s, !enabled)));
      toast('认证服务状态已更新');
      return;
    }
    if (action === 'save-settings') {
      try { saveSettings(state); render(D.load()); } catch (e) { toast(e.message); }
      return;
    }
    if (action === 'show-portal') {
      parent.postMessage({ type: 'ufi-demo', action: 'show', frame: 'phone' }, location.origin);
      return;
    }
    if (action === 'push-test' || action === 'remote-approval-test') {
      D.update(s => {
        s.messages.unshift({
          id: Math.random().toString(36).slice(2),
          time: Date.now(),
          unread: true,
          kind: 'test',
          channel: D.pushLabel(s.config.pushKind),
          title: action === 'push-test' ? '推送测试' : '远程审批测试',
          body: action === 'push-test' ? '这是一条模拟测试。真实环境会发往已保存的渠道。' : '测试链接 5 分钟内有效，不会放行任何设备。演示不发起外网请求。',
          requestId: ''
        });
        D.log(s, 'push', action === 'push-test' ? '已模拟测试推送' : '已模拟远程审批测试');
      });
      toast('测试已记入消息和运行日志');
      render(D.load());
      return;
    }
    if (action === 'diagnostics') {
      dialog('服务诊断', '<pre>' + D.esc(JSON.stringify({
        mode: 'browser-demo',
        version: '1.2.3',
        enabled: state.config.enabled,
        lan: state.config.lan,
        gateway: state.config.gateway,
        firewall: '未改动',
        clients: Object.values(state.clients).map(c => ({ name: c.name, phase: c.phase, connected: c.connected }))
      }, null, 2)) + '</pre>');
      return;
    }
    if (action === 'manual') {
      dialog('手动录入设备', field('newMAC', 'MAC 地址', '', 'text', 'required placeholder="02:00:00:00:00:01"') +
        select('manualAction', '初始操作', [['trust', '永久信任'], ['block', '预封禁'], ['allow', '预授权']], 'trust'),
      () => { D.update(s => D.deviceAction(s, '', 'manual', { mac: $('newMAC').value, op: $('manualAction').value })); toast('设备已录入'); });
      return;
    }
    if (action === 'read-all') {
      D.update(s => s.messages.forEach(m => { m.unread = false; }));
      openInbox();
      return;
    }
    if (action === 'clear-read') {
      if (!await confirmBox('删除已读消息？', '确认删除')) return;
      D.update(s => { s.messages = s.messages.filter(m => m.unread); });
      openInbox();
      return;
    }
    if (action.indexOf('device-') === 0) {
      const mac = el.closest('[data-mac]').dataset.mac;
      const op = action.slice(7);
      const dev = D.findDevice(state, mac);
      if (!dev) return;
      if (op === 'rate') {
        dialog('独立限速', field('manualRate', '上传 / 下载限速（各方向）· KiB/s', dev.manualRateKBps || 128, 'number', 'required min="1" max="102400" step="1"') +
          '<p class="note">与配额限速同时存在时取较低值。1024 KiB/s 约等于 8 Mbps。</p>',
        () => {
          D.update(s => D.deviceAction(s, mac, 'rate', { rate: Number($('manualRate').value) }));
          toast('独立限速已设置');
        });
        return;
      }
      if (op === 'edit') {
        dialog('设备配额与备注', field('note', '备注', dev.note) + quotaFields(D.quotaOf(dev, state.config)),
          () => { D.update(s => D.deviceAction(s, mac, 'edit', { note: $('note').value, quota: readQuota('q') })); toast('设备设置已保存'); });
        return;
      }
      const labels = {
        block: '封禁设备', release: '解除限制并给予新一轮配额', reset: '重置全部流量计数',
        trust: '设为永久信任', untrust: '取消永久信任', revoke: '清除会话',
        'admin-add': '允许此设备访问管理后台？仍需账号密码。', 'admin-remove': '移除此设备的管理权限？联网授权保持不变。',
        delete: '删除该设备的连接记录？下次接入会当作新访客。', allow: '立即放行此设备'
      };
      if (labels[op] && !await confirmBox(labels[op] + '？', '确认')) return;
      try {
        D.update(s => D.deviceAction(s, mac, op));
        toast('设备操作已完成');
        render(D.load());
      } catch (e) { toast(e.message); }
      return;
    }
    if (action === 'allow-request' || action === 'deny-request') {
      const id = el.closest('[data-id]').dataset.id;
      const op = action === 'allow-request' ? 'allow' : 'deny';
      dialog(op === 'allow' ? '同意上网' : '拒绝申请', field('decisionReason', '处理备注', ''), async () => {
        if (!await confirmBox(op === 'allow' ? '确认同意此设备上网？' : '确认拒绝此设备的申请？', '确认处理')) return false;
        D.update(s => D.decide(s, id, op, $('decisionReason').value));
        toast('审批已完成');
        const box = $('inbox-box');
        if (box && box.open) box.close();
      }, '确认处理');
      return;
    }
    if (action === 'prune') {
      if (!await confirmBox('清理已结束和过期的审批记录？', '确认清理')) return;
      D.update(s => { s.requests = s.requests.filter(r => r.status === 'pending' && r.expires > Date.now()); });
      render(D.load());
      return;
    }
    if (action === 'new-account') {
      dialog('创建上网账号', field('newUser', '账号', '', 'text', 'required maxlength="64"') + field('newPass', '密码', '', 'password', 'required minlength="8"') +
        '<div class="checks">' + flag('accountQuota', '为此账号绑定独立配额', false) + '</div>' + quotaFields(state.config.defaultQuota, 'aq'),
      () => {
        const name = $('newUser').value.trim();
        const pass = $('newPass').value;
        if (!D.strong(pass)) throw new Error('密码至少 8 位，并包含字母和数字');
        D.update(s => {
          if (s.accounts.some(a => a.name === name)) throw new Error('账号已存在');
          s.accounts.push({ name: name, password: pass, expires: 0, quota: $('accountQuota').checked ? D.cloneQuota(readQuota('aq')) : null });
          D.log(s, 'account', '创建上网账号 ' + name);
        });
        toast('上网账号已创建');
      });
      return;
    }
    if (action === 'delete-account') {
      if (!await confirmBox('删除上网账号 ' + el.dataset.name + '？', '确认删除')) return;
      D.update(s => { s.accounts = s.accounts.filter(a => a.name !== el.dataset.name); D.log(s, 'account', '删除上网账号 ' + el.dataset.name); });
      render(D.load());
      return;
    }
    if (action === 'change-password') {
      dialog('修改管理员账号', field('adminUser', '新的管理员账号', state.adminName, 'text', 'required') +
        field('oldPass', '原密码', '', 'password', 'required') + field('newPass', '新密码', '', 'password', 'required minlength="8"'),
      () => {
        if ($('oldPass').value !== D.load().adminPassword) throw new Error('原密码不正确');
        if (!D.strong($('newPass').value)) throw new Error('密码至少 8 位，并包含字母和数字');
        D.update(s => {
          s.adminName = $('adminUser').value.trim() || 'admin';
          s.adminPassword = $('newPass').value;
          s.loggedIn = false;
          D.log(s, 'admin', '管理员密码已更新');
        });
        toast('已更新，请重新登录');
        render(D.load());
      });
    }
  }

  function bindActions(scope) {
    (scope || document).querySelectorAll('[data-action]').forEach(el => {
      el.onclick = () => {
        if (busy) return;
        busy = true;
        Promise.resolve(doAction(el.dataset.action, el)).catch(e => toast(e.message)).finally(() => { busy = false; });
      };
    });
  }

  let painted = false;
  function render(state) {
    if (!state.loggedIn) {
      painted = false;
      login(state);
      return;
    }
    if (!painted || !$('view')) {
      shell(state);
      painted = true;
    }
    header(state);
    const section = names[state.section] ? state.section : 'dashboard';
    if (section === 'dashboard') renderDashboard(state);
    else if (section === 'devices') renderDevices(state);
    else if (section === 'approvals') renderApprovals(state);
    else if (section === 'accounts') renderAccounts(state);
    else if (section === 'logs') renderLogs(state);
    else if (section === 'about') renderAbout(state);
    else if (section === 'designer') renderDesigner(state);
    else renderSettings(state);
  }

  function onState(state) {
    if (!state.loggedIn) return;
    if (document.querySelector('dialog[open]')) {
      header(state);
      return;
    }
    if (typing() && !['dashboard', 'devices', 'approvals', 'logs'].includes(state.section)) {
      header(state);
      return;
    }
    if (['dashboard', 'devices', 'approvals', 'logs'].includes(state.section)) {
      header(state);
      if (state.section === 'dashboard') renderDashboard(state);
      else if (state.section === 'devices' && document.activeElement && document.activeElement.id === 'search') return;
      else if (state.section === 'devices') renderDevices(state);
      else if (state.section === 'approvals') renderApprovals(state);
      else renderLogs(state);
      return;
    }
    header(state);
  }

  D.subscribe(onState);
  render(D.load());
  window.addEventListener('storage', event => {
    if (event.key !== 'ufi-site-theme') return;
    const theme = event.newValue;
    if (theme === 'light' || theme === 'dark') document.documentElement.setAttribute('data-theme', theme);
    else document.documentElement.removeAttribute('data-theme');
  });
})();
