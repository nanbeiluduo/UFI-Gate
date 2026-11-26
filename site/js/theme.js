(function () {
  'use strict';
  const KEY = 'ufi-site-theme';

  function apply(theme) {
    if (theme === 'light' || theme === 'dark') document.documentElement.setAttribute('data-theme', theme);
    else document.documentElement.removeAttribute('data-theme');
    const stored = theme === 'light' || theme === 'dark' ? theme : 'system';
    document.querySelectorAll('[data-theme-toggle]').forEach(btn => {
      const label = stored === 'dark' ? '深色' : stored === 'light' ? '浅色' : '跟随系统';
      btn.setAttribute('aria-label', '主题：' + label + '。点击切换');
      btn.dataset.mode = stored;
    });
  }

  try { apply(localStorage.getItem(KEY) || ''); } catch (e) { apply(''); }

  document.addEventListener('click', event => {
    const btn = event.target.closest('[data-theme-toggle]');
    if (!btn) return;
    const order = ['', 'dark', 'light'];
    let current = '';
    try { current = localStorage.getItem(KEY) || ''; } catch (e) { current = ''; }
    const next = order[(order.indexOf(current) + 1) % order.length];
    try {
      if (next) localStorage.setItem(KEY, next);
      else localStorage.removeItem(KEY);
    } catch (e) { /* ignore */ }
    apply(next);
  });
})();
