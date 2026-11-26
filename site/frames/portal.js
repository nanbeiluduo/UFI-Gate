(function () {
  'use strict';
  const D = window.UFIDemo;
  const params = new URLSearchParams(location.search);
  const clientId = ['phone', 'tablet', 'pc'].includes(params.get('client')) ? params.get('client') : 'phone';
  const root = document.getElementById('root');
  const labels = { account: '账号登录', captcha: '图形验证码', approval: '申请上网', thirdparty: '人机验证' };
  let method = '';

  function clientOf(state) {
    return state.clients[clientId];
  }

  function paintAccent(state) {
    document.documentElement.style.setProperty('--accent', state.config.accent || '#0067c0');
  }

  function logo() {
    return '<div class="logo custom-logo"><img src="../assets/logo.png" alt="网络 Logo"></div>';
  }

  function shell(title, subtitle, content, error) {
    return '<main class="auth-shell"><section class="card auth-card">' + logo() +
      '<div class="badge">UFI-GATE · NETWORK ACCESS</div>' +
      '<h1>' + D.esc(title) + '</h1>' +
      '<p class="muted">' + D.esc(subtitle) + '</p>' +
      '<div id="content">' + content + '</div>' +
      '<div class="error" id="error" role="alert">' + D.esc(error || '') + '</div>' +
      '<p><button type="button" class="button" id="admin-login">管理员登录</button></p>' +
      '<div class="footnote">共享网络 · 安心连接<br>UFI-Gate · 浏览器演示</div>' +
      '</section></main>';
  }

  function drawCaptcha(code) {
    const canvas = document.getElementById('captcha');
    if (!canvas || !code) return;
    const ctx = canvas.getContext('2d');
    const dark = document.documentElement.getAttribute('data-theme') === 'dark' ||
      (!document.documentElement.getAttribute('data-theme') && matchMedia('(prefers-color-scheme: dark)').matches);
    canvas.width = 180;
    canvas.height = 60;
    ctx.fillStyle = dark ? '#1c2836' : '#eef3f9';
    ctx.fillRect(0, 0, 180, 60);
    for (let i = 0; i < 7; i++) {
      ctx.strokeStyle = dark ? 'rgba(120,170,220,.35)' : 'rgba(0,103,192,.28)';
      ctx.beginPath();
      ctx.moveTo(Math.random() * 180, Math.random() * 60);
      ctx.lineTo(Math.random() * 180, Math.random() * 60);
      ctx.stroke();
    }
    ctx.font = '700 28px "Segoe UI", "WenQuanYi Micro Hei", sans-serif';
    ctx.fillStyle = dark ? '#e8edf4' : '#17263b';
    ctx.fillText(code, 32, 40);
  }

  function bindAdmin() {
    const btn = document.getElementById('admin-login');
    if (btn) btn.onclick = () => parent.postMessage({ type: 'ufi-demo', action: 'show', frame: 'admin' }, location.origin);
  }

  function render(state) {
    paintAccent(state);
    const c = clientOf(state);
    const cfg = state.config;
    const phaseChanged = root.dataset.phase !== (c.connected ? c.phase : 'offline');
    root.dataset.phase = c.connected ? c.phase : 'offline';
    root.dataset.anim = phaseChanged ? '1' : '0';

    if (!c.connected) {
      root.innerHTML = shell('尚未连接热点', '连接到开放网络 UFI-WiFi 后，认证页会在这里打开。',
        '<button class="primary" id="connect" type="button">连接 UFI-WiFi</button><p class="note">演示热点无密码。连接不会影响你正在用的网络。</p>', '');
      document.getElementById('connect').onclick = () => render(D.update(s => D.connect(s, clientId)));
      bindAdmin();
      return;
    }

    if (c.phase === 'whitelist') {
      root.innerHTML = shell('已放行该地址', c.visit + ' 在免认证白名单中。',
        '<div class="help">未完成认证也可以访问白名单里的域名和 IPv4。其他网站仍会回到认证页。</div>' +
        '<button class="primary" id="back" type="button">返回</button>', '');
      document.getElementById('back').onclick = () => render(D.update(s => D.leaveSpecial(s, clientId)));
      bindAdmin();
      return;
    }

    if (c.phase === 'black') {
      root.innerHTML = shell('此网站暂不可访问', c.visit || '',
        '<div class="help">' + D.esc(cfg.blockWarning) + '</div><p class="muted">' + D.esc(cfg.contact) + '</p>' +
        '<button class="primary" id="back" type="button">返回</button>', '');
      document.getElementById('back').onclick = () => render(D.update(s => D.leaveSpecial(s, clientId)));
      bindAdmin();
      return;
    }

    if (!cfg.enabled || c.phase === 'open') {
      root.innerHTML = shell(cfg.title, '网络认证未开启。',
        '<div class="help">网络认证未开启，您可以直接使用网络。</div>', '');
      bindAdmin();
      return;
    }

    const dev = D.findDevice(state, c.mac);
    if (c.phase === 'blocked') {
      root.innerHTML = shell('设备暂不可上网', cfg.subtitle,
        '<div class="help">该设备已被管理员限制，请联系管理员。</div><p>' + D.esc(cfg.contact) + '</p>', '');
      bindAdmin();
      return;
    }
    if (c.phase === 'quota') {
      const used = dev ? D.bytes(dev.dayUp + dev.dayDown) : '0 B';
      const total = dev ? D.bytes(dev.up + dev.down) : '0 B';
      root.innerHTML = shell('流量已达到上限', cfg.subtitle,
        '<div class="help">' + D.esc(cfg.warning) + '</div><p class="muted">今日已用 ' + used + ' · 累计 ' + total + '</p><p>' + D.esc(cfg.contact) + '</p>', '');
      bindAdmin();
      return;
    }
    if (c.phase === 'online' || c.phase === 'limited') {
      const used = dev ? D.bytes(D.usedDay(dev)) : '0 B';
      const total = dev ? D.bytes(dev.up + dev.down) : '0 B';
      const q = dev ? D.quotaOf(dev, cfg) : cfg.defaultQuota;
      const pct = dev ? Math.min(100, Math.round(D.ratio(dev, cfg) * 100)) : 0;
      const limited = c.phase === 'limited';
      root.innerHTML = shell(limited ? '已连接 · 当前限速' : '连接成功', '网络已为您准备好。',
        '<div class="success-icon">✓</div><p>' + D.esc(limited
          ? (dev && dev.manualRateKBps ? '管理员已设置设备限速，当前可以正常联网。' : cfg.warning)
          : '您已通过认证，可以正常使用网络。') + '</p>' +
        '<div class="progress"><span class="' + (pct >= 100 ? 'bad' : pct >= 80 ? 'warn' : '') + '" style="width:' + pct + '%"></span></div>' +
        '<p class="muted">今日已用 ' + used + '<br>累计 ' + total + (q.dailyMiB ? ' · 日配额 ' + q.dailyMiB + ' MiB' : '') + '</p>' +
        '<button class="primary" id="done" type="button">完成</button>', '');
      document.getElementById('done').onclick = () => render(state);
      bindAdmin();
      return;
    }
    if (c.phase === 'waiting') {
      root.innerHTML = shell('正在等待管理员', '上网请求已送达。',
        '<div class="help">上网请求已送达。审批结果会自动显示，无需重复申请。</div><p class="muted">' + D.esc(cfg.contact) + '</p>', '');
      bindAdmin();
      return;
    }

    const methods = (cfg.methods || []).slice();
    if (!methods.length) {
      root.innerHTML = shell(cfg.title, cfg.subtitle, '<div class="help">管理员还没有开放认证方式。</div>', c.error);
      bindAdmin();
      return;
    }
    if (!methods.includes(method)) method = methods[0];
    const tabs = methods.map(m => '<button type="button" data-method="' + m + '" class="' + (m === method ? 'active' : '') + '">' + labels[m] + '</button>').join('');
    let fields = '';
    if (method === 'account') {
      fields = '<label>上网账号<input id="username" autocomplete="username" required maxlength="64" placeholder="输入管理员提供的账号"></label>' +
        '<label>密码<input id="password" type="password" autocomplete="current-password" required maxlength="128" placeholder="输入密码"></label>';
    } else if (method === 'captcha') {
      fields = '<label>输入图片中的字符</label><div class="captcha"><canvas id="captcha" width="180" height="60" aria-label="图形验证码，点击更换"></canvas><input id="answer" required maxlength="8" autocomplete="off" placeholder="验证码"></div>';
    } else if (method === 'approval') {
      fields = '<label>您的称呼<input id="name" required maxlength="100" placeholder="方便管理员识别您"></label>' +
        '<label>申请理由<input id="reason" maxlength="1000" placeholder="例如：临时上网、查阅资料"></label>';
    } else {
      fields = '<div class="help"><label class="checkline"><input id="robot" type="checkbox" ' + (c.thirdOk ? 'checked' : '') + '>我不是机器人</label>' +
        '<p class="note">演示不会加载 ' + D.esc(cfg.captchaProvider || 'hcaptcha') + '。真实门户会在这里渲染第三方验证组件。</p></div>';
    }
    root.innerHTML = shell(cfg.title, cfg.subtitle,
      '<div class="tabs">' + tabs + '</div><form id="authform">' + fields +
      '<button class="primary" id="submit" type="submit">' + (method === 'approval' ? '发送上网请求' : '验证并连接') + '</button></form>',
      c.error);
    root.querySelectorAll('[data-method]').forEach(btn => {
      btn.onclick = () => { method = btn.dataset.method; render(D.load()); };
    });
    if (method === 'captcha') {
      if (!c.captcha || c.captcha.exp < Date.now()) {
        render(D.update(s => D.issueCaptcha(s, clientId)));
        return;
      }
      drawCaptcha(c.captcha.code);
      document.getElementById('captcha').onclick = () => render(D.update(s => D.issueCaptcha(s, clientId)));
    }
    const robot = document.getElementById('robot');
    if (robot) {
      robot.onchange = () => D.update(s => { s.clients[clientId].thirdOk = robot.checked; s.clients[clientId].error = ''; });
    }
    document.getElementById('authform').onsubmit = event => {
      event.preventDefault();
      const submit = document.getElementById('submit');
      submit.disabled = true;
      try {
        const next = D.update(s => {
          const current = s.clients[clientId];
          if (method === 'approval') {
            const name = document.getElementById('name').value.trim();
            if (!name) { current.error = '请填写称呼'; return; }
            D.requestAccess(s, clientId, name, document.getElementById('reason').value);
          } else if (method === 'account') {
            D.authAccount(s, clientId, document.getElementById('username').value.trim(), document.getElementById('password').value);
          } else if (method === 'captcha') {
            D.authCaptcha(s, clientId, document.getElementById('answer').value);
          } else {
            D.authThird(s, clientId);
          }
        });
        render(next);
      } finally {
        const again = document.getElementById('submit');
        if (again) again.disabled = false;
      }
    };
    bindAdmin();
  }

  let lastKey = '';
  function keyOf(state) {
    const c = clientOf(state);
    const dev = D.findDevice(state, c.mac);
    const used = dev ? D.usedDay(dev) + ':' + (dev.up + dev.down) : '0';
    return [
      c.connected, c.phase, c.error, c.visit, c.thirdOk,
      c.captcha ? c.captcha.code : '',
      (state.config.methods || []).join(','),
      state.config.enabled, state.config.title, state.config.subtitle, state.config.accent,
      state.config.warning, state.config.blockWarning, state.config.contact,
      used
    ].join('|');
  }

  function maybeRender(state) {
    const key = keyOf(state);
    if (key === lastKey) return;
    const typing = document.activeElement && ['INPUT', 'TEXTAREA'].includes(document.activeElement.tagName);
    const c = clientOf(state);
    if (typing && c.connected && c.phase === 'portal' && !c.error) return;
    lastKey = key;
    render(state);
  }

  document.addEventListener('focusout', () => setTimeout(() => maybeRender(D.load()), 0));
  D.subscribe(maybeRender);
  render(D.load());
  window.addEventListener('storage', event => {
    if (event.key !== 'ufi-site-theme') return;
    const theme = event.newValue;
    if (theme === 'light' || theme === 'dark') document.documentElement.setAttribute('data-theme', theme);
    else document.documentElement.removeAttribute('data-theme');
    const c = clientOf(D.load());
    if (c.captcha) drawCaptcha(c.captcha.code);
  });
})();
