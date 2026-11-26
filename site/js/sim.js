(function () {
  const E = window.UFIDemo;
  const params = new URLSearchParams(location.search);
  const view = params.get('view') === 'admin' ? 'admin' : 'client';
  const deviceId = params.get('device') || 'phone';
  const names = {
    dashboard: '网络概览', devices: '设备管理', approvals: '审批中心', accounts: '上网账号',
    auth: '认证方式', policy: '网址规则', appearance: '门户外观', designer: '页面设计器',
    push: '消息推送', system: '服务设置', updates: '更新管理', logs: '运行日志', about: '关于'
  };
  const stateLabel = {
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
    push: 'M5 17V9a7 7 0 0 1 14 0v8l2 2H3zM9 22h6',
    system: 'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8M12 2v3M12 19v3M2 12h3M19 12h3M5 5l2 2M17 17l2 2M5 19l2-2M17 7l2-2',
    logs: 'M5 3h14v18H5zM8 7h8M8 11h8M8 15h6',
    updates: 'M20 7V3l-3 3a8 8 0 1 0 3 10M20 3h-5',
    about: 'M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20M12 10a1 1 0 1 0 0 2 1 1 0 0 0 0-2M11 9h2v7h-2z'
  };
  iconPaths.designer = iconPaths.appearance;
  const methodLabel = { account: '账号登录', captcha: '图形验证码', approval: '申请上网', thirdparty: '人机验证' };
  let section = 'dashboard';
  let filter = '';
  let sortBy = 'recent';
  let enableVerified = false;
  let highlightVerified = false;
  let method = '';
  let shell = false;
  let toastTimer = 0;

  const $ = (id) => document.getElementById(id);
  function esc(s) {
    return String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  }
  function icon(k) { return '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="' + iconPaths[k] + '"/></svg>'; }
  function button(action, label, klass, extra) {
    return '<button type="button" data-action="' + action + '" class="' + (klass || '') + '" ' + (extra || '') + '>' + label + '</button>';
  }
  function tag(status) { return '<span class="tag ' + esc(status) + '">' + esc(stateLabel[status] || status) + '</span>'; }
  function input(id, title, val, type, extra) {
    return '<label>' + esc(title) + '<input id="' + id + '" type="' + (type || 'text') + '" value="' + esc(val ?? '') + '" ' + (extra || '') + '></label>';
  }
  function textarea(id, title, val) {
    return '<label>' + esc(title) + '<textarea id="' + id + '">' + esc(val ?? '') + '</textarea></label>';
  }
  function select(id, title, opts, val) {
    return '<label>' + esc(title) + '<select id="' + id + '">' + opts.map(([k, v]) => '<option value="' + esc(k) + '"' + (k === val ? ' selected' : '') + '>' + esc(v) + '</option>').join('') + '</select></label>';
  }
  function flag(id, title, on) {
    return '<label><input id="' + id + '" type="checkbox"' + (on ? ' checked' : '') + '>' + esc(title) + '</label>';
  }
  function toast(msg) {
    const el = $('toast');
    if (!el) return;
    el.hidden = false;
    el.textContent = msg;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { el.hidden = true; }, 3200);
  }
  function run(res) {
    if (!res) return res;
    if (!res.ok) toast(res.error || '没有完成');
    else if (res.detail && res.detail.dropped) toast('这台设备当前不能上网，流量已被丢弃');
    else if (res.detail && res.detail.notice) toast(res.detail.notice);
    render();
    return res;
  }
  function gateway() {
    const s = E.load();
    return s.config.Gateway || '192.168.0.1';
  }
  function clock() {
    return new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false });
  }
  function quotaForm(q, prefix) {
    const mib = (n) => String(Math.round((Number(n) || 0) / E.MiB));
    return '<div class="form-grid">'
      + input(prefix + 'daily', '自然日配额 / MiB', mib(q.Daily), 'number', 'min="0" step="1"')
      + input(prefix + 'total', '历史累计配额 / MiB', mib(q.Total), 'number', 'min="0" step="1"')
      + input(prefix + 'session', '单次认证累计 / MiB', mib(q.Session), 'number', 'min="0" step="1"')
      + input(prefix + 'period', '周期配额 / MiB', mib(q.Period), 'number', 'min="0" step="1"')
      + input(prefix + 'days', '周期长度 / 天', q.PeriodDays, 'number', 'min="1" max="365"')
      + select(prefix + 'action', '达到 100% 后', [['block', '断网，等待管理员解除'], ['slow', '限速，等待管理员解除']], q.Action)
      + input(prefix + 'rate', '限速 / KiB/s', q.RateKBps, 'number', 'min="1" max="102400"')
      + input(prefix + 'slow', '提前限速阈值 / %', q.SlowPercent, 'number', 'min="0" max="100"')
      + '</div><p class="note">配额填 0 表示不限量；提前阈值填 0 表示不提前限速。自然日按 UTC+8。演示里的默认每日 512 MiB 只是为了方便观察，真实插件默认是 0。</p>';
  }
  function readQuota(prefix) {
    const mib = (id) => {
      const n = Number($(prefix + id).value);
      if (!Number.isFinite(n) || n < 0) throw new Error('流量配额须为非负数');
      return Math.round(n * E.MiB);
    };
    return {
      Daily: mib('daily'), Total: mib('total'), Session: mib('session'), Period: mib('period'),
      PeriodDays: Number($(prefix + 'days').value),
      Action: $(prefix + 'action').value,
      RateKBps: Number($(prefix + 'rate').value),
      SlowPercent: Number($(prefix + 'slow').value)
    };
  }
  function confirmBox(message, title) {
    const d = $('dialog');
    d.className = 'confirm-dialog';
    d.innerHTML = '<h2>' + esc(title || '请确认') + '</h2><p id="confirm-message">' + esc(message) + '</p><div class="dialog-actions"><button type="button" id="dlg-no">取消</button><button type="button" class="primary" id="dlg-yes">确定</button></div>';
    return new Promise((resolve) => {
      const finish = (v) => { d.close(); resolve(v); };
      d.querySelector('#dlg-no').onclick = () => finish(false);
      d.querySelector('#dlg-yes').onclick = () => finish(true);
      d.oncancel = (e) => { e.preventDefault(); finish(false); };
      d.showModal();
    });
  }
  function formDialog(title, body, onOk) {
    const d = $('dialog');
    d.className = '';
    d.innerHTML = '<form id="dialogForm"><h2>' + esc(title) + '</h2><div class="dialog-body">' + body + '</div><p class="error" id="dialogError"></p><div class="dialog-actions"><button type="button" id="dlg-no">取消</button><button class="primary" type="submit">确定</button></div></form>';
    return new Promise((resolve) => {
      const finish = (v) => { d.close(); resolve(v); };
      d.querySelector('#dlg-no').onclick = () => finish(false);
      d.oncancel = (e) => { e.preventDefault(); finish(false); };
      d.querySelector('#dialogForm').onsubmit = (e) => {
        e.preventDefault();
        try {
          const out = onOk();
          if (out === false) return;
          finish(true);
        } catch (err) {
          d.querySelector('#dialogError').textContent = err.message || String(err);
        }
      };
      d.showModal();
    });
  }
  function infoDialog(title, body) {
    const d = $('dialog');
    d.className = '';
    d.innerHTML = '<h2>' + esc(title) + '</h2><div class="dialog-body">' + body + '</div><div class="dialog-actions"><button type="button" class="primary" id="dlg-no">关闭</button></div>';
    d.querySelector('#dlg-no').onclick = () => d.close();
    d.oncancel = null;
    d.showModal();
  }
  function captchaSVG(code) {
    const letters = String(code || '').split('').map((ch, i) => {
      const x = 16 + i * 32;
      const rot = (i - 1.5) * 8;
      return '<text x="' + x + '" y="40" font-size="28" font-family="ui-monospace,monospace" fill="#17263b" transform="rotate(' + rot + ' ' + x + ' 36)">' + esc(ch) + '</text>';
    }).join('');
    return '<svg xmlns="http://www.w3.org/2000/svg" width="148" height="56" viewBox="0 0 148 56"><rect width="148" height="56" rx="6" fill="#f4f7fb"/>' + letters + '<path d="M8 30 H140" stroke="#9db4cc" stroke-width="1" fill="none"/></svg>';
  }
  function tellParent(mode) {
    if (window.parent && window.parent !== window) {
      window.parent.postMessage({ type: 'ufi-demo', action: 'show', mode: mode }, location.origin);
    }
  }

  function chrome(kind, address, inner) {
    if (kind === 'pc') {
      return '<div class="desktop"><section class="window" aria-label="桌面浏览器"><div class="window-bar"><span class="dots" aria-hidden="true"><i></i><i></i><i></i></span><span class="addr">' + esc(address) + '</span></div><div class="window-body">' + inner + '</div></section></div>';
    }
    return '<div class="device-screen ' + kind + '"><div class="status"><span>' + clock() + '</span><span>WLAN</span></div>'
      + (address ? '<div class="addr-mobile">' + esc(address) + '</div>' : '')
      + '<div class="device-scroll">' + inner + '</div></div>';
  }
  function portalInner(d, s) {
    const c = s.config;
    const kind = E.portalKind(s, d);
    let title = c.Title;
    let subtitle = c.Subtitle;
    let body = '';
    let showAdmin = true;
    if (kind === 'disabled') {
      body = '<div class="help">网络认证未开启，您可以直接使用网络。</div>';
    } else if (kind === 'blocked') {
      title = '设备暂不可上网';
      subtitle = '';
      body = '<div class="help">该设备已被管理员限制，请联系管理员。</div><p class="muted">今日已用 ' + E.bytes(d.DayUp + d.DayDown) + ' · 累计 ' + E.bytes(d.Up + d.Down) + '</p><p>' + esc(c.Contact) + '</p>';
    } else if (kind === 'quota') {
      title = '流量已达到上限';
      subtitle = '';
      body = '<div class="help">' + esc(c.Warning) + '</div><p class="muted">今日已用 ' + E.bytes(d.DayUp + d.DayDown) + ' · 累计 ' + E.bytes(d.Up + d.Down) + '</p><p>' + esc(c.Contact) + '</p>';
    } else if (kind === 'waiting') {
      title = '正在等待管理员';
      subtitle = '审批结果会自动出现在这一页。';
      body = '<div class="help">上网请求已送达。审批结果会自动显示，无需重复申请。</div><p class="muted">' + esc(c.Contact) + '</p>';
    } else if (kind === 'success' || kind === 'limited') {
      title = kind === 'limited' ? '已连接 · 当前限速' : '连接成功';
      subtitle = '网络已为您准备好。';
      const text = kind === 'limited'
        ? (d.ManualRateKBps ? '管理员已设置设备限速，当前可以正常联网。' : c.Warning)
        : '您已通过认证，可以正常使用网络。';
      body = '<div class="success-icon">✓</div><p>' + esc(text) + '</p>' + button('browse-open', '打开浏览器', 'primary');
      showAdmin = true;
    } else {
      body = authForm(d, s);
    }
    const deny = s.requests.find((r) => r.MAC === d.MAC && r.Status === 'deny');
    const error = kind === 'login' && deny ? '申请被拒绝：' + (deny.DecisionReason || '请联系管理员') : '';
    return '<main class="auth-shell"><section class="card auth-card"><div class="logo custom-logo"><img src="assets/logo.png" alt="网络 Logo"></div><div class="badge">UFI-GATE · NETWORK ACCESS</div><h1>' + esc(title) + '</h1>'
      + (subtitle ? '<p class="muted">' + esc(subtitle) + '</p>' : '')
      + '<div id="content">' + body + '</div><div class="error" role="alert">' + esc(error) + '</div>'
      + (showAdmin ? '<p class="admin-links"><button type="button" class="linkish" data-action="admin-access">管理员登录</button><button type="button" class="linkish" data-action="open-backend">打开管理后台</button></p>' : '')
      + '<div class="footnote">共享网络 · 安心连接<br>UFI-Gate · 浏览器演示</div></section></main>';
  }
  function authForm(d, s) {
    const methods = s.config.Methods || [];
    if (!method || !methods.includes(method)) method = methods[0] || 'account';
    const tabs = methods.map((m) => '<button type="button" data-method="' + m + '" class="' + (m === method ? 'active' : '') + '">' + methodLabel[m] + '</button>').join('');
    let fields = '';
    if (method === 'account') {
      fields = '<label>上网账号<input id="username" autocomplete="username" required maxlength="64" placeholder="输入管理员提供的账号"></label><label>密码<input id="password" type="password" autocomplete="current-password" required maxlength="128" placeholder="输入密码"></label><p class="note">演示账号 guest，密码 wifi2026。</p>';
    } else if (method === 'captcha') {
      const cap = E.ensureCaptcha(d.id);
      const code = (cap.detail && cap.detail.code) || (E.load().devices[d.id].captcha || {}).code || '';
      fields = '<label>输入图片中的字符</label><div class="captcha"><img id="captcha" alt="图形验证码，点击更换" src="data:image/svg+xml;charset=utf-8,' + encodeURIComponent(captchaSVG(code)) + '">' + '<input id="answer" required maxlength="8" autocomplete="off" placeholder="验证码"></div>';
    } else if (method === 'approval') {
      fields = '<label>您的称呼<input id="name" required maxlength="100" placeholder="方便管理员识别您"></label><label>申请理由<input id="reason" maxlength="1000" placeholder="例如：临时上网、查阅资料"></label>';
    } else {
      const provider = { hcaptcha: 'hCaptcha', turnstile: 'Cloudflare Turnstile', custom: '自定义验证' }[s.config.CaptchaProvider] || '人机验证';
      fields = '<label class="mock-captcha"><input id="robot" type="checkbox">我不是机器人<span class="muted">演示控件 · ' + esc(provider) + ' · 不会加载外部脚本</span></label>';
    }
    return '<div class="tabs">' + tabs + '</div><form id="authform">' + fields + '<button class="primary" id="submit" type="submit">' + (method === 'approval' ? '发送上网请求' : '验证并连接') + '</button></form>';
  }
  function wifiScreen(kind, d) {
    if (kind === 'pc') {
      return '<div class="desktop"><section class="flyout card"><div class="badge">WI-FI</div><h2>网络</h2><div class="wifi-row"><div><strong>UFI-Gate</strong><div class="muted">开放网络 · 未连接</div></div></div>'
        + button('connect', '连接', 'primary') + '<p class="note">连上开放热点后，系统会打开认证门户。这一步只发生在演示里。</p></section></div>';
    }
    return '<div class="device-screen ' + kind + '"><div class="status"><span>' + clock() + '</span><span>WLAN</span></div><div class="wifi"><p class="badge">WLAN</p><h1>无线局域网</h1><button type="button" class="wifi-row" data-action="connect"><span><strong>UFI-Gate</strong><small>开放网络 · 点击连接</small></span></button><p class="note">未认证的设备会被带到门户。若系统把网络记成已验证，真实手机上需要忽略该网络后重连。</p></div></div>';
  }
  function browserScreen(d, s) {
    const page = d.browse || { host: 'example.com', kind: 'open' };
    const gw = gateway();
    let inner = '';
    if (page.kind === 'black') {
      inner = '<main class="auth-shell"><section class="card auth-card"><div class="badge">RESTRICTED</div><h1>此网站暂不可访问</h1><div class="help">' + esc(s.config.BlockWarning) + '</div><p class="muted">' + esc(s.config.Contact) + '</p></section></main>';
    } else if (page.kind === 'white') {
      inner = '<main class="auth-shell"><section class="card auth-card"><div class="success-icon">✓</div><h1>已放行</h1><p>「' + esc(page.host) + '」在免认证白名单里，还没完成认证也可以打开。</p></section></main>';
    } else if (page.kind === 'blocked' || page.kind === 'quota') {
      inner = portalInner(d, s);
    } else {
      inner = '<article class="card web-page"><p class="badge">HTTP</p><h1>' + esc(page.host) + '</h1><p>这是一张示意页面。演示里的访问不会离开浏览器，也不会真正解析 DNS。</p><p class="muted">当前设备 ' + esc(d.Name) + ' · ' + esc(E.deviceState(d) === 'limited' ? '限速中仍可访问' : '已放行') + '</p></article>';
    }
    const address = 'http://' + page.host + '/';
    return chrome(deviceId, address || ('http://' + gw + '/'), inner + '<div class="browser-actions">' + button('show-portal', '回到认证页') + button('show-wifi', '无线网络') + '</div>');
  }
  function adminAccess(d) {
    return chrome(deviceId, 'http://' + gateway() + ':8090/admin-access', '<main class="auth-shell"><section class="card auth-card"><div class="logo custom-logo"><img src="assets/logo.png" alt=""></div><div class="badge">UFI-GATE · ADMINISTRATION</div><h1>管理员登录</h1><p class="muted">验证通过后，这台设备获得管理后台的访问资格，不改变联网授权和配额。</p><form id="claim-form"><div class="checks"><label><input id="forgot" type="checkbox">忘记密码，使用恢复密钥</label></div><div id="claim-pass"><label>管理员账号<input id="claim-user" autocomplete="username" value="admin"></label><label>管理员密码<input id="claim-password" type="password" autocomplete="current-password"></label></div><div id="claim-recover" hidden><label>恢复密钥<input id="claim-key" type="password"></label><label>新密码<input id="claim-new" type="password" minlength="8"></label><p class="note">演示密钥固定，不会轮换，也不会下载真实密钥文件。</p></div><button class="primary" type="submit" id="claim-submit">登录并添加此管理设备</button></form><p class="error" id="claim-error"></p><p><button type="button" class="linkish" data-action="show-portal">返回网络认证</button></p></section></main>');
  }
  function closedScreen() {
    return chrome(deviceId, 'http://' + gateway() + ':8081/', '<main class="auth-shell"><section class="card auth-card"><div class="badge">8081</div><h1>无法连接</h1><div class="help">连接已关闭。非管理员设备访问管理后台时，服务在读请求之前就断开，不会返回页面。</div>' + button('show-portal', '返回认证页', 'primary') + '</section></main>');
  }
  function renderClient() {
    document.body.className = 'sim-client';
    shell = false;
    const s = E.load();
    document.documentElement.style.setProperty('--accent', s.config.Accent || '#0067c0');
    const d = s.devices[deviceId];
    const root = $('root');
    if (!d) {
      root.innerHTML = chrome(deviceId, '', '<main class="auth-shell"><section class="card auth-card"><h1>没有这台设备的记录</h1><p class="muted">删除记录之后，下次接入会当成新访客。</p>' + button('connect', '重新接入', 'primary') + '</section></main>');
      return;
    }
    if (!d.connected || d.ui === 'wifi') {
      root.innerHTML = wifiScreen(deviceId, d);
      return;
    }
    if (d.ui === 'admin-closed') { root.innerHTML = closedScreen(); return; }
    if (d.ui === 'admin-access') { root.innerHTML = adminAccess(d); return; }
    if (d.ui === 'browser') { root.innerHTML = browserScreen(d, s); return; }
    root.innerHTML = chrome(deviceId, 'http://' + gateway() + ':8090/', portalInner(d, s));
  }

  function shellHTML(s) {
    const nav = Object.entries(names).map(([k, v]) => '<button type="button" data-nav="' + k + '">' + icon(k) + v + '</button>').join('');
    return '<div class="app"><button type="button" id="nav-backdrop" class="nav-backdrop" aria-label="关闭导航" hidden></button><aside class="sidebar" id="sidebar" aria-label="管理导航"><div class="brand"><img src="assets/logo.png" alt="">UFI-Gate<small>YOUR NETWORK, YOUR RULES</small><button type="button" id="nav-close" class="nav-close" aria-label="关闭导航">×</button></div><nav class="nav">' + nav + '</nav><footer>本地运行 · 浏览器演示<br>1.2.3<br><button type="button" id="logout">退出登录</button></footer></aside><main class="main"><header class="topbar"><div><div class="row"><button type="button" class="mobile-menu" id="menu" aria-label="打开导航">☰</button><span class="badge">NETWORK WORKSPACE</span></div><h1 id="heading"></h1><span class="muted" id="subheading"></span></div><div class="row">' + button('refresh', '刷新') + button('toggle', '认证服务', 'toggle') + '</div></header><section id="view"></section></main></div><button type="button" class="message-float" id="msg-btn">消息 <span id="unread">0</span></button>';
  }
  function loginCard() {
    return '<main class="auth-shell"><section class="card auth-card"><div class="logo custom-logo"><img src="assets/logo.png" alt=""></div><div class="badge">UFI-GATE · ADMINISTRATION</div><h1>欢迎回来</h1><p class="muted">登录后管理设备、流量和上网许可。演示账号 admin，密码 Admin1234。</p><form id="login"><label>管理员账号<input id="user" autocomplete="username" value="admin"></label><label>密码<input id="pass" type="password" autocomplete="current-password"></label><button class="primary" type="submit">登录管理后台</button></form><p><button type="button" class="linkish" data-action="forgot-admin">忘记密码</button></p><p class="error" id="login-error"></p></section></main>';
  }
  function paintHeader(s) {
    const dash = E.dashboard(s);
    $('heading').textContent = names[section] || '';
    $('subheading').textContent = dash.enabled ? '认证正在运行 · 演示数据只在这台浏览器里' : '认证尚未开启 · 当前网络在演示中保持放行';
    document.querySelectorAll('[data-nav]').forEach((b) => b.classList.toggle('active', b.dataset.nav === section));
    const toggle = document.querySelector('[data-action="toggle"]');
    if (toggle) toggle.textContent = dash.enabled ? '关闭认证' : '开启认证';
    const unread = s.messages.filter((m) => m.unread).length;
    const badge = $('unread');
    if (badge) badge.textContent = String(unread);
  }
  function metric(title, value, detail) {
    return '<article class="card metric"><div class="caption">' + esc(title) + '</div><div class="value">' + esc(value) + '</div><div class="detail">' + esc(detail) + '</div></article>';
  }
  function chart(dash) {
    const points = dash.rates || [];
    const max = Math.max(1, ...points.map((p) => Math.max(p.Up, p.Down)));
    const w = 720;
    const left = 36;
    const right = w - 12;
    const path = (k) => points.map((p, i) => (i ? 'L' : 'M') + (left + (60 - points.length + i) * (right - left) / 59).toFixed(1) + ',' + (155 - p[k] / max * 125).toFixed(1)).join(' ');
    return '<svg class="chart" viewBox="0 0 ' + w + ' 190" role="img" aria-label="最近五分钟上下行速率">' + [30, 70, 110, 155].map((y) => '<path class="gridline" d="M' + left + ' ' + y + 'H' + right + '"/>').join('') + '<text x="2" y="23">' + E.bytes(max) + '/s</text><path d="' + path('Up') + '" stroke="var(--accent)" stroke-width="2.5" fill="none"/><path d="' + path('Down') + '" stroke="#799c45" stroke-width="2.5" fill="none"/><text x="' + left + '" y="182">5 分钟前</text><text x="' + (w - 52) + '" y="182">现在</text></svg>';
  }
  function requestRow(r) {
    return '<div class="request"><div class="row between"><strong>' + esc(r.Name || '匿名访客') + '</strong>' + tag(r.Status) + '</div><p>' + esc(r.Reason || '未填写理由') + '</p><p class="tiny">' + esc(r.MAC) + ' · ' + esc(E.stamp(r.Created)) + '</p>'
      + (r.DecisionReason ? '<p class="muted">处理备注：' + esc(r.DecisionReason) + '</p>' : '')
      + (r.Status === 'pending' ? '<div class="row" data-id="' + esc(r.ID) + '">' + button('allow-request', '同意上网', 'primary') + button('deny-request', '拒绝', 'danger') + '</div>' : '')
      + '</div>';
  }
  function deviceCard(x) {
    const d = x.device;
    const pct = Math.min(100, Math.round(x.ratio * 100));
    const q = d.Quota;
    return '<article class="card device"><div class="row between"><h3>' + esc(d.Note || d.Name || '未命名设备') + '</h3>' + tag(x.status) + '</div><div class="address">' + esc(d.MAC) + '<br>' + esc(d.IP || '暂无 IP')
      + (d.Trusted ? ' · 永久信任' : '') + (x.admin ? ' · 管理员设备' : '') + (d.ManualRateKBps ? ' · 独立限速 ' + d.ManualRateKBps + ' KiB/s' : '') + (d.connected ? '' : ' · 未连接热点')
      + '</div><div class="device-stats"><span>今日 <strong>' + E.bytes(d.DayUp + d.DayDown) + '</strong></span><span>累计 <strong>' + E.bytes(d.Up + d.Down) + '</strong></span></div><div class="progress"><span class="' + (pct >= 100 ? 'bad' : pct >= 80 ? 'warn' : '') + '" style="width:' + pct + '%"></span></div><div class="row between tiny"><span>' + ((q.Daily || q.Total || q.Session || q.Period) ? '配额用量 ' + pct + '%' : '尚未设置流量上限') + '</span><span>↑ ' + E.bytes(d.RateUp) + '/s · ↓ ' + E.bytes(d.RateDown) + '/s</span></div><div class="actions" data-mac="' + esc(d.MAC) + '">'
      + button('device-allow', '放行') + button('device-edit', '配额与备注') + button('device-rate', d.ManualRateKBps ? '修改限速' : '一键限速')
      + (d.ManualRateKBps ? button('device-rate-clear', '取消独立限速') : '')
      + button(x.admin ? 'device-admin-remove' : 'device-admin-add', x.admin ? '移除管理员权限' : '设为管理员设备')
      + button('device-release', '解除限制') + button(d.Trusted ? 'device-untrust' : 'device-trust', d.Trusted ? '取消信任' : '永久信任')
      + button('device-revoke', '清除会话') + button('device-block', '封禁', 'danger') + button('device-reset', '重置流量', 'danger') + button('device-delete', '删除记录', 'danger')
      + '</div></article>';
  }
  function renderDashboard(s) {
    const dash = E.dashboard(s);
    const pending = dash.requests.filter((r) => r.Status === 'pending');
    const top = dash.devices.slice().sort((a, b) => (b.device.Up + b.device.Down) - (a.device.Up + a.device.Down)).slice(0, 5);
    const month = Math.max(dash.month, 1);
    const share = Math.min(1, dash.today / month);
    return '<div class="metrics">'
      + metric('当前活跃设备', String(dash.online), '最近有活动 · 已放行 ' + dash.approved + ' 台')
      + metric('今日用量', E.bytes(dash.today), '自然日 00:00 重置 · UTC+8')
      + metric('当前下载', E.bytes(dash.down) + '/s', '上传 ' + E.bytes(dash.up) + '/s')
      + metric('待处理申请', String(pending.length), '已有 ' + dash.limited + ' 台设备处于限速状态')
      + '</div><div class="grid dashboard-grid"><article class="card"><div class="row between"><h2>网络流量</h2><span class="muted">实时 · 最近 5 分钟</span></div>' + chart(dash) + '<div class="legend"><span><i></i>上传</span><span><i class="down"></i>下载</span></div></article><article class="card"><h2>流量概览</h2><div class="ring-row"><svg class="ring" viewBox="0 0 150 150"><circle cx="75" cy="75" r="57" stroke="var(--line)" stroke-width="12" fill="none"/><circle cx="75" cy="75" r="57" stroke="var(--accent)" stroke-width="12" fill="none" stroke-dasharray="' + (358 * share).toFixed(1) + ' 358" transform="rotate(-90 75 75)"/><text x="75" y="70" text-anchor="middle">今日占本月</text><text x="75" y="92" text-anchor="middle">' + Math.round(dash.today / month * 100) + '%</text></svg><div><p class="muted">本月 ' + E.bytes(dash.month) + '</p><p class="muted">累计 ' + E.bytes(dash.total) + '</p><p class="tiny">运行 ' + Math.floor(dash.uptime / 3600) + ' 小时 ' + Math.floor(dash.uptime % 3600 / 60) + ' 分钟</p></div></div></article><article class="card"><h2>待办审批</h2>' + (pending.length ? pending.slice(0, 5).map(requestRow).join('') : '<div class="empty">暂时没有待处理申请<br><span class="tiny">新请求会自动出现在这里</span></div>') + '</article><article class="card"><h2>用量排行</h2>' + (top.length ? top.map((x, i) => '<div class="rank"><span class="rank-name">' + (i + 1) + ' · ' + esc(x.device.Note || x.device.Name || x.device.MAC) + '</span><strong>' + E.bytes(x.device.Up + x.device.Down) + '</strong></div>').join('') : '<div class="empty">连接设备后开始统计</div>') + '<div class="row between tiny"><span>今日拦截包 ' + dash.blocked + '</span><span>黑名单拦截包 ' + dash.blackHits + '</span></div></article></div>';
  }
  function renderDevices(s) {
    const dash = E.dashboard(s);
    let items = dash.devices.filter((x) => (x.device.MAC + ' ' + x.device.IP + ' ' + x.device.Note + ' ' + x.device.Name).toLowerCase().includes(filter.toLowerCase()));
    items.sort((a, b) => sortBy === 'traffic' ? (b.device.Up + b.device.Down) - (a.device.Up + a.device.Down) : b.device.LastSeen - a.device.LastSeen);
    return '<div class="toolbar"><input id="search" placeholder="搜索 MAC、IP、备注" value="' + esc(filter) + '"><select id="sort"><option value="recent"' + (sortBy === 'recent' ? ' selected' : '') + '>最近活跃</option><option value="traffic"' + (sortBy === 'traffic' ? ' selected' : '') + '>流量从高到低</option></select>' + button('manual', '录入 MAC') + '</div><div class="device-grid">' + (items.length ? items.map(deviceCard).join('') : '<div class="card empty">暂无匹配设备<br><span class="tiny">连接热点后，设备将自动出现</span></div>') + '</div>';
  }
  function renderApprovals(s) {
    const dash = E.dashboard(s);
    return '<article class="card"><div class="row between"><h2>联网请求</h2>' + button('prune', '清理已结束记录') + '</div>' + (dash.requests.length ? dash.requests.map(requestRow).join('') : '<div class="empty">尚无上网申请</div>') + '</article>';
  }
  function renderAccounts(s) {
    const rows = s.accounts.map((a) => '<div class="request"><div class="row between"><strong>' + esc(a.Name) + '</strong><button type="button" class="danger" data-action="delete-account" data-name="' + esc(a.Name) + '">删除</button></div><p class="tiny">到期时间 ' + (a.Expires ? E.stamp(a.Expires) : '永久有效') + '</p></div>').join('');
    return '<article class="card"><div class="row between"><h2>上网账号</h2>' + button('new-account', '新建账号', 'primary') + '</div><p class="muted">这些账号用于访客联网认证，与管理员账号独立。演示把密码保存在这台浏览器里；真实服务只保存哈希。预设访客账号 guest，密码 wifi2026。</p>' + rows + (s.accounts.length ? '' : '<div class="empty">先为访客创建一个上网账号</div>') + '</article>';
  }
  function renderAuth(s) {
    const c = s.config;
    return '<article class="card"><h2>访客如何连接</h2><p class="muted">可同时开启多种认证方式，由访客自行选择。</p><div class="checks">'
      + ['account', 'captcha', 'approval', 'thirdparty'].map((m) => flag('method-' + m, { account: '账号密码', captcha: '图形验证码', approval: '审批申请', thirdparty: '第三方人机验证' }[m], c.Methods.includes(m))).join('')
      + '</div><div class="form-grid">' + input('sessionHours', '认证有效期 / 小时', c.SessionHours, 'number', 'min="1" max="720"')
      + select('captchaProvider', '验证码服务', [['hcaptcha', 'hCaptcha'], ['turnstile', 'Cloudflare Turnstile'], ['custom', '自定义兼容接口']], c.CaptchaProvider || 'hcaptcha')
      + input('portalHost', '认证页域名（可选）', c.PortalHost) + input('captchaSite', 'Site Key / AppID', c.CaptchaSite)
      + input('captchaSecret', '服务端 Secret（留空保留）', '', 'password') + input('captchaVerify', '自定义验证 API / HTTPS', c.CaptchaVerify)
      + input('captchaScript', '自定义前端脚本 / HTTPS', c.CaptchaScript) + '</div><div class="notice">第三方验证在演示里是一个本地勾选框，不会请求 hCaptcha 或 Turnstile。真实环境还要把验证服务的域名加入白名单。</div>' + settingsBar() + '</article>';
  }
  function renderPolicy(s) {
    const c = s.config;
    return '<article class="card"><h2>网址访问规则</h2><div class="form-grid">' + textarea('black', '黑名单 · 每行一项', (c.Black || []).join('\n')) + textarea('white', '免认证白名单 · 每行一项', (c.White || []).join('\n')) + '</div><div class="help">支持精确域名、*.example.com 后缀及 IPv4。黑名单优先；白名单只放行还没认证的访问。演示不解析 DNS，按你输入的主机名匹配。可以试 ads.example.com 和 captive.apple.com。</div>' + settingsBar() + '</article>';
  }
  function renderAppearance(s) {
    const c = s.config;
    return '<article class="card"><h2>让访客看到您的网络</h2><div class="form-grid">' + input('title', '认证页标题', c.Title) + input('accent', '主题色', c.Accent, 'color') + '<div class="full">' + textarea('subtitle', '欢迎语', c.Subtitle) + textarea('warning', '流量超限提示', c.Warning) + textarea('blockWarning', '网站被禁止时的提示', c.BlockWarning) + input('contact', '联系管理员', c.Contact) + '</div></div><p class="note">真实后台还可以分别上传网站 Logo 和认证页 Logo，PNG / JPG / GIF，单张不超过 32 KiB。演示使用内置的蓝色盾牌图标。</p>' + settingsBar() + '</article>';
  }
  function renderDesigner(s) {
    const c = s.config;
    const pages = [
      ['认证首页', c.Title, c.Subtitle],
      ['等待审批', '正在等待管理员', '上网请求已送达。审批结果会自动显示。'],
      ['认证成功', '连接成功', '您已通过认证，可以正常使用网络。'],
      ['访问受限', '流量已达到上限', c.Warning]
    ];
    return '<article class="card"><h2>页面设计器</h2><p class="muted">真实后台可以在认证组件前后添加文本、图片、双列、链接、公告和公共页脚，先存草稿，再发布给访客。认证表单本身不能被删掉。演示只按当前文案预览四种状态。</p><div class="grid">' + pages.map((p) => '<article class="card"><div class="badge">' + esc(p[0]) + '</div><h3>' + esc(p[1]) + '</h3><p class="muted">' + esc(p[2]) + '</p></article>').join('') + '</div></article>';
  }
  function renderPush(s) {
    const c = s.config;
    const p = c.Push;
    const r = c.RemoteApproval || { Enabled: false, PublicURL: '', Port: 8091 };
    return '<article class="card"><h2>让审批及时送达</h2><div class="form-grid">'
      + select('pushKind', '推送渠道', [['', '关闭推送'], ['wecom', '企业微信机器人'], ['feishu', '飞书群机器人'], ['webhook', '通用 Webhook'], ['smtp', 'SMTP 邮件']], p.Kind || '')
      + input('pushURL', 'Webhook / HTTPS', p.URL) + input('pushSecret', '飞书签名密钥 / Webhook Bearer（留空保留）', '', 'password')
      + input('publicBase', '局域网审批地址（留空使用网关）', p.PublicBase || '')
      + input('smtpHost', 'SMTP 主机', p.SMTPHost) + input('smtpPort', 'SMTP 端口', p.SMTPPort || 587, 'number')
      + input('smtpUser', 'SMTP 用户名', p.SMTPUser) + input('smtpPassword', 'SMTP 密码（留空保留）', '', 'password')
      + input('smtpFrom', '发件地址', p.SMTPFrom) + input('smtpTo', '收件地址', p.SMTPTo)
      + '</div><div class="notice">演示不会向企业微信、飞书、Webhook 或邮箱发出请求，只会在消息盒子和页面外侧的推送预览里显示一条本地消息。</div>' + button('push-test', '测试推送渠道（先保存）')
      + '<h2>远程审批 · 自备穿透服务</h2><div class="checks">' + flag('remoteEnabled', '启用远程审批', r.Enabled) + '</div><div class="form-grid">'
      + input('remotePublicURL', '公网 HTTPS 根地址', r.PublicURL || '') + input('remotePort', '本机端口', r.Port || 8091, 'number', 'min="1" max="65535"')
      + '</div><p class="note">真实部署要把你自己的 HTTPS 隧道转到 127.0.0.1 上的这个端口，不要转发 8081。项目不提供公共穿透。</p>' + button('remote-test', '发送远程审批测试') + settingsBar() + '</article>';
  }
  function renderSystem(s) {
    const c = s.config;
    const checked = s.config.Enabled ? c.Verified : enableVerified;
    return '<article class="card' + (highlightVerified ? '' : '') + '"><h2>服务与默认策略</h2><div class="form-grid">' + input('lan', '热点接口', c.LAN) + input('gateway', '热点 IPv4 网关', c.Gateway) + '</div><div class="checks" id="verify-box">' + flag('verified', '已核对接口、加速设置，并准备好 Root 恢复通道', checked) + '</div><p class="note">管理员设备：' + esc((c.AdminMACs || []).join('、') || '无') + '。可在设备管理中添加或移除，至少保留一台。管理员设备仍需账号密码登录。</p><h3>新设备默认配额</h3>' + quotaForm(c.DefaultQuota, 'q') + '<div class="notice">这里的配额只作用于之后新出现的设备。启用认证时，真实服务会关闭标准 tether offload，并阻断热点 IPv6。演示不会改这些系统设置。</div><div class="row">' + button('diagnostics', '运行诊断') + button('change-password', '修改管理账号') + '</div>' + settingsBar() + '</article>';
  }
  function renderUpdates() {
    return '<article class="card"><h2>服务更新</h2><div class="update-facts"><div><div class="tiny">当前版本</div><strong>1.2.3</strong></div><div><div class="tiny">架构</div><strong>浏览器演示</strong></div></div><p class="muted">真实后台会按直连 GitHub、再试镜像的顺序检查发行版，校验签名后才允许安装，并在失败时恢复旧版本。演示不会下载安装包。</p><div class="settings-actions">' + button('check-update', '检查更新', 'primary') + '</div><div class="update-notes"><h3>1.2.3</h3><p>新增门户页面设计器，让管理员可视化调整认证首页、等待审批、认证成功和访问受限页面。这一版不包含广告。</p></div></article>';
  }
  function renderLogs(s) {
    const rows = s.logs.slice().reverse().map((l) => '<div class="log-row"><div class="muted">' + esc(E.stamp(l.Time)) + '</div><div>' + esc(l.Kind) + '</div><div>' + esc(l.Message) + '</div></div>').join('');
    return '<article class="card"><h2>运行记录</h2><p class="muted">保留最近 500 条。这里不会出现密码或推送密钥。</p>' + rows + '</article>';
  }
  function renderAbout(s) {
    const dash = E.dashboard(s);
    return '<article class="card"><div class="about-hero"><img class="about-logo" src="assets/logo.png" alt=""><h2>UFI-Gate</h2><p class="muted">随身 WiFi 网络准入与流量配额管理</p></div><div class="about-grid"><div><div class="caption">当前版本</div><div class="value">v' + esc(dash.version) + '</div></div><div><div class="caption">架构</div><div class="value">演示</div></div><div><div class="caption">运行时间</div><div class="value">' + Math.floor(dash.uptime / 3600) + ' 小时 ' + Math.floor(dash.uptime % 3600 / 60) + ' 分</div></div><div><div class="caption">认证服务</div><div class="value">' + (dash.enabled ? '运行中' : '已关闭') + '</div></div></div><h3>项目资源</h3><div class="about-links"><a href="https://github.com/nanbeiluduo/UFI-Gate" target="_blank" rel="noopener">GitHub 仓库 ↗</a><a href="https://github.com/nanbeiluduo/UFI-Gate/releases" target="_blank" rel="noopener">发布历史 ↗</a><a href="https://github.com/nanbeiluduo/UFI-Gate/issues" target="_blank" rel="noopener">问题反馈 ↗</a><a href="https://t.me/nanbeiluduo" target="_blank" rel="noopener">Telegram ↗</a></div><p class="note">这一页跑在浏览器演示里，架构不会显示 arm64 或 arm32。</p></article>';
  }
  function settingsBar() {
    return '<div class="settings-actions">' + button('save-settings', '保存设置', 'primary') + button('show-portal', '打开认证页面') + '</div>';
  }
  function paintSection(s) {
    const viewEl = $('view');
    if (!viewEl) return;
    let html = '';
    if (section === 'dashboard') html = renderDashboard(s);
    else if (section === 'devices') html = renderDevices(s);
    else if (section === 'approvals') html = renderApprovals(s);
    else if (section === 'accounts') html = renderAccounts(s);
    else if (section === 'auth') html = renderAuth(s);
    else if (section === 'policy') html = renderPolicy(s);
    else if (section === 'appearance') html = renderAppearance(s);
    else if (section === 'designer') html = renderDesigner(s);
    else if (section === 'push') html = renderPush(s);
    else if (section === 'system') html = renderSystem(s);
    else if (section === 'updates') html = renderUpdates();
    else if (section === 'logs') html = renderLogs(s);
    else if (section === 'about') html = renderAbout(s);
    viewEl.innerHTML = html;
    if (highlightVerified && section === 'system') {
      const box = $('verify-box');
      if (box) box.classList.add('guide-flash');
    }
  }
  function renderAdmin() {
    document.body.className = 'admin-page';
    const s = E.load();
    document.documentElement.style.setProperty('--accent', s.config.Accent || '#0067c0');
    if (!s.loggedIn) {
      shell = false;
      $('root').innerHTML = loginCard();
      return;
    }
    if (!shell || !$('view')) {
      $('root').innerHTML = shellHTML(s);
      shell = true;
    }
    paintHeader(s);
    paintSection(s);
  }
  function render() {
    if (view === 'admin') renderAdmin();
    else renderClient();
  }
  function closeNav() {
    const bar = $('sidebar');
    if (!bar) return;
    bar.classList.remove('open');
    document.body.classList.remove('nav-open');
    const backdrop = $('nav-backdrop');
    if (backdrop) backdrop.hidden = true;
  }
  function openMessages() {
    const s = E.load();
    const box = $('mbox');
    const items = s.messages.length ? s.messages.map((m) => {
      const actions = m.requestId ? '<div class="row" data-id="' + esc(m.requestId) + '">' + button('allow-request', '同意', 'primary') + button('deny-request', '拒绝', 'danger') + '</div>' : '';
      return '<article class="message-item' + (m.unread ? ' unread' : '') + '"><strong>' + esc(m.title) + '</strong><p class="tiny">' + esc(E.channelName(m.channel)) + ' · ' + esc(E.stamp(m.time)) + '</p><p>' + esc(m.body) + '</p>' + actions + '</article>';
    }).join('') : '<div class="empty">没有消息</div>';
    box.innerHTML = '<div class="row between"><h2>消息</h2><button type="button" id="mbox-close">关闭</button></div><div class="row">' + button('msg-read', '全部已读') + button('msg-clear-read', '删除已读') + button('msg-clear-all', '清空') + '</div>' + items;
    box.querySelector('#mbox-close').onclick = () => box.close();
    if (!box.open) box.showModal();
  }
  async function onAction(action, el) {
    const s = E.load();
    if (action === 'refresh') { render(); toast('已刷新'); return; }
    if (action === 'toggle') {
      if (!s.config.Enabled) {
        if (!enableVerified) {
          section = 'system';
          highlightVerified = true;
          render();
          toast('请先勾选本次确认项，再开启认证');
          return;
        }
        if (!await confirmBox('启用将阻断未认证设备和热点 IPv6。请先核对接口，并准备 Root 恢复通道。继续？', '确认开启')) return;
        enableVerified = false;
        highlightVerified = false;
        const enabled = run(E.toggleAuth(true));
        if (enabled.ok) toast('认证服务状态已更新');
        return;
      }
      if (!await confirmBox('关闭认证将恢复原有网络准入。是否继续？', '确认关闭')) return;
      const disabled = run(E.toggleAuth(false));
      if (disabled.ok) toast('认证服务状态已更新');
      return;
    }
    if (action === 'show-portal') { tellParent('phone'); return; }
    if (action === 'save-settings') { saveSettings(); return; }
    if (action === 'push-test') { run(E.pushTest()); toast('测试已写入演示收件箱'); return; }
    if (action === 'remote-test') { run(E.remoteTest()); return; }
    if (action === 'check-update') { toast('演示不会连接更新服务器'); return; }
    if (action === 'diagnostics') {
      infoDialog('服务诊断', '<pre>' + esc(JSON.stringify({
        demo: true, enabled: s.config.Enabled, lan: s.config.LAN, gateway: s.config.Gateway,
        admin: s.config.AdminMACs, devices: Object.keys(s.devices).length,
        note: '演示不探测网卡，也不写入防火墙。'
      }, null, 2)) + '</pre>');
      return;
    }
    if (action === 'change-password') {
      const ok = await formDialog('修改管理员账号', input('adminUser', '管理员账号', s.admin.Name) + input('adminPass', '新密码', '', 'password', 'minlength="8"'), () => {
        const res = E.changePassword($('adminUser').value, $('adminPass').value);
        if (!res.ok) throw new Error(res.error);
      });
      if (ok) { toast('管理员账号已更新'); render(); }
      return;
    }
    if (action === 'manual') {
      const ok = await formDialog('手动录入设备', input('newMAC', 'MAC 地址', '', 'text', 'placeholder="02:00:00:00:00:01"') + select('manualAction', '初始操作', [['trust', '永久信任'], ['block', '预封禁'], ['allow', '预授权']], 'trust'), () => {
        const res = E.deviceAction($('newMAC').value, 'manual', { op: $('manualAction').value });
        if (!res.ok) throw new Error(res.error);
      });
      if (ok) { toast('设备已录入'); render(); }
      return;
    }
    if (action === 'prune') {
      if (await confirmBox('清理已结束和过期的审批记录？', '确认清理')) run(E.prune());
      return;
    }
    if (action === 'new-account') {
      const ok = await formDialog('创建上网账号', input('newUser', '账号', '', 'text', 'required maxlength="64"') + input('newPass', '密码', '', 'password', 'required minlength="8"') + input('expires', '到期时间（可不填）', '', 'datetime-local'), () => {
        const raw = $('expires').value;
        const expires = raw ? Math.floor(new Date(raw).getTime() / 1000) : 0;
        const res = E.addAccount($('newUser').value, $('newPass').value, expires);
        if (!res.ok) throw new Error(res.error);
      });
      if (ok) { toast('上网账号已创建'); render(); }
      return;
    }
    if (action === 'delete-account') {
      const name = el.dataset.name;
      if (await confirmBox('删除上网账号 ' + name + '？', '确认删除')) run(E.deleteAccount(name));
      return;
    }
    if (action === 'allow-request' || action === 'deny-request') {
      const id = el.closest('[data-id]').dataset.id;
      const allow = action === 'allow-request';
      const ok = await formDialog(allow ? '同意上网' : '拒绝申请', input('decisionReason', '处理备注', ''), () => {
        const res = E.decide(id, allow ? 'allow' : 'deny', $('decisionReason').value);
        if (!res.ok) throw new Error(res.error);
        if (res.detail && res.detail.notice) throw new Error(res.detail.notice);
      });
      if (ok) {
        toast('审批已完成');
        render();
        if ($('mbox').open) openMessages();
      }
      return;
    }
    if (action === 'msg-read') { run(E.markMessages('read')); openMessages(); return; }
    if (action === 'msg-clear-read') { run(E.markMessages('clear-read')); openMessages(); return; }
    if (action === 'msg-clear-all') { run(E.markMessages('clear-all')); openMessages(); return; }
    if (action.startsWith('device-')) {
      const mac = el.closest('[data-mac]').dataset.mac;
      const op = action.slice(7);
      const row = E.dashboard(s).devices.find((x) => x.device.MAC === mac);
      const d = row.device;
      if (op === 'rate') {
        const ok = await formDialog('独立限速', input('manualRate', '上传 / 下载限速（各方向）· KiB/s', d.ManualRateKBps || 128, 'number', 'min="1" max="102400"') + '<p class="note">与配额限速同时存在时取较低值。1024 KiB/s 约等于 8 Mbps。不会授予联网权限。</p>', () => {
          const res = E.deviceAction(mac, 'rate', { rate: $('manualRate').value });
          if (!res.ok) throw new Error(res.error);
        });
        if (ok) { toast('独立限速已设置'); render(); }
        return;
      }
      if (op === 'edit') {
        const ok = await formDialog('设备配额与备注', input('note', '备注', d.Note) + quotaForm(d.Quota, 'e'), () => {
          const res = E.deviceAction(mac, 'edit', { note: $('note').value, quota: readQuota('e') });
          if (!res.ok) throw new Error(res.error);
        });
        if (ok) { toast('设备设置已保存'); render(); }
        return;
      }
      if (op === 'admin-add' || op === 'admin-remove') {
        const text = op === 'admin-add' ? '允许此设备访问管理后台？仍需使用管理员账号密码登录，联网授权保持不变。' : '移除此设备的管理权限？联网授权保持不变。至少要留下一台管理员设备。';
        if (!await confirmBox(text, '管理权限')) return;
        run(E.deviceAction(mac, op));
        return;
      }
      if (op === 'delete') {
        if (!await confirmBox('删除该设备的连接记录？下次接入会当成新访客。管理员权限不会一起删除。', '确认删除')) return;
        run(E.deviceAction(mac, 'delete'));
        return;
      }
      const labels = { block: '封禁设备', release: '解除限制并给予新一轮配额', reset: '重置全部流量计数', trust: '设为永久信任', untrust: '取消永久信任', revoke: '清除会话', 'rate-clear': '取消独立限速' };
      if (labels[op] && !await confirmBox(labels[op] + '？', labels[op])) return;
      run(E.deviceAction(mac, op));
      if (op !== 'allow') toast('设备操作已完成');
      return;
    }
  }
  function saveSettings() {
    const s = E.load();
    let values = {};
    try {
      if (section === 'auth') {
        values = {
          Methods: ['account', 'captcha', 'approval', 'thirdparty'].filter((m) => $('method-' + m).checked),
          SessionHours: $('sessionHours').value,
          CaptchaProvider: $('captchaProvider').value,
          PortalHost: $('portalHost').value,
          CaptchaSite: $('captchaSite').value,
          CaptchaSecret: $('captchaSecret').value,
          CaptchaVerify: $('captchaVerify').value,
          CaptchaScript: $('captchaScript').value
        };
      } else if (section === 'policy') {
        values = { Black: $('black').value, White: $('white').value };
      } else if (section === 'appearance') {
        values = {
          Title: $('title').value, Subtitle: $('subtitle').value, Warning: $('warning').value,
          BlockWarning: $('blockWarning').value, Contact: $('contact').value, Accent: $('accent').value
        };
      } else if (section === 'push') {
        values = {
          Push: {
            Kind: $('pushKind').value, URL: $('pushURL').value, Secret: $('pushSecret').value, PublicBase: $('publicBase').value,
            SMTPHost: $('smtpHost').value, SMTPPort: $('smtpPort').value, SMTPUser: $('smtpUser').value,
            SMTPPassword: $('smtpPassword').value, SMTPFrom: $('smtpFrom').value, SMTPTo: $('smtpTo').value
          },
          RemoteApproval: { Enabled: $('remoteEnabled').checked, PublicURL: $('remotePublicURL').value, Port: $('remotePort').value }
        };
      } else if (section === 'system') {
        values = { LAN: $('lan').value, Gateway: $('gateway').value, Verified: $('verified').checked, DefaultQuota: readQuota('q') };
        enableVerified = $('verified').checked;
      } else return;
    } catch (err) {
      toast(err.message || String(err));
      return;
    }
    const res = E.saveSection(section, values);
    if (!res.ok) { toast(res.error); return; }
    highlightVerified = false;
    toast('已保存');
    render();
  }
  function submitAuth(e) {
    if (e) e.preventDefault();
    const s = E.load();
    const d = s.devices[deviceId];
    if (!d) return;
    let res;
    if (method === 'account') res = E.authAccount(deviceId, $('username').value, $('password').value);
    else if (method === 'captcha') {
      res = E.authCaptcha(deviceId, $('answer').value);
      if (!res.ok) E.renewCaptcha(deviceId);
    } else if (method === 'approval') res = E.requestApproval(deviceId, $('name').value, $('reason').value);
    else res = E.authThird(deviceId, $('robot') && $('robot').checked);
    if (!res.ok) toast(res.error);
    render();
  }
  function submitClaim(e) {
    e.preventDefault();
    const forgot = $('forgot').checked;
    let res;
    if (forgot) res = E.recover($('claim-key').value, $('claim-new').value);
    else {
      res = E.login($('claim-user').value, $('claim-password').value);
      if (res.ok) {
        E.logout();
        res = E.markAdmin(deviceId);
        if (res.ok) toast('这台设备已可以打开管理后台，联网授权没有变化');
      }
    }
    if (!res.ok) {
      $('claim-error').textContent = res.error;
      return;
    }
    if (forgot) {
      $('claim-error').textContent = '密码已重置。演示密钥仍然是 ' + E.RECOVERY_KEY;
      return;
    }
    E.setUI(deviceId, 'portal');
    render();
  }

  document.body.addEventListener('click', (e) => {
    const theme = e.target.closest('[data-method]');
    if (theme) { method = theme.dataset.method; render(); return; }
    if (e.target.id === 'captcha') { E.renewCaptcha(deviceId); render(); return; }
    const nav = e.target.closest('[data-nav]');
    if (nav) { section = nav.dataset.nav; closeNav(); render(); return; }
    if (e.target.id === 'menu') {
      $('sidebar').classList.add('open');
      document.body.classList.add('nav-open');
      $('nav-backdrop').hidden = false;
      return;
    }
    if (e.target.id === 'nav-close' || e.target.id === 'nav-backdrop') { closeNav(); return; }
    if (e.target.id === 'logout') { shell = false; run(E.logout()); return; }
    if (e.target.id === 'msg-btn' || e.target.closest('#msg-btn')) { openMessages(); return; }
    const actionEl = e.target.closest('[data-action]');
    if (!actionEl) return;
    const action = actionEl.dataset.action;
    if (view === 'client') {
      if (action === 'connect') { run(E.connect(deviceId)); return; }
      if (action === 'show-wifi') { run(E.setUI(deviceId, 'wifi')); return; }
      if (action === 'show-portal') { run(E.setUI(deviceId, 'portal')); return; }
      if (action === 'browse-open') { run(E.visit(deviceId, 'example.com')); return; }
      if (action === 'admin-access') { run(E.setUI(deviceId, 'admin-access')); return; }
      if (action === 'open-backend') {
        const s = E.load();
        const d = s.devices[deviceId];
        if (d && E.isAdmin(s, d.MAC)) tellParent('admin');
        else run(E.setUI(deviceId, 'admin-closed'));
        return;
      }
      return;
    }
    onAction(action, actionEl);
  });
  document.body.addEventListener('submit', (e) => {
    if (e.target.id === 'authform') submitAuth(e);
    else if (e.target.id === 'login') {
      e.preventDefault();
      const res = E.login($('user').value, $('pass').value);
      if (!res.ok) $('login-error').textContent = res.error;
      else render();
    } else if (e.target.id === 'claim-form') submitClaim(e);
  });
  document.body.addEventListener('change', (e) => {
    if (e.target.id === 'verified') enableVerified = e.target.checked;
    if (e.target.id === 'forgot') {
      $('claim-recover').hidden = !e.target.checked;
      $('claim-pass').hidden = e.target.checked;
      $('claim-submit').textContent = e.target.checked ? '重置密码' : '登录并添加此管理设备';
    }
    if (e.target.id === 'sort') { sortBy = e.target.value; paintSection(E.load()); }
  });
  document.body.addEventListener('input', (e) => {
    if (e.target.id !== 'search') return;
    filter = e.target.value;
    const pos = e.target.selectionStart;
    paintSection(E.load());
    const box = $('search');
    if (box) { box.focus(); box.setSelectionRange(pos, pos); }
  });
  window.addEventListener('message', (e) => {
    if (e.origin !== location.origin || !e.data) return;
    if (e.data.type === 'ufi-theme') {
      if (e.data.theme === 'light' || e.data.theme === 'dark') document.documentElement.setAttribute('data-theme', e.data.theme);
      else document.documentElement.removeAttribute('data-theme');
      return;
    }
    if (e.data.type !== 'ufi-demo-state') return;
    if ($('dialog') && $('dialog').open) return;
    const active = document.activeElement;
    if (active && active.closest && active.closest('form, .dialog-body')) return;
    render();
  });
  render();
})();
