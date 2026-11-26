(function () {
  'use strict';
  const REPO = 'nanbeiluduo/UFI-Gate';
  const API = 'https://api.github.com/repos/' + REPO;
  const RELEASES = 'https://github.com/' + REPO + '/releases';
  const CACHE = 10 * 60 * 1000;

  const $ = id => document.getElementById(id);

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, ch => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    }[ch]));
  }

  async function getJson(url) {
    const response = await fetch(url, {
      headers: { Accept: 'application/vnd.github+json' }
    });
    if (!response.ok) throw new Error(String(response.status));
    return response.json();
  }

  function cached(key) {
    try {
      const raw = sessionStorage.getItem(key);
      if (!raw) return null;
      const parsed = JSON.parse(raw);
      if (!parsed || Date.now() - parsed.t > CACHE) return null;
      return parsed.data;
    } catch (e) {
      return null;
    }
  }

  function store(key, data) {
    try { sessionStorage.setItem(key, JSON.stringify({ t: Date.now(), data: data })); } catch (e) { /* quota */ }
  }

  async function loadJson(key, url) {
    const hit = cached(key);
    if (hit) return hit;
    const data = await getJson(url);
    store(key, data);
    return data;
  }

  function formatBytes(n) {
    n = Number(n) || 0;
    if (n >= 1048576) return (n / 1048576).toFixed(1) + ' MiB';
    if (n >= 1024) return Math.round(n / 1024) + ' KiB';
    return n + ' B';
  }

  function formatDate(iso) {
    if (!iso) return '—';
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return '—';
    return date.toLocaleString('zh-CN', { hour12: false, year: 'numeric', month: 'long', day: 'numeric' });
  }

  function formatDay(iso) {
    if (!iso) return '—';
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return '—';
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    return date.getFullYear() + '-' + month + '-' + day;
  }

  function formatNumber(n) {
    return new Intl.NumberFormat('zh-CN').format(Number(n) || 0);
  }

  function setStat(id, value, detail) {
    const node = $(id);
    if (!node) return;
    node.querySelector('.stat-value').textContent = value;
    if (detail != null) node.querySelector('.stat-detail').textContent = detail;
  }

  function pluginAsset(release, arch) {
    return (release.assets || []).find(asset => new RegExp('-' + arch + '\\.js$', 'i').test(asset.name));
  }

  function renderRelease(release) {
    const arm64 = pluginAsset(release, 'arm64');
    const arm32 = pluginAsset(release, 'arm32');
    const sums = (release.assets || []).find(asset => asset.name === 'SHA256SUMS');
    const actions = $('download-actions');
    const buttons = [];
    if (arm64) buttons.push(assetButton(arm64, '下载 ARM64', 'aarch64 / arm64', true));
    if (arm32) buttons.push(assetButton(arm32, '下载 ARM32', 'armv7l / armv8l', !arm64));
    buttons.push('<a class="btn ghost" href="' + esc(release.html_url || RELEASES) + '">全部文件</a>');
    if (sums) buttons.push('<a class="btn text" href="' + esc(sums.browser_download_url) + '">SHA256SUMS</a>');
    actions.innerHTML = buttons.join('');
    $('download-version').textContent = release.name || release.tag_name || '最新版本';
    $('download-date').textContent = '发布于 ' + formatDate(release.published_at);
    const fallback = $('download-fallback');
    if (fallback) fallback.hidden = !!(arm64 || arm32);
  }

  function assetButton(asset, label, hint, primary) {
    return '<a class="btn ' + (primary ? 'primary' : 'secondary') + '" href="' + esc(asset.browser_download_url) + '">' +
      '<span>' + esc(label) + '</span><small>' + esc(asset.name) + ' · ' + formatBytes(asset.size) + '</small>' +
      '<em>' + esc(hint) + '</em></a>';
  }

  async function allReleases() {
    const hit = cached('ufi-releases');
    if (hit) return hit;
    const all = [];
    for (let page = 1; page <= 10; page++) {
      const batch = await getJson(API + '/releases?per_page=100&page=' + page);
      all.push.apply(all, batch);
      if (!Array.isArray(batch) || batch.length < 100) break;
    }
    store('ufi-releases', all);
    return all;
  }

  function showDownloadError() {
    $('download-version').textContent = '前往发布页下载';
    $('download-date').textContent = '暂时读不到最新版本，Releases 页面仍然可用';
    const fallback = $('download-fallback');
    if (fallback) fallback.hidden = false;
  }

  function lineChart(views, clones) {
    const byDate = new Map();
    (views || []).forEach(row => byDate.set(row.date, { views: row.count || 0, clones: 0 }));
    (clones || []).forEach(row => {
      const item = byDate.get(row.date) || { views: 0, clones: 0 };
      item.clones = row.count || 0;
      byDate.set(row.date, item);
    });
    const dates = Array.from(byDate.keys()).sort();
    if (!dates.length) return '';
    const max = Math.max(1, ...dates.map(date => Math.max(byDate.get(date).views, byDate.get(date).clones)));
    const w = 720;
    const h = 220;
    const left = 36;
    const right = w - 8;
    const top = 16;
    const bottom = 188;
    const x = i => left + (dates.length === 1 ? (right - left) / 2 : i * (right - left) / (dates.length - 1));
    const y = value => bottom - (value / max) * (bottom - top);
    const path = key => dates.map((date, i) => (i ? 'L' : 'M') + x(i).toFixed(1) + ',' + y(byDate.get(date)[key]).toFixed(1)).join(' ');
    const ticks = [0, Math.round(max / 2), max];
    return '<svg class="chart" viewBox="0 0 ' + w + ' ' + h + '" role="img" aria-label="仓库浏览与克隆">' +
      ticks.map(tick => '<path class="gridline" d="M' + left + ' ' + y(tick).toFixed(1) + 'H' + right + '"/><text x="0" y="' + (y(tick) + 4).toFixed(1) + '">' + tick + '</text>').join('') +
      '<path d="' + path('views') + '" stroke="var(--accent)" stroke-width="2.5" fill="none"/>' +
      '<path d="' + path('clones') + '" stroke="#799c45" stroke-width="2.5" fill="none"/>' +
      '<text x="' + left + '" y="210">' + esc(dates[0].slice(5)) + '</text>' +
      '<text x="' + (w - 42) + '" y="210">' + esc(dates[dates.length - 1].slice(5)) + '</text></svg>' +
      '<div class="legend"><span><i></i>浏览</span><span><i class="down"></i>克隆</span></div>';
  }

  function sum(rows, key) {
    return (rows || []).reduce((total, row) => total + (Number(row[key]) || 0), 0);
  }

  function renderTraffic(doc) {
    const panel = $('traffic-panel');
    if (!panel) return;
    const views = doc && Array.isArray(doc.views) ? doc.views : [];
    const clones = doc && Array.isArray(doc.clones) ? doc.clones : [];
    if (!views.length && !clones.length) {
      const reason = doc && doc.last_error ? doc.last_error : '还没有采集到记录。';
      panel.innerHTML = '<div class="empty-traffic"><h3>流量历史尚未开始</h3><p>' + esc(reason) + ' 星标、复刻和下载次数来自公开接口，不依赖这份文件。</p></div>';
      setStat('stat-views', '—', '等待定时任务写入');
      setStat('stat-clones', '—', '等待定时任务写入');
      return;
    }
    panel.innerHTML = lineChart(views, clones) +
      '<p class="note">累计自首次采集以来的记录，可以超过 GitHub 只保留的约 14 天。最近写入 ' + esc(formatDate(doc.updated)) + '。独立访客按日相加，跨日会重复计算。</p>';
    setStat('stat-views', formatNumber(sum(views, 'count')), '独立访客按日合计 ' + formatNumber(sum(views, 'uniques')));
    setStat('stat-clones', formatNumber(sum(clones, 'count')), '独立克隆按日合计 ' + formatNumber(sum(clones, 'uniques')));
  }

  async function loadTraffic() {
    try {
      const response = await fetch('data/traffic.json', { cache: 'no-cache' });
      if (!response.ok) throw new Error(String(response.status));
      renderTraffic(await response.json());
    } catch (e) {
      renderTraffic(null);
    }
  }

  function statFallback() {
    ['stat-stars', 'stat-forks', 'stat-watchers', 'stat-issues', 'stat-downloads', 'stat-version', 'stat-updated'].forEach(id => {
      setStat(id, '—', '暂时无法读取');
    });
  }

  async function init() {
    const star = $('star-count');
    loadTraffic();
    try {
      const repo = await loadJson('ufi-repo', API);
      if (star) star.textContent = formatNumber(repo.stargazers_count);
      setStat('stat-stars', formatNumber(repo.stargazers_count), 'GitHub 星标');
      setStat('stat-forks', formatNumber(repo.forks_count), '复刻');
      setStat('stat-watchers', formatNumber(repo.subscribers_count), '正在关注的人');
      setStat('stat-updated', formatDay(repo.pushed_at), '最近推送 ' + formatDate(repo.pushed_at));
      const starLink = $('star-link');
      if (starLink) starLink.href = repo.html_url || starLink.href;
    } catch (e) {
      if (star) star.textContent = 'GitHub';
      statFallback();
    }

    try {
      const issues = await loadJson('ufi-issues', 'https://api.github.com/search/issues?q=' + encodeURIComponent('repo:' + REPO + ' is:issue is:open') + '&per_page=1');
      setStat('stat-issues', formatNumber(issues.total_count), '不含拉取请求');
    } catch (e) {
      setStat('stat-issues', '—', '议题接口暂不可用');
    }

    try {
      const latest = await loadJson('ufi-latest', API + '/releases/latest');
      renderRelease(latest);
      setStat('stat-version', latest.tag_name || '—', latest.name || '最新正式版');
    } catch (e) {
      showDownloadError();
      setStat('stat-version', '—', '打开 Releases 查看');
    }

    try {
      const releases = await allReleases();
      const downloads = releases.reduce((total, release) => total + (release.assets || []).reduce((sumAssets, asset) => sumAssets + (asset.download_count || 0), 0), 0);
      setStat('stat-downloads', formatNumber(downloads), '全部版本的安装包与附件');
    } catch (e) {
      setStat('stat-downloads', '—', '发布列表暂不可用');
    }
  }

  const badge = $('visit-badge');
  if (badge) {
    badge.addEventListener('error', () => {
      badge.replaceWith(Object.assign(document.createElement('span'), { className: 'muted', textContent: '计数暂不可用' }));
    });
  }

  init();
})();
