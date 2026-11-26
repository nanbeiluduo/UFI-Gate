(function () {
  const key = 'ufi-site-theme';
  function explicit() {
    return document.documentElement.getAttribute('data-theme');
  }
  function current() {
    return explicit() || (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  }
  function paint(theme) {
    if (theme === 'light' || theme === 'dark') {
      document.documentElement.setAttribute('data-theme', theme);
      document.documentElement.style.colorScheme = theme;
    } else {
      document.documentElement.removeAttribute('data-theme');
      document.documentElement.style.colorScheme = 'light dark';
    }
    const button = document.getElementById('theme-toggle');
    if (button) {
      const dark = current() === 'dark';
      button.textContent = dark ? '浅色' : '深色';
      button.setAttribute('aria-pressed', dark ? 'true' : 'false');
    }
    const payload = { type: 'ufi-theme', theme: explicit() || 'auto' };
    document.querySelectorAll('iframe').forEach((frame) => {
      try { if (frame.contentWindow) frame.contentWindow.postMessage(payload, location.origin); } catch (e) { /* ignore */ }
    });
  }
  const button = document.getElementById('theme-toggle');
  if (button) {
    button.addEventListener('click', function () {
      const next = current() === 'dark' ? 'light' : 'dark';
      try { localStorage.setItem(key, next); } catch (e) { /* ignore */ }
      paint(next);
    });
  }
  let stored = null;
  try { stored = localStorage.getItem(key); } catch (e) { stored = null; }
  paint(stored || 'auto');
  window.UFITheme = { paint: paint, current: current };
})();
