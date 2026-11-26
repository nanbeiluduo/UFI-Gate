(function () {
  'use strict';
  const D = window.UFIDemo;
  let target = 'phone';
  let mode = 'phone';

  const frames = {
    phone: '手机访客',
    tablet: '平板访客',
    pc: '电脑访客',
    admin: '管理后台'
  };

  function $(id) { return document.getElementById(id); }

  function show(next) {
    mode = next;
    document.querySelectorAll('[data-frame-btn]').forEach(btn => {
      const on = btn.dataset.frameBtn === next;
      btn.classList.toggle('active', on);
      btn.setAttribute('aria-pressed', on ? 'true' : 'false');
    });
    document.querySelectorAll('.slot').forEach(slot => {
      const visible = next === 'split'
        ? slot.dataset.frame === 'phone' || slot.dataset.frame === 'admin'
        : slot.dataset.frame === next;
      slot.hidden = !visible;
    });
    $('stage-frames').classList.toggle('split', next === 'split');
    if (next === 'phone' || next === 'tablet' || next === 'pc') {
      target = next;
      $('target').value = next;
    }
    renderChrome(D.load());
    requestAnimationFrame(fit);
  }

  function fit() {
    const stage = $('stage-frames');
    const visible = Array.from(document.querySelectorAll('.slot')).filter(slot => !slot.hidden);
    const stageW = stage.clientWidth || 360;
    const gap = 12;
    visible.forEach(slot => {
      const wrap = slot.querySelector('.bezel-wrap');
      const width = Number(wrap.dataset.w);
      const height = Number(wrap.dataset.h);
      let slotW = stageW;
      if (visible.length > 1) {
        const phoneW = Math.min(360, Math.max(240, stageW * 0.36));
        slotW = slot.dataset.frame === 'phone' ? phoneW : Math.max(280, stageW - phoneW - gap);
      }
      const availH = Math.max(460, window.innerHeight - 300);
      const scale = Math.min(1, slotW / width, availH / height);
      wrap.style.setProperty('--w', String(width));
      wrap.style.setProperty('--h', String(height));
      wrap.style.setProperty('--scale', String(Math.max(0.2, scale)));
    });
  }

  function steps(state) {
    const phone = state.clients.phone;
    const dev = D.findDevice(state, phone.mac);
    const approved = state.requests.some(item => item.status === 'allow') || (dev && (dev.trusted || dev.until > Date.now()));
    return [
      { ok: phone.connected, text: '手机已连上热点' },
      { ok: phone.connected && phone.phase !== 'offline', text: '手机上出现了认证或结果页' },
      { ok: state.requests.some(item => item.mac === phone.mac) || ['online', 'limited', 'quota'].includes(phone.phase), text: '提交过申请，或已经通过认证' },
      { ok: state.messages.some(item => item.kind === 'approval' || item.kind === 'result'), text: '管理后台收到了消息' },
      { ok: phone.phase === 'online' || phone.phase === 'limited' || (approved && phone.phase === 'quota'), text: '手机获得上网结果' },
      { ok: !!(dev && (dev.dayUp + dev.dayDown) > 0), text: '手机产生过流量，配额条会变化' }
    ];
  }

  function renderChrome(state) {
    const list = $('steps');
    list.innerHTML = steps(state).map(item => '<li class="' + (item.ok ? 'ok' : '') + '">' + item.text + '</li>').join('');
    const sim = state.simulating[target];
    $('sim-btn').classList.toggle('active', !!sim);
    $('sim-btn').textContent = sim ? '停止模拟上网' : '模拟上网';
    $('sim-btn').setAttribute('aria-pressed', sim ? 'true' : 'false');
    const client = state.clients[target];
    $('link-state').textContent = client.connected ? '已连接 · ' + phaseLabel(client.phase) : '未连接';
  }

  function phaseLabel(phase) {
    return {
      offline: '未连接', portal: '认证页', waiting: '等待审批', online: '已放行',
      limited: '限速中', quota: '配额用尽', blocked: '已封禁', open: '认证关闭',
      whitelist: '白名单', black: '黑名单'
    }[phase] || phase;
  }

  function act(mutator) {
    renderChrome(D.update(mutator));
  }

  document.querySelectorAll('[data-frame-btn]').forEach(btn => {
    btn.onclick = () => show(btn.dataset.frameBtn);
  });
  $('target').onchange = () => { target = $('target').value; renderChrome(D.load()); };
  $('connect-btn').onclick = () => act(s => D.connect(s, target));
  $('disconnect-btn').onclick = () => act(s => D.disconnect(s, target));
  $('browse-btn').onclick = () => act(s => D.browse(s, target, 'normal'));
  $('white-btn').onclick = () => act(s => D.browse(s, target, 'white'));
  $('black-btn').onclick = () => act(s => D.browse(s, target, 'black'));
  $('sim-btn').onclick = () => act(s => {
    if (!s.clients[target].connected) D.connect(s, target);
    s.simulating[target] = !s.simulating[target];
  });
  $('reset-btn').onclick = () => {
    if (!confirm('重置演示？当前浏览器里的配置、设备和审批都会回到初始状态。')) return;
    renderChrome(D.reset());
  };

  window.addEventListener('message', event => {
    if (event.origin !== location.origin) return;
    if (!event.data || event.data.type !== 'ufi-demo') return;
    if (event.data.action === 'show' && frames[event.data.frame]) show(event.data.frame);
  });

  D.subscribe(renderChrome);
  window.addEventListener('resize', fit);
  show('phone');
  renderChrome(D.load());

  setInterval(() => {
    const state = D.load();
    if (!Object.values(state.simulating).some(Boolean)) return;
    const changed = D.update(D.applyTick);
    if (changed) renderChrome(changed);
  }, 1000);
})();
