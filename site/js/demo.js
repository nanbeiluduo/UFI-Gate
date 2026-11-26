(function () {
  const E = window.UFIDemo;
  const MODES = {
    phone: { w: 390, h: 844, label: '手机 · 系统无线网络 / 认证门户', view: 'client', device: 'phone' },
    tablet: { w: 834, h: 1080, label: '平板 · 认证门户', view: 'client', device: 'tablet' },
    pc: { w: 1120, h: 720, label: '电脑 · 桌面浏览器里的认证页', view: 'client', device: 'pc' },
    admin: { w: 1360, h: 820, label: '管理后台 · 8081', view: 'admin', device: '' }
  };
  let mode = 'phone';
  const frame = document.getElementById('screen');
  const bezel = document.getElementById('bezel');
  const wrap = document.getElementById('bezel-wrap');
  const stage = document.getElementById('stage');

  function targetId() { return mode === 'admin' ? 'phone' : mode; }
  function layout() {
    const spec = MODES[mode];
    const pad = mode === 'phone' || mode === 'tablet' ? 12 : 0;
    const outerW = spec.w + pad * 2;
    const outerH = spec.h + pad * 2;
    frame.style.width = spec.w + 'px';
    frame.style.height = spec.h + 'px';
    bezel.style.width = outerW + 'px';
    bezel.style.height = outerH + 'px';
    bezel.dataset.kind = mode;
    const maxW = Math.max(280, stage.clientWidth - 28);
    const maxH = Math.max(460, stage.clientHeight - 28);
    const scale = Math.min(1, maxW / outerW, maxH / outerH);
    bezel.style.transform = 'scale(' + scale + ')';
    wrap.style.width = Math.round(outerW * scale) + 'px';
    wrap.style.height = Math.round(outerH * scale) + 'px';
  }
  function sendTheme() {
    const theme = document.documentElement.getAttribute('data-theme') || 'auto';
    try { frame.contentWindow.postMessage({ type: 'ufi-theme', theme: theme }, location.origin); } catch (e) { /* ignore */ }
  }
  function show(next) {
    if (!MODES[next]) next = 'phone';
    const changed = next !== mode || !frame.getAttribute('src');
    mode = next;
    document.querySelectorAll('[data-mode]').forEach((button) => {
      const on = button.dataset.mode === mode;
      button.setAttribute('aria-pressed', on ? 'true' : 'false');
      button.classList.toggle('active', on);
    });
    document.getElementById('frame-label').textContent = MODES[mode].label;
    if (changed) {
      const spec = MODES[mode];
      const query = spec.view === 'admin' ? 'view=admin' : 'view=client&device=' + spec.device;
      frame.src = 'sim.html?' + query;
    }
    const url = new URL(location.href);
    url.searchParams.set('mode', mode);
    history.replaceState(null, '', url);
    layout();
    refresh();
  }
  function status(text) {
    const el = document.getElementById('rail-status');
    if (el) el.textContent = text || '';
  }
  function refresh() {
    const s = E.load();
    const id = targetId();
    const device = s.devices[id];
    const connect = document.getElementById('act-connect');
    const who = document.getElementById('act-who');
    if (who) who.textContent = mode === 'admin' ? '下面的操作作用在「我的手机」上。管理后台本身不代表某一台访客。' : '当前画面里的设备是「' + ((device && device.Name) || '这台设备') + '」。';
    if (connect) connect.textContent = device && device.connected ? '断开热点' : '连接热点';
    const steps = document.getElementById('steps');
    if (steps) {
      const phone = s.devices.phone;
      const phoneState = phone ? E.deviceState(phone) : 'pending';
      const pending = s.requests.some((r) => r.Status === 'pending' && r.Expires * 1000 > Date.now());
      const items = [
        [s.config.Enabled && s.config.Methods.length > 0, '认证开着，并且至少有一种认证方式'],
        [!!(phone && phone.connected), '手机连上了热点 UFI-Gate'],
        [!!(phone && (phoneState === 'online' || phoneState === 'limited' || phoneState === 'quota' || pending || phone.ui === 'portal' && phone.connected)), '手机看到认证门户，并提交了认证或审批'],
        [s.requests.some((r) => r.Status === 'allow' || r.Status === 'deny') || phoneState === 'online' || phoneState === 'limited', '审批已处理，或手机用账号 / 验证码直接通过'],
        [!!(phone && phone.Up + phone.Down > 0), '给手机模拟过流量，配额有变化']
      ];
      steps.innerHTML = items.map(([done, text]) => '<li class="' + (done ? 'done' : '') + '">' + text + '</li>').join('');
    }
    const inbox = document.getElementById('inbox');
    if (inbox) {
      const rows = s.messages.slice(0, 4);
      inbox.innerHTML = rows.length ? rows.map((m) => {
        const actions = m.requestId && s.requests.some((r) => r.ID === m.requestId && r.Status === 'pending')
          ? '<div class="row"><button type="button" data-decide="allow" data-id="' + m.requestId + '">同意</button><button type="button" data-decide="deny" data-id="' + m.requestId + '">拒绝</button></div>'
          : '';
        return '<article><strong>' + escapeHTML(m.title) + '</strong><p>' + escapeHTML(m.body) + '</p>' + actions + '</article>';
      }).join('') : '<p class="quiet">还没有推送。用手机提交「申请上网」后，会在这里和管理后台的消息盒子里各出现一条。</p>';
    }
    const authLine = document.getElementById('auth-line');
    if (authLine) {
      const labels = { account: '账号密码', captcha: '图形验证码', approval: '审批', thirdparty: '人机验证' };
      authLine.textContent = (s.config.Enabled ? '认证已开启' : '认证已关闭') + ' · ' + (s.config.Methods.map((m) => labels[m] || m).join('、') || '未选择方式');
    }
  }
  function escapeHTML(value) {
    return String(value ?? '').replace(/[&<>"']/g, (ch) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch]));
  }
  function act(res, okText) {
    if (!res.ok) status(res.error);
    else if (res.detail && res.detail.dropped) status('流量被准入策略丢弃了。先让这台设备通过认证。');
    else status(okText || '');
    refresh();
  }

  document.getElementById('modes').addEventListener('click', (e) => {
    const button = e.target.closest('[data-mode]');
    if (button) show(button.dataset.mode);
  });
  document.getElementById('act-connect').addEventListener('click', () => {
    const id = targetId();
    const device = E.load().devices[id];
    if (device && device.connected) act(E.disconnect(id), '已断开热点');
    else act(E.connect(id), '已连接。认证页会在设备画面里打开。');
  });
  document.getElementById('traffic').addEventListener('click', (e) => {
    const button = e.target.closest('[data-mb]');
    if (!button) return;
    const id = targetId();
    const mb = button.dataset.mb;
    const res = mb === 'full' ? E.fillQuota(id) : E.addTraffic(id, Number(mb) * E.MiB);
    act(res, '已记入 ' + (mb === 'full' ? '直到超过配额' : mb + ' MiB'));
  });
  document.getElementById('visit-form').addEventListener('submit', (e) => {
    e.preventDefault();
    const id = targetId();
    const host = document.getElementById('host').value;
    const res = E.visit(id, host);
    if (!res.ok) { status(res.error); return; }
    const kind = res.detail && res.detail.kind;
    const text = {
      portal: '还没认证，认证页已弹出',
      black: '命中黑名单',
      white: '白名单免认证放行',
      open: '已放行，示意页面已打开',
      blocked: '设备被封禁',
      quota: '配额已用尽'
    }[kind] || '已处理';
    status((mode === 'admin' ? '作用在手机上。' : '') + text);
    refresh();
  });
  document.getElementById('inbox').addEventListener('click', (e) => {
    const button = e.target.closest('[data-decide]');
    if (!button) return;
    const res = E.decide(button.dataset.id, button.dataset.decide, '来自演示推送');
    act(res, button.dataset.decide === 'allow' ? '已同意，设备可以上网' : '已拒绝');
  });
  document.getElementById('reset-demo').addEventListener('click', () => {
    if (!window.confirm('重置会清掉这台浏览器里的演示配置、设备和审批。')) return;
    E.reset();
    status('已恢复初始演示数据');
    refresh();
  });
  frame.addEventListener('load', sendTheme);
  window.addEventListener('resize', layout);
  window.addEventListener('ufi-demo-change', refresh);
  window.addEventListener('message', (e) => {
    if (e.origin !== location.origin || !e.data) return;
    if (e.data.type === 'ufi-demo-state') refresh();
    if (e.data.type === 'ufi-demo' && e.data.action === 'show') show(e.data.mode);
  });

  const initial = new URLSearchParams(location.search).get('mode') || 'phone';
  mode = '';
  show(MODES[initial] ? initial : 'phone');
  if (window.UFIGitHub) window.UFIGitHub.start();
})();
