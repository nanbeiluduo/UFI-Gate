/* Public GitHub metadata for the landing page. Stars and release downloads only. */
(function (root, factory) {
  const api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  root.UFIGitHub = api;
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  const REPO = 'nanbeiluduo/UFI-Gate';
  const CACHE_KEY = 'ufi-gate-gh-v1';
  const TTL = 10 * 60 * 1000;
  const RELEASES = 'https://github.com/' + REPO + '/releases';
  const REPO_URL = 'https://github.com/' + REPO;

  function archOf(name) {
    const n = String(name || '').toLowerCase();
    if (n.endsWith('-arm64.js')) return 'arm64';
    if (n.endsWith('-arm32.js')) return 'arm32';
    return '';
  }
  function summarize(repo, releases, latest) {
    const stars = repo && Number.isFinite(Number(repo.stargazers_count)) ? Number(repo.stargazers_count) : null;
    const list = Array.isArray(releases) ? releases : [];
    const downloads = list.reduce((sum, rel) => {
      return sum + (rel.assets || []).reduce((inner, asset) => inner + (Number(asset.download_count) || 0), 0);
    }, 0);
    let rel = latest && latest.tag_name ? latest : null;
    if (!rel) rel = list.find((item) => item.tag_name && !item.draft && !item.prerelease) || list.find((item) => item.tag_name) || null;
    let release = null;
    if (rel) {
      const assets = (rel.assets || []).map((asset) => ({
        name: asset.name,
        url: asset.browser_download_url,
        size: Number(asset.size) || 0,
        arch: archOf(asset.name)
      }));
      release = {
        tag: rel.tag_name,
        name: rel.name || rel.tag_name,
        published: rel.published_at || '',
        prerelease: !!rel.prerelease,
        htmlUrl: rel.html_url || (RELEASES + '/tag/' + rel.tag_name),
        arm64: assets.find((asset) => asset.arch === 'arm64') || null,
        arm32: assets.find((asset) => asset.arch === 'arm32') || null
      };
    }
    return {
      stars: stars,
      downloads: list.length || (latest && latest.tag_name) ? downloads : null,
      release: release,
      repoUrl: REPO_URL,
      releasesUrl: RELEASES
    };
  }
  function readCache() {
    try {
      const raw = localStorage.getItem(CACHE_KEY);
      if (!raw) return null;
      const data = JSON.parse(raw);
      if (!data || !data.summary) return null;
      return data;
    } catch (e) {
      return null;
    }
  }
  function writeCache(summary) {
    try {
      localStorage.setItem(CACHE_KEY, JSON.stringify({ at: Date.now(), summary: summary }));
    } catch (e) { /* ignore quota */ }
  }
  async function fetchJSON(url) {
    const response = await fetch(url, {
      headers: { Accept: 'application/vnd.github+json' }
    });
    if (!response.ok) {
      const error = new Error('GitHub API ' + response.status);
      error.status = response.status;
      throw error;
    }
    return response.json();
  }
  async function fetchReleases() {
    const all = [];
    for (let page = 1; page <= 10; page++) {
      const batch = await fetchJSON('https://api.github.com/repos/' + REPO + '/releases?per_page=100&page=' + page);
      if (!Array.isArray(batch)) throw new Error('releases');
      all.push.apply(all, batch);
      if (batch.length < 100) break;
    }
    return all;
  }
  async function load(options) {
    const opts = options || {};
    const cached = readCache();
    if (!opts.force && cached && Date.now() - cached.at < TTL) {
      return { summary: cached.summary, source: 'cache', stale: false };
    }
    try {
      const repo = await fetchJSON('https://api.github.com/repos/' + REPO);
      let latest = null;
      try { latest = await fetchJSON('https://api.github.com/repos/' + REPO + '/releases/latest'); } catch (e) { latest = null; }
      const releases = await fetchReleases();
      const summary = summarize(repo, releases, latest);
      if (summary.stars == null) throw new Error('stars');
      writeCache(summary);
      return { summary: summary, source: 'network', stale: false };
    } catch (e) {
      if (cached) return { summary: cached.summary, source: 'cache', stale: true, error: e };
      throw e;
    }
  }
  function formatCount(n) {
    if (n == null || !Number.isFinite(Number(n))) return '—';
    return new Intl.NumberFormat('zh-CN').format(Number(n));
  }
  function formatBytes(n) {
    n = Number(n) || 0;
    if (n >= 1048576) return (n / 1048576).toFixed(1) + ' MiB';
    if (n >= 1024) return Math.round(n / 1024) + ' KiB';
    return n + ' B';
  }
  function formatDate(iso) {
    if (!iso) return '';
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return '';
    return date.toLocaleDateString('zh-CN', { year: 'numeric', month: 'long', day: 'numeric' });
  }
  function render(result) {
    const summary = result.summary;
    document.querySelectorAll('[data-star]').forEach((el) => { el.textContent = formatCount(summary.stars); });
    document.querySelectorAll('[data-downloads]').forEach((el) => { el.textContent = formatCount(summary.downloads); });
    const meta = document.getElementById('release-meta');
    const actions = document.getElementById('download-actions');
    const fallback = document.getElementById('download-fallback');
    const status = document.getElementById('gh-status');
    if (status) {
      if (result.stale) status.textContent = '接口暂时不可用，显示的是这台浏览器上次成功获取的数字。';
      else if (result.source === 'cache') status.textContent = '数字来自 GitHub 公开接口，并在这台浏览器里缓存了几分钟。';
      else status.textContent = '数字来自 GitHub 公开接口。';
    }
    if (!actions) return;
    const release = summary.release;
    if (!release || (!release.arm64 && !release.arm32)) {
      if (meta) meta.textContent = '还没有可直接下载的安装包。';
      actions.innerHTML = '';
      if (fallback) fallback.hidden = false;
      return;
    }
    if (fallback) fallback.hidden = true;
    const when = formatDate(release.published);
    if (meta) {
      meta.textContent = release.tag + (when ? ' · 发布于 ' + when : '') + (release.prerelease ? ' · 预发布' : '');
    }
    const buttons = [];
    if (release.arm64) {
      buttons.push(assetLink(release.arm64, '下载 ARM64 安装包', 'aarch64 / arm64'));
    }
    if (release.arm32) {
      buttons.push(assetLink(release.arm32, '下载 ARM32 安装包', 'armv7l / armv8l'));
    }
    actions.innerHTML = buttons.join('');
  }
  function esc(value) {
    return String(value ?? '').replace(/[&<>"']/g, (ch) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch]));
  }
  function assetLink(asset, label, arch) {
    const safeUrl = esc(asset.url || RELEASES);
    return '<a class="btn primary" href="' + safeUrl + '"><span>' + esc(label) + '</span><small>' + esc(asset.name) + ' · ' + formatBytes(asset.size) + ' · ' + esc(arch) + '</small></a>';
  }
  function renderFailure() {
    document.querySelectorAll('[data-star], [data-downloads]').forEach((el) => {
      if (!el.textContent || el.textContent === '…') el.textContent = '—';
    });
    const meta = document.getElementById('release-meta');
    const actions = document.getElementById('download-actions');
    const fallback = document.getElementById('download-fallback');
    const status = document.getElementById('gh-status');
    if (meta) meta.textContent = '暂时读不到最新版本。';
    if (actions) actions.innerHTML = '';
    if (fallback) fallback.hidden = false;
    if (status) status.textContent = 'GitHub 接口没有返回数据。星标和下载量先留空，安装包请到 Releases 页面下载。';
  }
  async function start() {
    try {
      const result = await load();
      render(result);
    } catch (e) {
      renderFailure();
    }
  }
  return {
    REPO, RELEASES, REPO_URL, summarize, load, render, renderFailure, start, formatCount, formatBytes, archOf
  };
});
