'use strict';
// Shared administrative interactions; never replace the browser's global confirm.
window.ufiAdminUI = (() => {
  let pending = false;
  function confirm(message, label = '确认操作') {
    if (pending) return Promise.resolve(false);
    pending = true;
    return new Promise(resolve => {
      const previous = document.activeElement;
      const modal = document.createElement('dialog');
      modal.id = 'confirm-dialog';
      modal.className = 'confirm-dialog';
      modal.setAttribute('aria-labelledby', 'confirm-title');
      modal.setAttribute('aria-describedby', 'confirm-message');
      modal.innerHTML = '<h2 id="confirm-title">请确认操作</h2><div class="dialog-body"><p id="confirm-message"></p><p class="note">请阅读操作影响，等待 3 秒后手动确认。</p></div><div class="dialog-actions"><button type="button" id="confirm-cancel" autofocus>取消</button><button type="button" id="confirm-accept" class="primary" disabled></button></div>';
      modal.querySelector('#confirm-message').textContent = message;
      document.body.append(modal);
      const accept = modal.querySelector('#confirm-accept');
      const deadline = performance.now() + 3000;
      let timer, settled = false;
      function finish(result) {
        if (settled) return;
        settled = true;
        clearInterval(timer);
        modal.close();
        modal.remove();
        pending = false;
        if (previous?.isConnected) previous.focus({preventScroll:true});
        resolve(result);
      }
      function tick() {
        const remaining = Math.max(0, Math.ceil((deadline - performance.now()) / 1000));
        accept.disabled = remaining > 0;
        accept.textContent = label + (remaining ? '（' + remaining + 's）' : '');
      }
      modal.querySelector('#confirm-cancel').onclick = () => finish(false);
      accept.onclick = () => { if (performance.now() >= deadline) finish(true); };
      modal.addEventListener('cancel', e => { e.preventDefault(); finish(false); });
      modal.addEventListener('close', () => finish(false));
      modal.addEventListener('click', e => {
        const r = modal.getBoundingClientRect();
        if (e.target === modal && (e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom)) finish(false);
      });
      tick();
      timer = setInterval(tick, 100);
      modal.showModal();
    });
  }
  let cleanup = () => {};
  function mountNavigation() {
    cleanup();
    const sidebar = document.getElementById('sidebar'), menu = document.getElementById('menu');
    const backdrop = document.getElementById('nav-backdrop'), close = document.getElementById('nav-close');
    const mobile = matchMedia('(max-width: 767px)');
    function setOpen(open, restore = false) {
      open = open && mobile.matches;
      sidebar.classList.toggle('open', open);
      document.body.classList.toggle('nav-open', open);
      sidebar.inert = mobile.matches && !open;
      menu.setAttribute('aria-expanded', String(open));
      backdrop.hidden = !open;
      if (open) close.focus();
      else if (restore) menu.focus();
    }
    function keydown(e) {
      if (!sidebar.classList.contains('open')) return;
      if (e.key === 'Escape') { e.preventDefault(); setOpen(false, true); }
      if (e.key === 'Tab') {
        const items = [...sidebar.querySelectorAll('button, a[href]')].filter(e => !e.disabled && e.getClientRects().length);
        const first = items[0], last = items[items.length - 1];
        if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
        else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
      }
    }
    menu.onclick = () => setOpen(!sidebar.classList.contains('open'));
    close.onclick = backdrop.onclick = () => setOpen(false, true);
    const resize = () => setOpen(false);
    mobile.addEventListener('change', resize);
    document.addEventListener('keydown', keydown);
    cleanup = () => { mobile.removeEventListener('change', resize); document.removeEventListener('keydown', keydown); document.body.classList.remove('nav-open'); };
    window.ufiAdminUI.closeNavigation = () => setOpen(false);
    setOpen(false);
  }
  return {confirm, mountNavigation, closeNavigation: () => {}, resetNavigation: () => cleanup()};
})();
